// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2025 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package snapstate

import (
	"errors"
	"fmt"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/i18n"
	"github.com/snapcore/snapd/kernel"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/overlord/snapstate/backend"
	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/snap"
	"gopkg.in/tomb.v2"
)

// checkKernelDriversTreeChangeKind/checkKernelDriversTreeTaskKind identify
// the single-task change/task used to check whether the currently active
// kernel snap's on-disk drivers tree needs to be regenerated because the
// generation logic changed since it was last built (see
// kernel.DriversTreeNeedsCheck), without requiring a kernel snap refresh.
const (
	checkKernelDriversTreeChangeKind = "check-kernel-drivers-tree"
	checkKernelDriversTreeTaskKind   = "check-kernel-drivers-tree"
)

// checkKernelDriversTreeTask builds a single-task TaskSet for the current
// kernel snap, carrying enough snap-setup information for
// SnapsAffectedByTask/CheckChangeConflict to treat it like any other
// operation touching that snap.
func checkKernelDriversTreeTask(st *state.State, kernelInfo *snap.Info) *state.TaskSet {
	t := st.NewTask(checkKernelDriversTreeTaskKind,
		fmt.Sprintf(i18n.G("Check kernel drivers tree for %q"), kernelInfo.InstanceName()))
	t.Set("snap-setup", &SnapSetup{
		SideInfo: &kernelInfo.SideInfo,
		Type:     snap.TypeKernel,
	})
	return state.NewTaskSet(t)
}

// doCheckKernelDriversTree is the handler for checkKernelDriversTreeTaskKind.
// It re-derives the current kernel snap and its currently active
// kernel-modules components live at execution time (mirroring
// doDiscardOldKernelSnapSetup's style) rather than trusting anything
// stashed on the task, and asks the backend to check (and, if needed,
// regenerate) the on-disk drivers tree. There is no undo handler: this is a
// best-effort, idempotent verify/fix-forward operation with no other
// system state depending on it being reversed. If it fails, the change is
// left in an error state and the next SnapManager.Ensure() tick will detect
// that the on-disk marker is still stale and create a fresh change to
// retry.
func (m *SnapManager) doCheckKernelDriversTree(t *state.Task, _ *tomb.Tomb) error {
	st := t.State()
	st.Lock()
	defer st.Unlock()

	deviceCtx, err := DeviceCtx(st, t, nil)
	if err != nil {
		return err
	}
	kernelInfo, err := KernelInfo(st, deviceCtx)
	if err != nil {
		return err
	}

	var snapst SnapState
	if err := Get(st, kernelInfo.InstanceName(), &snapst); err != nil {
		return err
	}
	currentComps := snapst.Sequence.ComponentsWithTypeForRev(snapst.Current, snap.KernelModulesComponent)

	st.Unlock()
	pm := NewTaskProgressAdapterUnlocked(t)
	changed, setupErr := m.backend.SetupKernelSnap(
		kernelInfo.InstanceName(), kernelInfo.Revision, currentComps,
		&backend.SetupKernelSnapOptions{Regenerate: true}, pm)
	st.Lock()
	if setupErr != nil {
		return setupErr
	}

	if changed {
		logger.Noticef("kernel drivers tree for %q (%s) was regenerated", kernelInfo.InstanceName(), kernelInfo.Revision)
	} else {
		logger.Debugf("kernel drivers tree for %q (%s) already up to date", kernelInfo.InstanceName(), kernelInfo.Revision)
	}

	t.SetStatus(state.DoneStatus)
	return nil
}

// ensureKernelDriversTreeChecked looks at the currently active kernel snap
// (if any) and, if its on-disk drivers tree was built by older generation
// logic than what is currently running, launches a best-effort change to
// check and, if needed, regenerate it - without requiring a kernel snap
// refresh. See kernel.DriversTreeNeedsCheck for why this is safe to run
// unconditionally and cheaply on every Ensure() tick.
func (m *SnapManager) ensureKernelDriversTreeChecked() error {
	logger.Trace("ensure", "manager", "SnapManager", "func", "ensureKernelDriversTreeChecked")

	m.state.Lock()
	defer m.state.Unlock()

	var seeded bool
	if err := m.state.Get("seeded", &seeded); err != nil && !errors.Is(err, state.ErrNoState) {
		return err
	}
	if !seeded {
		return nil
	}

	deviceCtx, err := DeviceCtx(m.state, nil, nil)
	if err != nil {
		// model not known yet, or similar - nothing to do
		if errors.Is(err, state.ErrNoState) {
			return nil
		}
		return err
	}
	if deviceCtx.Model() == nil || !kernel.NeedsKernelDriversTree(deviceCtx.Model()) {
		return nil
	}

	kernelInfo, err := KernelInfo(m.state, deviceCtx)
	if err != nil {
		if errors.Is(err, state.ErrNoState) {
			return nil
		}
		return err
	}

	destDir := kernel.DriversTreeDir(dirs.GlobalRootDir, kernelInfo.InstanceName(), kernelInfo.Revision)
	exists, isDir, err := osutil.DirExists(destDir)
	if err != nil {
		return err
	}
	if !exists || !isDir {
		// Nothing to check against/regenerate; leave it to the normal
		// install flow.
		return nil
	}

	needsCheck, err := kernel.DriversTreeNeedsCheck(destDir)
	if err != nil {
		return err
	}
	if !needsCheck {
		return nil
	}

	// Coarse pre-check: defer entirely while anything else is happening -
	// this is what makes this run "after snapd (and everything else) is
	// done", and naturally wait out a change that ends in a reboot (its
	// tasks stay non-Ready across the reboot). See ensureUbuntuCoreTransition
	// for the exact same changeInFlight guard.
	if changeInFlight(m.state) {
		return nil
	}

	// Precise, authoritative guard (defense in depth on top of the coarse
	// check above): conflicts with a real in-flight kernel/component
	// change, or our own previously-launched check that hasn't finished
	// yet, prevent a duplicate launch.
	if err := CheckChangeConflict(m.state, kernelInfo.InstanceName(), nil); err != nil {
		return nil // retry next Ensure() tick
	}

	ts := checkKernelDriversTreeTask(m.state, kernelInfo)
	chg := m.state.NewChange(checkKernelDriversTreeChangeKind,
		fmt.Sprintf(i18n.G("Check kernel drivers tree for %q"), kernelInfo.InstanceName()))
	chg.AddAll(ts)

	return nil
}

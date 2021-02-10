// -*- Mode: Go; indent-tabs-mode: t -*-
/*
 * Copyright (C) 2021 Canonical Ltd
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

package devicestate

import (
	"fmt"

	"gopkg.in/tomb.v2"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/boot"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/overlord/assertstate"
	"github.com/snapcore/snapd/overlord/snapstate"
	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/release"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/strutil"
)

func (m *DeviceManager) doCreateRecoverySystem(t *state.Task, _ *tomb.Tomb) error {
	if release.OnClassic {
		// TODO: this may need to be lifted in the future
		return fmt.Errorf("cannot run update gadget assets task on a classic system")
	}

	st := t.State()
	st.Lock()
	defer st.Unlock()

	remodelCtx, err := DeviceCtx(st, t, nil)
	if err != nil {
		return err
	}
	isRemodel := remodelCtx.ForRemodeling()
	groundDeviceCtx := remodelCtx.GroundContext()

	model := groundDeviceCtx.Model()
	if isRemodel {
		model = remodelCtx.Model()
	}

	var label string
	if err := t.Get("system-label", &label); err != nil {
		return fmt.Errorf("cannot create recovery system without a label")
	}

	// get all infos
	infoGetter := func(name string) (*snap.Info, bool, error) {
		// snap may be present in the system in which case info comes
		// from snapstate
		info, err := snapstate.CurrentInfo(st, name)
		if err == nil {
			logger.Noticef("info for installed snap %q, id %q", name, info.ID())
			hash, _, err := asserts.SnapFileSHA3_384(info.MountFile())
			if err != nil {
				return nil, true, fmt.Errorf("cannot compute SHA3 of snap file: %v", err)
			}
			info.Sha3_384 = hash
			return info, true, nil
		}
		if _, ok := err.(*snap.NotInstalledError); !ok {
			return nil, false, err
		}
		logger.Noticef("info for not yet installed snap %q", name)
		// snap is not installed, pull info from snapsup of relevant
		// snapsup of the download task
		return nil, false, fmt.Errorf("not implemented")
	}

	db := assertstate.DB(st)
	// 1. prepare recovery system from remodel snaps (or current snaps)
	newFiles, systemDir, err := createSystemForModelFromValidatedSnaps(infoGetter, db, label, model)
	logger.Noticef("recovery system dir: %v", systemDir)
	logger.Noticef("new common snap files: %v", newFiles)
	if err != nil {
		return fmt.Errorf("cannot create a recovery system with label %q for %v: %v", label, model.Model(), err)
	}

	// 2. set up boot variables for tracking the tried system state
	if err := boot.SetTryRecoverySystem(remodelCtx, label); err != nil {
		// rollback?
		return fmt.Errorf("cannot attempt booting into recovery system %q: %v", label, err)
	}
	// 3. and set up the next boot that that system
	if err := boot.SetRecoveryBootSystemAndMode(remodelCtx, label, "recover"); err != nil {
		return fmt.Errorf("cannot set device to boot into candidate system %q: %v", label, err)
	}

	// keep track of new files in task state
	t.Set("new-seed-system-files", newFiles)
	// this task is done, further processing happens in finalize
	t.SetStatus(state.DoneStatus)

	logger.Noticef("restarting into candidate system %q", label)
	m.state.RequestRestart(state.RestartSystemNow)
	return nil
}

func (m *DeviceManager) undoCreateRecoverySystem(t *state.Task, _ *tomb.Tomb) error {
	// XXX: clean up files, drop from current
	return nil
}

func (m *DeviceManager) doFinalizeTriedRecoverySystem(t *state.Task, _ *tomb.Tomb) error {
	if release.OnClassic {
		// TODO: this may need to be lifted in the future
		return fmt.Errorf("cannot run update gadget assets task on a classic system")
	}

	if ok, _ := t.State().Restarting(); ok {
		// don't continue until we are in the restarted snapd
		t.Logf("Waiting for system reboot...")
		return &state.Retry{}
	}

	// if in remodel, the recovery system is promoted to good ones at the end

	// see tried-systems list in state

	logger.Noticef("in finalize recovery system")

	st := t.State()
	st.Lock()
	defer st.Unlock()

	var triedSystems []string
	err := st.Get("tried-systems", &triedSystems)
	if err != nil {
		return fmt.Errorf("cannot obtain tried recovery systems: %v", err)
	}
	// XXX: needs to be set on this task too?
	var label string
	if err := t.Get("system-label", &label); err != nil {
		return fmt.Errorf("cannot obtain the recovery system label: %v", err)
	}

	// so far so good
	if !strutil.ListContains(triedSystems, label) {
		// system failed, trigger undoing of everything we did so far
		return fmt.Errorf("tried recovery system %q failed", label)
	}

	// XXX: candidate system is promoted to the list of good ones once we
	// complete the whole change

	remodelCtx, err := DeviceCtx(st, t, nil)
	if err != nil {
		return err
	}
	isRemodel := remodelCtx.ForRemodeling()
	if isRemodel {
		logger.Noticef("recovery system will be promoted later")
	}

	return nil
}

func (m *DeviceManager) undoFinalizeTriedRecoverySystem(t *state.Task, _ *tomb.Tomb) error {
	st := t.State()
	st.Lock()
	defer st.Unlock()

	remodelCtx, err := DeviceCtx(st, t, nil)
	if err != nil {
		return err
	}
	// isRemodel := remodelCtx.ForRemodeling()
	// groundDeviceCtx := remodelCtx.GroundContext()

	var label string
	if err := t.Get("system-label", &label); err != nil {
		return fmt.Errorf("cannot obtain the recovery system label: %v", err)
	}

	// TODO: demote recovery system from good ones and reseal

	if err := boot.DropGoodRecoverySystem(remodelCtx, label); err != nil {
		return fmt.Errorf("cannot demote a candidate recovery system: %v", err)
	}

	return nil
}

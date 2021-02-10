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

	"github.com/snapcore/snapd/boot"
	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/release"
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

	// XXX: create recovery system with some label
	// XXX: where does the label come from? task?
	// XXX: task may be run from a remodel change
	// XXX: idempotent

	// 1. prepare recovery system from remodel snaps (or current snaps)
	// 2. reboot to system
	//   a. trySwitchToSystemAndMode ?
	// 3. when back mark task as done

	label := ""
	if err := createSystemForModel(label, model); err != nil {
		return fmt.Errorf("cannot create a recovery system with label %q for %v: %v", label, model, err)
	}

	if err := boot.SetTryRecoverySystem(remodelCtx, label); err != nil {
		// rollback?
		return fmt.Errorf("cannot attempt booting into recovery system %q: %v", label, err)
	}

	if err := m.Reboot(label, "recover"); err != nil {
		return fmt.Errorf("cannot reboot into try recovery system %q: %v", label, err)
	}

	return nil
}

func (m *DeviceManager) undoCreateRecoverySystem(t *state.Task, _ *tomb.Tomb) error {
	return nil
}

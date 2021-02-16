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

package boot

import (
	"fmt"

	"github.com/snapcore/snapd/bootloader"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/strutil"
)

// clearTryRecoverySystem removes a given candidate recovery system from the
// modeenv state file, reseals and clears related bootloader variables.
func clearTryRecoverySystem(dev Device, systemLabel string) error {
	if !dev.HasModeenv() {
		return fmt.Errorf("internal error: recovery systems can only be used on UC20")
	}

	m, err := loadModeenv()
	if err != nil {
		return err
	}
	opts := &bootloader.Options{
		// setup the recovery bootloader
		Role: bootloader.RoleRecovery,
	}
	bl, err := bootloader.Find(InitramfsUbuntuSeedDir, opts)
	if err != nil {
		return err
	}

	found := false
	for idx, sys := range m.CurrentRecoverySystems {
		if sys == systemLabel {
			found = true
			m.CurrentRecoverySystems = append(m.CurrentRecoverySystems[:idx],
				m.CurrentRecoverySystems[idx+1:]...)
			break
		}
	}
	if found {
		// we may be repeating the cleanup, in which case the system may
		// not be present in modeenv already
		if err := m.Write(); err != nil {
			return err
		}
	}
	// but we still want to reseal, in case the cleanup did not reach this
	// point before
	const expectReseal = true
	resealErr := resealKeyToModeenv(dirs.GlobalRootDir, dev.Model(), m, expectReseal)

	// clear both variables, no matter the values they hold
	vars := map[string]string{
		"try_recovery_system":    "",
		"recovery_system_status": "",
	}
	// try to clear regardless of reseal failing
	blErr := bl.SetBootVars(vars)

	if resealErr != nil {
		return resealErr
	}
	return blErr
}

// SetTryRecoverySystem sets up the boot environment for trying out a recovery
// system with given label. Once done, the caller should request switching to a
// given recovery system.
func SetTryRecoverySystem(dev Device, systemLabel string) (err error) {
	if !dev.HasModeenv() {
		return fmt.Errorf("internal error: recovery systems can only be used on UC20")
	}

	m, err := loadModeenv()
	if err != nil {
		return err
	}

	opts := &bootloader.Options{
		// setup the recovery bootloader
		Role: bootloader.RoleRecovery,
	}
	// TODO:UC20: seed may need to be switched to RW
	bl, err := bootloader.Find(InitramfsUbuntuSeedDir, opts)
	if err != nil {
		return err
	}

	// we could have rebooted before resealing the keys
	if !strutil.ListContains(m.CurrentRecoverySystems, systemLabel) {
		m.CurrentRecoverySystems = append(m.CurrentRecoverySystems, systemLabel)
	}
	if err := m.Write(); err != nil {
		return err
	}

	defer func() {
		if err == nil {
			return
		}
		if cleanupErr := clearTryRecoverySystem(dev, systemLabel); cleanupErr != nil {
			err = fmt.Errorf("%v (cleanup failed: %v)", err, cleanupErr)
		}
	}()
	const expectReseal = true
	if err := resealKeyToModeenv(dirs.GlobalRootDir, dev.Model(), m, expectReseal); err != nil {
		return err
	}
	vars := map[string]string{
		"try_recovery_system":    systemLabel,
		"recovery_system_status": "try",
	}
	return bl.SetBootVars(vars)
}

// TryingRecoverySystem checks whether the boot variables indicate that the
// given recovery system is being tried.
func TryingRecoverySystem(currentSystemLabel string) (bool, error) {
	opts := &bootloader.Options{
		// setup the recovery bootloader
		Role: bootloader.RoleRecovery,
	}
	bl, err := bootloader.Find(InitramfsUbuntuSeedDir, opts)
	if err != nil {
		return false, err
	}

	vars, err := bl.GetBootVars("try_recovery_system", "recovery_system_status")
	if err != nil {
		return false, err
	}

	status := vars["recovery_system_status"]
	if status == "" {
		// not trying any recovery systems right now
		return false, nil
	}

	trySystem := vars["try_recovery_system"]
	if trySystem == "" {
		// XXX: could we end up with one variable set and the other not?
		return false, fmt.Errorf("try recovery system is unset")
	}

	if trySystem != currentSystemLabel {
		// this may still be ok, eg. if we're running the actual recovery system
		return false, nil
	}
	return true, nil
}

// MarkTryRecoverySystemResults updates the boot environment to indicate that
// the outcome of trying out a recovery system and sets up the system to boot
// into run mode.
func MarkTryRecoverySystemResultForRunMode(success bool) error {
	opts := &bootloader.Options{
		// setup the recovery bootloader
		Role: bootloader.RoleRecovery,
	}
	// TODO:UC20: seed may need to be switched to RW
	bl, err := bootloader.Find(InitramfsUbuntuSeedDir, opts)
	if err != nil {
		return err
	}

	vars := map[string]string{
		// always going to back to run mode
		"snapd_recovery_mode":   "run",
		"snapd_recovery_system": "",
	}
	if success {
		vars["recovery_system_status"] = "tried"
	}
	return bl.SetBootVars(vars)
}

// IsTryRecoverySystemSuccessful indicates whether the candidate recovery system
// of a given label has successfully booted and updated the boot environment.
func IsTryRecoverySystemSuccessful(systemLabel string) (success bool, err error) {
	opts := &bootloader.Options{
		// setup the recovery bootloader
		Role: bootloader.RoleRecovery,
	}
	// TODO:UC20: seed may need to be switched to RW
	bl, err := bootloader.Find(InitramfsUbuntuSeedDir, opts)
	if err != nil {
		return false, err
	}

	vars, err := bl.GetBootVars("try_recovery_system", "recovery_system_status")
	if err != nil {
		return false, err
	}

	// we expect both variables to be set
	status := vars["recovery_system_status"]
	if status == "" {
		return false, fmt.Errorf("internal error: recovery system status is unset")
	}
	trySystem := vars["try_recovery_system"]
	if trySystem == "" {
		return false, fmt.Errorf("internal error: try recovery system is unset")
	}
	if trySystem != systemLabel {
		return false, fmt.Errorf("internal error: try recovery system label mismatch, expected %q (got %q)",
			systemLabel, trySystem)
	}
	return status == "tried", nil
}

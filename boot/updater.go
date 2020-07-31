// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2020 Canonical Ltd
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
	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/gadget"
)

func InterceptManagedAssetsUpdate(model *asserts.Model) gadget.DelegateUpdaterFunc {
	if model.Grade() == asserts.ModelGradeUnset {
		// no need to intercept updates when assets are not managed
		return nil
	}

	return func(what *gadget.LaidOutStructure, updateContext *gadget.PendingStructureUpdateContext) (gadget.Updater, error) {
		return interceptManagedBootUpdate(model, what, updateContext)
	}
}

func interceptManagedBootUpdate(model *asserts.Model, what *gadget.LaidOutStructure, updateContext *gadget.PendingStructureUpdateContext) (gadget.Updater, error) {
	// TODO: not implemented
	return nil, nil
}

type managedAssetsAwareUpdater struct {
}

// Implements gadget.Updater.
func (m *managedAssetsAwareUpdater) Backup() error {
	// TODO:UC20:
	// - device lookup
	// - find bootloader
	// - cache new assets
	// - update modeenv, reseal with new and old command lines
	// - proceed with actual updater backup step
	return nil
}

// Implements gadget.Updater.
func (m *managedAssetsAwareUpdater) Update() error {
	// TODO:UC20: allow actual updater update step
	return nil
}

// Implements gadget.Updater.
func (m *managedAssetsAwareUpdater) Rollback() error {
	// TODO:UC20:
	// - actual updater rollback
	// - drop newly cached assets from modeenv, reseal with remaining assets
	// - drop newly cached assets
	return nil
}

func NewFilesystemWriter(model *asserts.Model, gadgetRoot string, ds *gadget.LaidOutStructure) (*bootFilesystemWriter, error) {
	return &bootFilesystemWriter{}, nil
}

type bootFilesystemWriter struct{}

func (b *bootFilesystemWriter) Write(mountPoint string, preserve []string) error {
	// TODO:UC20:
	// - populate assets cache
	// - delegate writing to gadget.MountedFilesystemWriter
	return nil
}

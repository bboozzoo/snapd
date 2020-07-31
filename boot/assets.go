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

func InterceptManagedAssets(model *asserts.Model) gadget.InterceptorFunc {
	if model.Grade() == asserts.ModelGradeUnset {
		// no need to intercept updates when assets are not managed
		return nil
	}

	return func(gadgetStruct *gadget.LaidOutStructure) (gadget.FileActionInterceptor, error) {
		return interceptManagedBootUpdate(model, what)
	}
}

func interceptManagedBootUpdate(model *asserts.Model, gadgetStruct *gadget.LaidOutStructure) (gadget.Updater, error) {
	if model.Grade() == asserts.ModelGradeUnset {
		// not uc20, nothing to intercept
		return nil, nil
	}

	return nil, nil
}

type managedAssetsInterceptor struct{}

// Implements gadget.FileActionInterceptor.
func (m *managedAssetsAwareUpdater) Intercept(action gadget.Action, root, realSource, relativeTarget string) (ActionResult, error) {
	// do we have a bootloader there?
	// is relativeTarget a managed asset?
	// prefer ActionResultNeedsBackup
	// return gadget.ErrNoUpdate when nothing happens
	return nil
}

// Implements gadget.FileActionInterceptor.
func (m *managedAssetsAwareUpdater) Apply(root string) error {
	// steps:
	// - copy managed assets to assets cache
	// - update modeeenv
	// - reseal
	// - write assets
	return nil
}

// Implements gadget.FileActionInterceptor.
func (m *managedAssetsAwareUpdater) Revert(root string) error {
	// - update modeenv
	// - drop files from cache
	// - reseal
	return nil
}

// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2019 Canonical Ltd
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

package backend

import (
	"fmt"

	"github.com/snapcore/snapd/progress"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/wrappers"
)

func (b Backend) UnlinkSnapdSnap(info *snap.Info, meter progress.Meter) error {
	if snapType := info.GetType(); snapType != snap.TypeSnapd {
		return fmt.Errorf("cannot unlink snap %q: type %v is unsupported by the backend",
			info.InstanceName(), snapType)
	}

	err1 := wrappers.UndoSnapdServicesOnCore(info, meter)

	// and finally remove current symlinks
	err2 := removeCurrentSymlinks(info)

	return firstErr(err1, err2)
}

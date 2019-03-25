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
package gadget

import (
	"sort"

	"github.com/snapcore/snapd/snap"
)

type Size snap.GadgetSize

type LayoutConstraints struct {
	NonMBRStartOffset snap.GadgetSize
	SectorSize        int
}

const (
	MiB = snap.GadgetSize(2 << 20)
)

type Volume struct {
	// Size is the total size of the volume
	Size snap.GadgetSize
	// Structures are sorted in order of 'appearance' in the volume
	Structures []snap.PositionedStructure
}

func LayOutVolume(volume *snap.GadgetVolume, constraints LayoutConstraints, data SnapContainer) (*Volume, error) {
	lastOffset := snap.GadgetSize(0)
	farthestEnd := snap.GadgetSize(0)
	structures := make([]snap.PositionedStructure, len(volume.Structure))

	for idx, s := range volume.Structure {
		start := s.Offset
		if start == 0 {
			if lastOffset < constraints.NonMBRStartOffset {
				start = constraints.NonMBRStartOffset
			} else {
				start = lastOffset
			}
		}
		end := start + s.Size
		ps := snap.PositionedStructure{
			VolumeStructure: &volume.Structure[idx],
			StartOffset:     start,
		}
		structures[idx] = ps

		if end > farthestEnd {
			farthestEnd = end
		}
		lastOffset = end
	}

	// sort by starting offset
	sort.Sort(snap.ByStartOffset(structures))

	vol := &Volume{
		Size:       farthestEnd,
		Structures: structures,
	}
	return vol, nil
}

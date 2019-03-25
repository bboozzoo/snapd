// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2016 Canonical Ltd
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

package gadget_test

import (
	"testing"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/snap"
)

type layoutTestSuite struct{}

var _ = Suite(&layoutTestSuite{})

func TestGadget(t *testing.T) {
	TestingT(t)
}

func (l *layoutTestSuite) TestVolumeSize(c *C) {
	vol := snap.GadgetVolume{
		Structure: []snap.VolumeStructure{
			{Size: 2 * gadget.MiB},
		},
	}
	constraints := gadget.LayoutConstraints{
		NonMBRStartOffset: 1 * gadget.MiB,
	}
	v, err := gadget.LayOutVolume(&vol, constraints, nil)
	c.Assert(err, IsNil)

	c.Assert(v, DeepEquals, &gadget.Volume{
		Size: 3 * gadget.MiB,
		Structures: []snap.PositionedStructure{
			{VolumeStructure: &snap.VolumeStructure{Size: 2 * gadget.MiB}, StartOffset: 1 * gadget.MiB},
		},
	})
}

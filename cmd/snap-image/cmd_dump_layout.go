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
package main

import (
	"fmt"

	"github.com/jessevdk/go-flags"

	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/i18n"
)

type cmdShowLayout struct {
	Positional struct {
		GadgetRootDir string
		VolumeName    string
	} `positional-args:"yes" required:"yes"`
}

func init() {
	addCommand("show-layout",
		func() flags.Commander {
			return &cmdShowLayout{}
		},
		nil,
		[]argDesc{{
			name: "<gadget-root-dir>",
			desc: i18n.G("Gadget root directory"),
		}, {
			name: "<volume-name>",
			desc: i18n.G("Volume name"),
		}})
}

func (x *cmdShowLayout) Execute(args []string) error {
	gi, err := gadget.ReadInfo(x.Positional.GadgetRootDir, nil)
	if err != nil {
		return err
	}

	vol, ok := gi.Volumes[x.Positional.VolumeName]
	if !ok {
		return fmt.Errorf("volume %q not defined", x.Positional.VolumeName)
	}

	pv, err := gadget.LayoutVolume(x.Positional.GadgetRootDir, &vol, defaultConstraints)
	if err != nil {
		return fmt.Errorf("cannot lay out volume %q: %v", x.Positional.VolumeName, err)
	}

	// GPT schema uses a GPT header at the start of the image and a backup
	// header at the end. The backup header size is 34 * LBA size (512B)
	if pv.EffectiveSchema() == "gpt" {
		pv.Size += 34 * pv.SectorSize
	}

	dumpVolumeInfo(pv, nil)
	return nil
}

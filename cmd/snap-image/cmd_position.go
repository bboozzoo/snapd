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
	"path/filepath"

	"github.com/jessevdk/go-flags"

	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/i18n"
)

type cmdPositionVolume struct {
	Positional struct {
		PreparedRootDir string
		VolumeName      string
	} `positional-args:"yes" required:"yes"`
}

func init() {
	addCommand("position-volume",
		func() flags.Commander {
			return &cmdPositionVolume{}
		},
		map[string]string{},
		[]argDesc{{
			name: "<image-root-dir>",
			desc: i18n.G("Prepared image root directory"),
		}, {
			name: "<volume-name>",
			desc: i18n.G("Volume name"),
		}})
}

func showPositionedVolume(pv *gadget.PositionedVolume) {
	alignedInfo := func(size gadget.Size) string {
		rem := size % pv.SectorSize
		cnt := size / pv.SectorSize
		if rem == 0 {
			return fmt.Sprintf("%d sectors", cnt)
		} else {
			return fmt.Sprintf("%d sectors + %d bytes (unaligned)", cnt, rem)
		}
	}
	sizeInfo := func(size gadget.Size) string {
		sectorInfo := alignedInfo(size)

		if size < gadget.SizeMiB {
			return sectorInfo
		}
		return fmt.Sprintf("%s (%v MB)", sectorInfo, size>>20)
	}
	fmt.Fprintf(Stdout, "volume:\n")
	fmt.Fprintf(Stdout, "  sector-size: %v\n", pv.SectorSize)
	fmt.Fprintf(Stdout, "  size: %-20d      # %s\n", pv.Size, sizeInfo(pv.Size))
	fmt.Fprintf(Stdout, "  schema: %v\n", pv.EffectiveSchema())
	fmt.Fprintf(Stdout, "  structures:\n")
	for _, ps := range pv.PositionedStructure {
		fmt.Fprintf(Stdout, "     %v:\n", ps)
		fmt.Fprintf(Stdout, "       type: %v\n", ps.Type)
		fmt.Fprintf(Stdout, "       size: %-10d           # %s\n", ps.Size, sizeInfo(ps.Size))
		fmt.Fprintf(Stdout, "       start-offset: %-10d   # %s\n", ps.StartOffset, alignedInfo(ps.StartOffset))
		erole := ps.EffectiveRole()
		if erole == "" {
			erole = "<none>"
		}
		fmt.Fprintf(Stdout, "       effective-role: %v\n", erole)
		if ps.IsBare() {
			fmt.Fprintf(Stdout, "       content:\n")
			for _, pc := range ps.PositionedContent {
				fmt.Fprintf(Stdout, "       - image: %v\n", pc.Image)
				fmt.Fprintf(Stdout, "         size: %-10d         # %s\n", pc.Size, sizeInfo(pc.Size))
				fmt.Fprintf(Stdout, "         start-offset: %-10d # %s\n", pc.StartOffset, alignedInfo(pc.StartOffset))
			}
		} else {
			fmt.Fprintf(Stdout, "       filesystem: %v\n", ps.Filesystem)
			fmt.Fprintf(Stdout, "       filesystem-label: %v\n", ps.EffectiveFilesystemLabel())
		}
	}
}

func (x *cmdPositionVolume) Execute(args []string) error {

	gadgetRootDir := filepath.Join(x.Positional.PreparedRootDir, "gadget")
	rootfsDir := filepath.Join(x.Positional.PreparedRootDir, "image")

	gi, err := gadget.ReadInfo(gadgetRootDir, false)
	if err != nil {
		return err
	}

	vol, ok := gi.Volumes[x.Positional.VolumeName]
	if !ok {
		return fmt.Errorf("volume %q not defined", x.Positional.VolumeName)
	}

	autoAdd := len(gi.Volumes) == 1
	if err := setupSystemData(&vol, rootfsDir, autoAdd); err != nil {
		return err
	}

	pv, err := gadget.PositionVolume(gadgetRootDir, &vol, defaultConstraints)
	if err != nil {
		return fmt.Errorf("cannot position volume %q: %v", x.Positional.VolumeName, err)
	}

	if pv.EffectiveSchema() == gadget.GPT {
		// save space for backup GPT
		pv.Size += 34 * pv.SectorSize
	}

	showPositionedVolume(pv)
	return nil
}

// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2018 Canonical Ltd
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

package main_test

import (
	. "gopkg.in/check.v1"

	snap_image "github.com/snapcore/snapd/cmd/snap-image"
)

type positionSuite struct {
	CmdBaseTest

	dir string
}

var _ = Suite(&positionSuite{})

func (s *positionSuite) SetUpTest(c *C) {
	s.CmdBaseTest.SetUpTest(c)

	s.dir = c.MkDir()
}

func (s *positionSuite) TearDownTest(c *C) {
	s.CmdBaseTest.TearDownTest(c)
}

func (s *positionSuite) TestPositionHappy(c *C) {
	var gadgetYaml = `
volumes:
  vol-1:
    schema: gpt
    bootloader: u-boot
    structure:
      - name: boot
        type: bare
        offset: 1M
        size: 1M
        content:
          - image: foo.img
          - image: bar.img
            offset: 123
      - name: EFI System
        type: EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B
        filesystem: vfat
        filesystem-label: system-boot
        size: 10M
        content:
          - source: /content
            target: /
      - name: writable
        filesystem: ext4
        size: 10M
        type: 83,0FC63DAF-8483-4772-8E79-3D69D8477DE4
        role: system-data
`

	preparedDir := c.MkDir()

	makeDirectoryTree(c, preparedDir, []mockEntry{
		// unpacked gadget contents
		{name: "gadget/meta/gadget.yaml", content: gadgetYaml},
		{name: "gadget/foo.img", content: "foo.img"},
		{name: "gadget/bar.img", content: "bar.img"},
		// snaps
		{name: "image/var/lib/snapd/seed.yaml", content: "seed.yaml"},
	})

	rest, err := snap_image.Parser().ParseArgs([]string{"position-volume", preparedDir, "vol-1"})
	c.Assert(err, IsNil)
	c.Assert(rest, DeepEquals, []string{})

	c.Check(s.stdout.String(), Equals, `volume:
  sector-size: 512
  size: 23086080                  # 45090 sectors (22 MB)
  schema: gpt
  structures:
     #0 ("boot"):
       type: bare
       size: 1048576              # 2048 sectors (1 MB)
       start-offset: 1048576      # 2048 sectors
       effective-role: <none>
       content:
       - image: foo.img
         size: 7                  # 0 sectors + 7 bytes (unaligned)
         start-offset: 1048576    # 2048 sectors
       - image: bar.img
         size: 7                  # 0 sectors + 7 bytes (unaligned)
         start-offset: 1048699    # 2048 sectors + 123 bytes (unaligned)
     #1 ("EFI System"):
       type: EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B
       size: 10485760             # 20480 sectors (10 MB)
       start-offset: 2097152      # 4096 sectors
       effective-role: system-boot
       filesystem: vfat
       filesystem-label: system-boot
     #2 ("writable"):
       type: 83,0FC63DAF-8483-4772-8E79-3D69D8477DE4
       size: 10485760             # 20480 sectors (10 MB)
       start-offset: 12582912     # 24576 sectors
       effective-role: system-data
       filesystem: ext4
       filesystem-label: writable
`)
}

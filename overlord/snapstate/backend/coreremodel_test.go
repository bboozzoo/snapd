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

package backend_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/progress"
	"github.com/snapcore/snapd/release"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/snap/snaptest"
	"github.com/snapcore/snapd/systemd"
	"github.com/snapcore/snapd/testutil"
	"github.com/snapcore/snapd/timings"

	"github.com/snapcore/snapd/overlord/snapstate/backend"
)

type coreRemodelBackendSuite struct {
	be                backend.Backend
	systemctlRestorer func()
	perfTimings       *timings.Timings
	dir               string
}

var _ = Suite(&coreRemodelBackendSuite{})

func (s *coreRemodelBackendSuite) SetUpTest(c *C) {
	s.dir = c.MkDir()
	dirs.SetRootDir(c.MkDir())

	s.perfTimings = timings.New(nil)
	s.systemctlRestorer = systemd.MockSystemctl(func(cmd ...string) ([]byte, error) {
		return []byte("ActiveState=inactive\n"), nil
	})
}

func (s *coreRemodelBackendSuite) TearDownTest(c *C) {
	dirs.SetRootDir("")
	s.systemctlRestorer()
}

func (s *coreRemodelBackendSuite) TestUnlinkErrorOutOnNonSnapd(c *C) {
	const yaml = `name: not-snapd
type: app
`
	info := snaptest.MockSnap(c, yaml, &snap.SideInfo{Revision: snap.R(11)})

	err := s.be.UnlinkSnapdSnap(info, progress.Null)
	c.Assert(err, ErrorMatches, `cannot unlink snap "not-snapd": type app is unsupported by the backend`)
}

func (s *coreRemodelBackendSuite) TestUndoGeneratedWrappers(c *C) {
	restore := release.MockOnClassic(false)
	defer restore()
	restore = release.MockReleaseInfo(&release.OS{ID: "ubuntu"})
	defer restore()
	dirs.SetRootDir(s.dir)

	err := os.MkdirAll(dirs.SnapServicesDir, 0755)
	c.Assert(err, IsNil)
	err = os.MkdirAll(dirs.SnapUserServicesDir, 0755)
	c.Assert(err, IsNil)

	const yaml = `name: snapd
version: 1.0
type: snapd
`
	// units from the snapd snap
	snapdUnits := [][]string{
		// system services
		{"lib/systemd/system/snapd.service", "[Unit]\nExecStart=/usr/lib/snapd/snapd\n# X-Snapd-Snap: do-not-start"},
		{"lib/systemd/system/snapd.socket", "[Unit]\n[Socket]\nListenStream=/run/snapd.socket"},
		{"lib/systemd/system/snapd.snap-repair.timer", "[Unit]\n[Timer]\nOnCalendar=*-*-* 5,11,17,23:00"},
		// user services
		{"usr/lib/systemd/user/snapd.session-agent.service", "[Unit]\nExecStart=/usr/bin/snap session-agent"},
		{"usr/lib/systemd/user/snapd.session-agent.socket", "[Unit]\n[Socket]\nListenStream=%t/snap-session.socket"},
	}
	// all generated untis
	generatedSnapdUnits := append(snapdUnits,
		[]string{"usr-lib-snapd.mount", "mount unit"})

	toEtcUnitPath := func(p string) string {
		if strings.HasPrefix(p, "usr/lib/systemd/user") {
			return filepath.Join(dirs.SnapUserServicesDir, filepath.Base(p))
		}
		return filepath.Join(dirs.SnapServicesDir, filepath.Base(p))
	}

	info := snaptest.MockSnapWithFiles(c, yaml, &snap.SideInfo{Revision: snap.R(11)}, snapdUnits)

	reboot, err := s.be.LinkSnap(info, mockDev, nil, s.perfTimings)
	c.Assert(err, IsNil)
	c.Assert(reboot, Equals, false)

	// sanity checks
	c.Check(filepath.Join(dirs.SnapServicesDir, "snapd.service"), testutil.FileEquals,
		fmt.Sprintf("[Unit]\nExecStart=%s/usr/lib/snapd/snapd\n# X-Snapd-Snap: do-not-start", info.MountDir()))
	// expecting all generated untis to be present
	for _, entry := range generatedSnapdUnits {
		c.Check(toEtcUnitPath(entry[0]), testutil.FilePresent)
	}

	err = s.be.UnlinkSnapdSnap(info, nil)
	c.Assert(err, IsNil)

	// generated wrappers should be gone now
	for _, entry := range generatedSnapdUnits {
		c.Check(toEtcUnitPath(entry[0]), testutil.FileAbsent)
	}

	// unlink is idempotent
	err = s.be.UnlinkSnapdSnap(info, nil)
	c.Assert(err, IsNil)
}

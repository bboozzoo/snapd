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

package userd_test

import (
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"path/filepath"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/snap/snaptest"
	"github.com/snapcore/snapd/testutil"
	"github.com/snapcore/snapd/userd"
)

type autostartSuite struct {
	dir string
}

var _ = Suite(&autostartSuite{})

func (s *autostartSuite) SetUpTest(c *C) {
	s.dir = c.MkDir()
	dirs.SetRootDir(s.dir)
	snap.MockSanitizePlugsSlots(func(snapInfo *snap.Info) {})
}

func (s *autostartSuite) TearDownTest(c *C) {
	s.dir = c.MkDir()
	dirs.SetRootDir("/")
}

func (s *autostartSuite) TestFindExec(c *C) {
	allGood := `[Desktop Entry]
Exec=foo --bar
`
	allGoodWithFlags := `[Desktop Entry]
Exec=foo --bar %U %D +%s
`
	noExec := `[Desktop Entry]
Type=Application
`
	emptyExec := `[Desktop Entry]
Exec=
`
	onlySpacesExec := `[Desktop Entry]
Exec=
`
	for i, tc := range []struct {
		in  string
		out string
		err string
	}{{
		in:  allGood,
		out: "foo --bar",
	}, {
		in:  noExec,
		err: "Exec not found or invalid",
	}, {
		in:  emptyExec,
		err: "Exec not found or invalid",
	}, {
		in:  onlySpacesExec,
		err: "Exec not found or invalid",
	}, {
		in:  allGoodWithFlags,
		out: "foo --bar   +%s",
	}} {
		c.Logf("tc %d", i)

		cmd, err := userd.FindExec([]byte(tc.in))
		if tc.err != "" {
			c.Check(cmd, Equals, "")
			c.Check(err, ErrorMatches, tc.err)
		} else {
			c.Check(err, IsNil)
			c.Check(cmd, Equals, tc.out)
		}
	}
}

var mockYaml = []byte(`name: snapname
version: 1.0
apps:
 foo:
  command: run-app
  autostart: foo-stable.desktop
`)

func (s *autostartSuite) TestTryAutostartAppValid(c *C) {
	userDir := path.Join(s.dir, "home")
	autostartDir := path.Join(userDir, ".config", "autostart")

	si := snaptest.MockSnap(c, string(mockYaml), &snap.SideInfo{
		Revision: snap.R("x2"),
	})
	err := os.Symlink(si.MountDir(), filepath.Join(si.MountDir(), "../current"))
	c.Assert(err, IsNil)

	appWrapperPath := si.Apps["foo"].WrapperPath()
	err = os.MkdirAll(filepath.Dir(appWrapperPath), 0755)
	c.Assert(err, IsNil)

	appCmd := testutil.MockCommand(c, appWrapperPath, "")
	defer appCmd.Restore()

	userd.MockUserCurrent(func() (*user.User, error) {
		return &user.User{HomeDir: userDir}, nil
	})

	err = os.MkdirAll(autostartDir, 0755)
	c.Assert(err, IsNil)

	fooDesktopFile := filepath.Join(autostartDir, "foo-stable.desktop")
	err = ioutil.WriteFile(fooDesktopFile,
		[]byte(`[Desktop Entry]
Exec=this-is-ignored -a -b --foo="a b c" -z "dev"
`), 0644)
	c.Assert(err, IsNil)

	cmd, err := userd.TryAutostartApp("snapname", fooDesktopFile)
	c.Assert(err, IsNil)
	c.Assert(cmd.Path, Equals, appWrapperPath)

	cmd.Wait()
	c.Assert(appCmd.Calls(), DeepEquals,
		[][]string{
			{
				filepath.Base(appWrapperPath),
				"-a",
				"-b",
				"--foo=a b c",
				"-z",
				"dev",
			},
		})
}

func (s *autostartSuite) TestTryAutostartAppNoMatchingApp(c *C) {
	userDir := path.Join(s.dir, "home")
	autostartDir := path.Join(userDir, ".config", "autostart")

	si := snaptest.MockSnap(c, string(mockYaml), &snap.SideInfo{
		Revision: snap.R("x2"),
	})
	err := os.Symlink(si.MountDir(), filepath.Join(si.MountDir(), "../current"))
	c.Assert(err, IsNil)

	userd.MockUserCurrent(func() (*user.User, error) {
		return &user.User{HomeDir: userDir}, nil
	})

	err = os.MkdirAll(autostartDir, 0755)
	c.Assert(err, IsNil)

	fooDesktopFile := filepath.Join(autostartDir, "foo-no-match.desktop")
	err = ioutil.WriteFile(fooDesktopFile,
		[]byte(`[Desktop Entry]
Exec=this-is-ignored -a -b --foo="a b c" -z "dev"
`), 0644)
	c.Assert(err, IsNil)

	cmd, err := userd.TryAutostartApp("snapname", fooDesktopFile)
	c.Assert(cmd, IsNil)
	c.Assert(err, ErrorMatches, "failed to determine startup command: Exec not found or invalid")
}

func (s *autostartSuite) TestTryAutostartAppNoSnap(c *C) {
	userDir := path.Join(s.dir, "home")
	autostartDir := path.Join(userDir, ".config", "autostart")

	userd.MockUserCurrent(func() (*user.User, error) {
		return &user.User{HomeDir: userDir}, nil
	})

	err := os.MkdirAll(autostartDir, 0755)
	c.Assert(err, IsNil)

	fooDesktopFile := filepath.Join(autostartDir, "foo-stable.desktop")
	err = ioutil.WriteFile(fooDesktopFile,
		[]byte(`[Desktop Entry]
Exec=this-is-ignored -a -b --foo="a b c" -z "dev"
`), 0644)
	c.Assert(err, IsNil)

	cmd, err := userd.TryAutostartApp("snapname", fooDesktopFile)
	c.Assert(cmd, IsNil)
	c.Assert(err, ErrorMatches, `failed to obtain snap information for snap "snapname".*`)
}

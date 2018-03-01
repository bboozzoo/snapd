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
	"fmt"
	"io/ioutil"
	"path/filepath"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/userd"
)

type autostartSuite struct {
	dir string
}

var _ = Suite(&autostartSuite{})

func (s *autostartSuite) SetUpTest(c *C) {
	s.dir = c.MkDir()
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
		path := filepath.Join(s.dir, fmt.Sprintf("tc-%d.desktop", i))
		err := ioutil.WriteFile(path, []byte(tc.in), 0600)
		c.Assert(err, IsNil)

		cmd, err := userd.FindExec(path)
		if tc.err != "" {
			c.Check(cmd, Equals, "")
			c.Check(err, ErrorMatches, tc.err)
		} else {
			c.Check(err, IsNil)
			c.Check(cmd, Equals, tc.out)
		}
	}
}

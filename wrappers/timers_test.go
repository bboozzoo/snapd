// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2018 Canonical Ltd
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

package wrappers_test

import (
	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/timeutil"
	"github.com/snapcore/snapd/wrappers"
)

type timersTestSuite struct{}

var _ = Suite(&timersTestSuite{})

func (s *timersTestSuite) TestTimer(c *C) {
	for _, t := range []struct {
		in       string
		expected string
	}{
		{"9:00-11:00,,20:00-22:00", "*-*-* 9..11:00:00..11:00:00"},
		{"mon,10:00,,fri,15:00", ""},
		{"mon1,10:00", ""},
		{"mon,10:00~12:00,,fri,15:00", ""},
	} {
		c.Logf("trying %+v", t)

		schedule, err := timeutil.ParseSchedule(t.in)
		c.Assert(err, IsNil)
		timer, err := wrappers.GenTimer(schedule)
		c.Check(err, IsNil)
		c.Check(timer, Equals, t.expected)
	}
}

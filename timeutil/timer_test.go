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

package timeutil_test

import (
	"time"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/timeutil"
)

type timerSuite struct {
	timersDir string
}

var _ = Suite(&timerSuite{})

func (ts *timerSuite) SetUpTest(c *C) {
	ts.timersDir = c.MkDir()
}

func (ts *timerSuite) TestExpire(c *C) {
	pt, err := timeutil.NewPersistentTimer("foo", "bad-schedule", ts.timersDir)
	c.Assert(pt, IsNil)
	c.Assert(err, ErrorMatches, `failed to parse timer schedule: cannot parse "bad-schedule": "bad" is not a valid weekday`)

	pt, err = timeutil.NewPersistentTimer("foo", "10:00-12:00", ts.timersDir)
	c.Assert(err, IsNil)
	c.Assert(pt, NotNil)
	c.Check(pt.PlannedNext().IsZero(), Equals, true)
	c.Check(pt.Last().IsZero(), Equals, true)
	c.Check(pt.Timer, Equals, "10:00-12:00")
	c.Check(pt.Schedule, NotNil)

	now := time.Now()
	// today, 9:55
	fakeNow := time.Date(now.Year(), now.Month(), now.Day(), 9, 55, 0, 0, now.Location())

	restore := timeutil.MockTimeNow(func() time.Time { return fakeNow })
	defer restore()

	pt.Expire(fakeNow)

	c.Check(pt.Last(), Equals, fakeNow)
	c.Check(pt.LastUTC, Equals, fakeNow.UTC())

	c.Check(pt.Next(fakeNow).Sub(fakeNow), Equals, 5*time.Minute)
	c.Check(pt.PlannedNext(), Equals, pt.Next(fakeNow))
}

func (ts *timerSuite) TestSaveRestore(c *C) {
	pt, err := timeutil.NewPersistentTimer("foo", "10:00-12:00", ts.timersDir)

	pt.Expire(time.Now())
	err = pt.Save()
	c.Assert(err, IsNil)

	otherPt, err := timeutil.PersistentTimerFromStorage("foo", ts.timersDir)
	c.Assert(err, IsNil)

	c.Check(otherPt, DeepEquals, pt)
}

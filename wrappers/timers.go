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

package wrappers

import (
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/timeutil"
)

func AddSnapTimers(info *snap.Info) error {
	return nil
}

func generateTimerSchedules(timer string) ([]string, error) {

	_, err := timeutil.ParseSchedule(timer)
	if err != nil {
		return nil, err
	}

	// TODO: fixed schedule, every 10 minutes
	return []string{"*-*-* *:00,10,20,30,40,50:00"}, nil
}

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

package release

import (
	"sort"
	"strings"
	"sync"
)

var secCompProbe = &secCompProber{}

// SecCompActions returns a sorted list of seccomp actions like
// []string{"allow", "errno", "kill", "log", "trace", "trap"}.
func SecCompActions() []string {
	return secCompProbe.probe()
}

func SecCompSupportsAction(action string) bool {
	actions := SecCompActions()
	i := sort.SearchStrings(actions, action)
	if i < len(actions) && actions[i] == action {
		return true
	}
	return false
}

// probing

type secCompProber struct {
	actions []string

	once sync.Once
}

func (scp *secCompProber) probe() []string {
	scp.once.Do(func() {
		scp.actions = probeSecCompActions()
	})
	return scp.actions
}

func probeSecCompActions() []string {
	contents, err := ioutilReadFile("/proc/sys/kernel/seccomp/actions_avail")
	if err != nil {
		return []string{}
	}
	actions := strings.Split(strings.TrimRight(string(contents), "\n"), " ")
	sort.Strings(actions)
	return actions
}

// mocking

func MockSecCompActions(actions []string) (restore func()) {
	old := secCompProbe
	secCompProbe = &secCompProber{
		actions: actions,
	}
	secCompProbe.once.Do(func() {})
	return func() {
		secCompProbe = old
	}
}

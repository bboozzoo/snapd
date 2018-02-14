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
package timeutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/snapcore/snapd/osutil"
)

// PersistentTimer is a timer which records its state information in persistent
// storage
type PersistentTimer struct {
	location       string
	Name           string
	Timer          string
	LastUTC        time.Time
	PlannedNextUTC time.Time
	Schedule       []*Schedule `json:"-"`
}

// NewPersistentTimer creates a new persistent timer with the name, timer
// schedule and given storage location
func NewPersistentTimer(name, timer, dir string) (*PersistentTimer, error) {
	schedule, err := ParseSchedule(timer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timer schedule: %v", err)
	}

	p := PersistentTimer{
		Name:     name,
		Timer:    timer,
		location: filepath.Join(dir, name),
		Schedule: schedule,
	}
	return &p, nil
}

// PersistentTimerFromStorage restores the persistent timer from its storage location
func PersistentTimerFromStorage(name, dir string) (*PersistentTimer, error) {
	timerFile := filepath.Join(dir, name)

	f, err := os.Open(timerFile)
	if err != nil {
		return nil, fmt.Errorf("cannot load timer %q information: %v", name, err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)

	p := PersistentTimer{}
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("failed to decode timer %q information for %v: %v", name, err)
	}

	p.Schedule, err = ParseSchedule(p.Timer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timer %q schedule: %v", name, err)
	}

	p.Name = name
	p.location = timerFile

	return &p, nil
}

// Expire notifies the timer that it has expired now and updates its state information.
func (p *PersistentTimer) Expire(now time.Time) {
	p.LastUTC = now.UTC()
	p.PlannedNextUTC = p.Next(now).UTC()
}

// PlannedNext returns when the timer was planned to expire next, converted to local timezone
func (p *PersistentTimer) PlannedNext() time.Time {
	return p.PlannedNextUTC.Local()
}

// Next returns the time when the timer expires next given now as expiration
// time, converted to local timezone
func (p *PersistentTimer) Next(now time.Time) time.Time {
	return now.Add(Next(p.Schedule, now))
}

// Last returns when the timer expired last, converted to local timezone
func (p *PersistentTimer) Last() time.Time {
	return p.LastUTC.Local()
}

// Save writes the timer state data to persistent storage
func (p *PersistentTimer) Save() error {
	f, err := osutil.NewAtomicFile(p.location, 0644, 0, osutil.NoChown, osutil.NoChown)
	if err != nil {
		return fmt.Errorf("failed to open storage for timer %q: %v", p.Name, err)
	}
	defer f.Cancel()

	enc := json.NewEncoder(f)

	if err := enc.Encode(p); err != nil {
		return fmt.Errorf("failed to encode persistent timer %q data: %v", p.Name, err)
	}

	return f.Commit()
}

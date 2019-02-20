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

package main

import (
	"fmt"
	"strings"

	"github.com/jessevdk/go-flags"

	"github.com/snapcore/snapd/client"
	"github.com/snapcore/snapd/i18n"
)

type cmdSlots struct {
	clientMixin
	All         bool `long:"all"`
	Positionals struct {
		Snap installedSnapName
	} `positional-args:"true"`
}

var shortslotsHelp = i18n.G("List slots")
var longSlotsHelp = i18n.G(`
The connections command lists slots in the system.
`)

func init() {
	addCommand("slots", shortslotsHelp, longSlotsHelp, func() flags.Commander {
		return &cmdSlots{}
	}, nil, []argDesc{{
		// TRANSLATORS: This needs to be wrapped in <>s.
		name: "<snap>",
		// TRANSLATORS: This should not start with a lowercase letter.
		desc: i18n.G("Constrain listing to a specific snap"),
	}})
}

func isSystemSnap(snap string) bool {
	return snap == "core" || snap == "snapd" || snap == "system"
}

func endpoint(snap, name string) string {
	if isSystemSnap(snap) {
		return ":" + name
	}
	return snap + ":" + name
}

func wantedSnapMatches(name, wanted string) bool {
	if wanted == "system" {
		if isSystemSnap(name) {
			return true
		}
		return false
	}
	return wanted == name
}

type connectionNotes struct {
	slot   string
	plug   string
	manual bool
	gadget bool
}

func (cn connectionNotes) String() string {
	opts := []string{}
	if cn.manual {
		opts = append(opts, "manual")
	}
	if cn.gadget {
		opts = append(opts, "gadget")
	}
	if len(opts) == 0 {
		return "-"
	}
	return strings.Join(opts, ",")
}

func connName(conn client.Connection) string {
	return endpoint(conn.Plug.Snap, conn.Plug.Name) + " " + endpoint(conn.Slot.Snap, conn.Slot.Name)
}

func (x *cmdSlots) Execute(args []string) error {
	if len(args) > 0 {
		return ErrExtraArgs
	}

	opts := client.ConnectionOptions{
		All: true,
	}
	wanted := string(x.Positionals.Snap)
	if wanted != "" {
		if x.All {
			// passing a snap name already implies --all, error out
			// when it was passed explicitly
			return fmt.Errorf(i18n.G("cannot use --all with snap name"))
		}
		// when asking for a single snap, include its disconnected plugs
		// and slots
		opts.Snap = wanted
	}

	connections, err := x.client.Connections(&opts)
	if err != nil {
		return err
	}
	if len(connections.Plugs) == 0 && len(connections.Slots) == 0 {
		return nil
	}

	w := tabWriter()
	fmt.Fprintln(w, i18n.G("Slot\tInterface"))

	for _, slot := range connections.Slots {
		if wanted != "" && !wantedSnapMatches(slot.Snap, wanted) {
			continue
		}
		sname := endpoint(slot.Snap, slot.Name)
		fmt.Fprintf(w, "%s\t%s\t\n", sname, slot.Interface)
	}

	w.Flush()
	return nil
}

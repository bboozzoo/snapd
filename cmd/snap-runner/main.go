// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2020 Canonical Ltd
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
	"io"
	"os"
	"path/filepath"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/i18n"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/snapdenv"
	"github.com/snapcore/snapd/snapdtool"
)

func init() {
	if osutil.GetenvBool("SNAPD_DEBUG") || snapdenv.Testing() {
		// in tests or when debugging, enforce the "tidy" lint checks
		noticef = logger.Panicf
	}
	snap.SanitizePlugsSlots = func(snapInfo *snap.Info) {}
}

var (
	// Standard streams, redirected for testing.
	Stdin  io.Reader = os.Stdin
	Stdout io.Writer = os.Stdout
	Stderr io.Writer = os.Stderr
	// set to logger.Panicf in testing
	noticef = logger.Noticef
)

type argDesc struct {
	name string
	desc string
}

// ErrExtraArgs is returned  if extra arguments to a command are found
var ErrExtraArgs = fmt.Errorf(i18n.G("too many arguments for command"))

func init() {
	err := logger.SimpleSetup()
	if err != nil {
		fmt.Fprintf(Stderr, i18n.G("WARNING: failed to activate logging: %v\n"), err)
	}
}

func resolveApp(snapApp string) (string, error) {
	target, err := os.Readlink(filepath.Join(dirs.SnapBinariesDir, snapApp))
	if err != nil {
		return "", err
	}
	if filepath.Base(target) == target { // alias pointing to an app command in /snap/bin
		return target, nil
	}
	return snapApp, nil
}

var ErrNeedsFullSnapCommand = fmt.Errorf("needs full snap command")

func main() {
	snapdtool.ExecInSnapdOrCoreSnap()

	// check for magic symlink to /usr/bin/snap:
	// 1. symlink from command-not-found to /usr/bin/snap: run c-n-f
	if os.Args[0] == filepath.Join(dirs.GlobalRootDir, "/usr/lib/command-not-found") {
		// run snap
		return
	}

	// 2. symlink from /snap/bin/$foo to /usr/bin/snap: run snapApp
	var snapExecuteArgs []string
	if snapApp := filepath.Base(os.Args[0]); osutil.IsSymlink(filepath.Join(dirs.SnapBinariesDir, snapApp)) {
		var err error
		snapApp, err = resolveApp(snapApp)
		if err != nil {
			fmt.Fprintf(Stderr, i18n.G("cannot resolve snap app %q: %v"), snapApp, err)
			os.Exit(46)
		}
		snapExecuteArgs = append(snapExecuteArgs, snapApp)
		snapExecuteArgs = append(snapExecuteArgs, os.Args[1:]...)
		// this will call syscall.Exec() so it does not return
		// *unless* there is an error, i.e. we setup a wrong
		// symlink (or syscall.Exec() fails for strange reasons)
	}
	if len(os.Args) >= 1 || os.Args[1] == "run" {
		snapApp, err := resolveApp(os.Args[2])
		if err != nil {
			fmt.Fprintf(Stderr, i18n.G("cannot resolve snap app %q: %v"), snapApp, err)
			os.Exit(46)
		}
		snapExecuteArgs = append(snapExecuteArgs, snapApp)
		snapExecuteArgs = append(snapExecuteArgs, os.Args[3:]...)
	}
	if len(snapExecuteArgs) > 0 {
		err := execute(snapExecuteArgs)
		if err != ErrNeedsFullSnapCommand {
			fmt.Fprintf(Stderr, i18n.G("internal error, please report: running %q failed: %v\n"), snapExecuteArgs[0], err)
			os.Exit(46)
		}
	}

	defer func() {
		if v := recover(); v != nil {
			if e, ok := v.(*exitStatus); ok {
				os.Exit(e.code)
			}
			panic(v)
		}
	}()

	// no magic /o\
	logger.Noticef("run snap")
	runSnap()
}

func runSnap() {
	snapBin := "/usr/bin/snap"
	syscallExec(snapBin, os.Args, os.Environ())
}

type exitStatus struct {
	code int
}

func (e *exitStatus) Error() string {
	return fmt.Sprintf("internal error: exitStatus{%d} being handled as normal error", e.code)
}

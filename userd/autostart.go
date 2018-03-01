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

package userd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/snap"
)

var (
	replacedDesktopKeys = []string{"%f", "%F", "%u", "%U", "%d", "%D",
		"%n", "%N", "%i", "%c", "%k", "%v", "%m"}
)

func findExec(desktopFilePath string) (string, error) {
	f, err := os.Open(desktopFilePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	execCmd := ""
	for scanner.Scan() {
		bline := scanner.Bytes()

		if !bytes.HasPrefix(bline, []byte("Exec=")) {
			continue
		}

		execCmd = strings.Split(string(bline), "Exec=")[1]
		for _, key := range replacedDesktopKeys {
			execCmd = strings.Replace(execCmd, key, "", -1)
		}
		return execCmd, nil
	}

	if execCmd == "" {
		return "", fmt.Errorf("Exec not found")
	}
	return "", nil
}

func getCurrentSnapInfo(snapName string) (*snap.Info, error) {
	curFn := filepath.Join(dirs.SnapMountDir, snapName, "current")
	realFn, err := os.Readlink(curFn)
	if err != nil {
		return nil, fmt.Errorf("cannot find current revision for snap %s: %s", snapName, err)
	}
	rev := filepath.Base(realFn)
	revision, err := snap.ParseRevision(rev)
	if err != nil {
		return nil, fmt.Errorf("cannot read revision %s: %s", rev, err)
	}

	return snap.ReadInfo(snapName, &snap.SideInfo{Revision: revision})
}

func tryAutostartApp(snapName, desktopFilePath string) error {
	desktopFile := filepath.Base(desktopFilePath)

	info, err := getCurrentSnapInfo(snapName)
	if err != nil {
		return fmt.Errorf("failed to obtain snap information for snap %q: %v", snapName, err)
	}

	var app *snap.AppInfo
	for _, candidate := range info.Apps {
		if candidate.Autostart == desktopFile {
			app = candidate
			break
		}
	}

	if app == nil {
		return fmt.Errorf("could not match desktop file %v with an app in snap %q", desktopFile, snapName)
	}

	// use the sanitized desktop file
	command, err := findExec(desktopFilePath)
	if err != nil {
		return fmt.Errorf("failed to determine startup command: %v", err)
	}
	logger.Debugf("exec line: %v", command)

	pos := strings.Index(command, app.Command)
	if pos == -1 {
		return fmt.Errorf("startup command does not match app %q from snap %q", app.Name, snapName)
	}

	args := command[pos+len(app.Command):]
	logger.Debugf(`remaining args: "%v"`, args)

	// TODO: shlex
	args = strings.TrimSpace(args)
	split := strings.Split(args, " ")
	cmd := exec.Command(app.WrapperPath(), split...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to autostart %q: %v", app.Name, err)
	}
	return nil
}

type failedAutostartError map[string]error

func (f failedAutostartError) Error() string {
	var out bytes.Buffer
	for app, err := range f {
		fmt.Fprintf(&out, "- %q: %v\n", app, err)
	}
	return out.String()
}

// AutostartSessionApps starts applications which have placed their desktop
// files in $SNAP_USER_DATA/.config/autostart
func AutostartSessionApps() error {
	usr, err := user.Current()
	if err != nil {
		return err
	}

	usrSnapDir := filepath.Join(usr.HomeDir, "snap")

	glob := filepath.Join(usrSnapDir, "*/current/.config/autostart/*.desktop")
	matches, err := filepath.Glob(glob)
	if err != nil {
		return err
	}

	failedApps := make(failedAutostartError)
	for _, desktopFilePath := range matches {
		desktopFile := filepath.Base(desktopFilePath)
		logger.Debugf("autostart desktop file %v", desktopFile)

		// /home/foo/snap/some-snap/current/.config/autostart/some-app.desktop ->
		//    some-snap/current/.config/autostart/some-app.desktop
		noHomePrefix := strings.TrimPrefix(desktopFilePath, usrSnapDir+"/")
		// some-snap/current/.config/autostart/some-app.desktop -> some-snap
		snapName := noHomePrefix[0:strings.IndexByte(noHomePrefix, '/')]

		logger.Debugf("snap name: %q", snapName)

		if err := tryAutostartApp(snapName, desktopFilePath); err != nil {
			logger.Debugf("error encountered when trying to autostart %v for snap %q: %v", desktopFile, snapName, err)
			failedApps[desktopFile] = err
		}
	}
	if len(failedApps) > 0 {
		return failedApps
	}
	return nil
}

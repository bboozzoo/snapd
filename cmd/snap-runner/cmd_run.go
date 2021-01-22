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

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/i18n"
	"github.com/snapcore/snapd/interfaces"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/sandbox/cgroup"
	"github.com/snapcore/snapd/sandbox/selinux"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/snap/snapenv"
)

var (
	syscallExec              = syscall.Exec
	userCurrent              = user.Current
	osGetenv                 = os.Getenv
	timeNow                  = time.Now
	selinuxIsEnabled         = selinux.IsEnabled
	selinuxVerifyPathContext = selinux.VerifyPathContext
	selinuxRestoreContext    = selinux.RestoreContext
)

// isStopping returns true if the system is shutting down.
func isStopping() (bool, error) {
	// Make sure, just in case, that systemd doesn't localize the output string.
	env, err := osutil.OSEnvironment()
	if err != nil {
		return false, err
	}
	env["LC_MESSAGES"] = "C"
	// Check if systemd is stopping (shutting down or rebooting).
	cmd := exec.Command("systemctl", "is-system-running")
	cmd.Env = env.ForExec()
	stdout, err := cmd.Output()
	// systemctl is-system-running returns non-zero for outcomes other than "running"
	// As such, ignore any ExitError and just process the stdout buffer.
	if _, ok := err.(*exec.ExitError); ok {
		return string(stdout) == "stopping\n", nil
	}
	return false, err
}

func maybeWaitForSecurityProfileRegeneration() error {
	// check if the security profiles key has changed, if so, we need
	// to wait for snapd to re-generate all profiles
	mismatch, err := interfaces.SystemKeyMismatch()
	if err == nil && !mismatch {
		return nil
	}
	// something went wrong with the system-key compare, try to
	// reach snapd before continuing
	if err != nil {
		logger.Debugf("SystemKeyMismatch returned an error: %v", err)
	}

	// We have a mismatch but maybe it is only because systemd is shutting down
	// and core or snapd were already unmounted and we failed to re-execute.
	// For context see: https://bugs.launchpad.net/snapd/+bug/1871652
	stopping, err := isStopping()
	if err != nil {
		logger.Debugf("cannot check if system is stopping: %s", err)
	}
	if stopping {
		logger.Debugf("ignoring system key mismatch during system shutdown/reboot")
		return nil
	}

	// let full snap command sort it out
	logger.Noticef("system key mismatch detected...")
	return ErrNeedsFullSnapCommand
}

func execute(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf(i18n.G("need the application to run as argument"))
	}
	snapApp := args[0]
	args = args[1:]

	return snapRunApp(snapApp, args)
}

// antialias changes snapApp and args if snapApp is actually an alias
// for something else. If not, or if the args aren't what's expected
// for completion, it returns them unchanged.
func antialias(snapApp string, args []string) (string, []string) {
	if len(args) < 7 {
		// NOTE if len(args) < 7, Something is Wrong (at least WRT complete.sh and etelpmoc.sh)
		return snapApp, args
	}

	actualApp, err := resolveApp(snapApp)
	if err != nil || actualApp == snapApp {
		// no alias! woop.
		return snapApp, args
	}

	compPoint, err := strconv.Atoi(args[2])
	if err != nil {
		// args[2] is not COMP_POINT
		return snapApp, args
	}

	if compPoint <= len(snapApp) {
		// COMP_POINT is inside $0
		return snapApp, args
	}

	if compPoint > len(args[5]) {
		// COMP_POINT is bigger than $#
		return snapApp, args
	}

	if args[6] != snapApp {
		// args[6] is not COMP_WORDS[0]
		return snapApp, args
	}

	// it _should_ be COMP_LINE followed by one of
	// COMP_WORDBREAKS, but that's hard to do
	re, err := regexp.Compile(`^` + regexp.QuoteMeta(snapApp) + `\b`)
	if err != nil || !re.MatchString(args[5]) {
		// (weird regexp error, or) args[5] is not COMP_LINE
		return snapApp, args
	}

	argsOut := make([]string, len(args))
	copy(argsOut, args)

	argsOut[2] = strconv.Itoa(compPoint - len(snapApp) + len(actualApp))
	argsOut[5] = re.ReplaceAllLiteralString(args[5], actualApp)
	argsOut[6] = actualApp

	return actualApp, argsOut
}

func getSnapInfo(snapName string, revision snap.Revision) (info *snap.Info, err error) {
	if revision.Unset() {
		info, err = snap.ReadCurrentInfo(snapName)
	} else {
		info, err = snap.ReadInfo(snapName, &snap.SideInfo{
			Revision: revision,
		})
	}

	return info, err
}

func createOrUpdateUserDataSymlink(info *snap.Info, usr *user.User) error {
	// 'current' symlink for user data (SNAP_USER_DATA)
	userData := info.UserDataDir(usr.HomeDir)
	wantedSymlinkValue := filepath.Base(userData)
	currentActiveSymlink := filepath.Join(userData, "..", "current")

	var err error
	var currentSymlinkValue string
	for i := 0; i < 5; i++ {
		currentSymlinkValue, err = os.Readlink(currentActiveSymlink)
		// Failure other than non-existing symlink is fatal
		if err != nil && !os.IsNotExist(err) {
			// TRANSLATORS: %v the error message
			return fmt.Errorf(i18n.G("cannot read symlink: %v"), err)
		}

		if currentSymlinkValue == wantedSymlinkValue {
			break
		}

		if err == nil {
			// We may be racing with other instances of snap-run that try to do the same thing
			// If the symlink is already removed then we can ignore this error.
			err = os.Remove(currentActiveSymlink)
			if err != nil && !os.IsNotExist(err) {
				// abort with error
				break
			}
		}

		err = os.Symlink(wantedSymlinkValue, currentActiveSymlink)
		// Error other than symlink already exists will abort and be propagated
		if err == nil || !os.IsExist(err) {
			break
		}
		// If we arrived here it means the symlink couldn't be created because it got created
		// in the meantime by another instance, so we will try again.
	}
	if err != nil {
		return fmt.Errorf(i18n.G("cannot update the 'current' symlink of %q: %v"), currentActiveSymlink, err)
	}
	return nil
}

func createUserDataDirs(info *snap.Info) error {
	// Adjust umask so that the created directories have the permissions we
	// expect and are unaffected by the initial umask. While go runtime creates
	// threads at will behind the scenes, the setting of umask applies to the
	// entire process so it doesn't need any special handling to lock the
	// executing goroutine to a single thread.
	oldUmask := syscall.Umask(0)
	defer syscall.Umask(oldUmask)

	usr, err := userCurrent()
	if err != nil {
		return fmt.Errorf(i18n.G("cannot get the current user: %v"), err)
	}

	// see snapenv.User
	instanceUserData := info.UserDataDir(usr.HomeDir)
	instanceCommonUserData := info.UserCommonDataDir(usr.HomeDir)
	createDirs := []string{instanceUserData, instanceCommonUserData}
	if info.InstanceKey != "" {
		// parallel instance snaps get additional mapping in their mount
		// namespace, namely /home/joe/snap/foo_bar ->
		// /home/joe/snap/foo, make sure that the mount point exists and
		// is owned by the user
		snapUserDir := snap.UserSnapDir(usr.HomeDir, info.SnapName())
		createDirs = append(createDirs, snapUserDir)
	}
	for _, d := range createDirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			// TRANSLATORS: %q is the directory whose creation failed, %v the error message
			return fmt.Errorf(i18n.G("cannot create %q: %v"), d, err)
		}
	}

	if err := createOrUpdateUserDataSymlink(info, usr); err != nil {
		return err
	}

	return maybeRestoreSecurityContext(usr)
}

// maybeRestoreSecurityContext attempts to restore security context of ~/snap on
// systems where it's applicable
func maybeRestoreSecurityContext(usr *user.User) error {
	snapUserHome := filepath.Join(usr.HomeDir, dirs.UserHomeSnapDir)
	enabled, err := selinuxIsEnabled()
	if err != nil {
		return fmt.Errorf("cannot determine SELinux status: %v", err)
	}
	if !enabled {
		logger.Debugf("SELinux not enabled")
		return nil
	}

	match, err := selinuxVerifyPathContext(snapUserHome)
	if err != nil {
		return fmt.Errorf("failed to verify SELinux context of %v: %v", snapUserHome, err)
	}
	if match {
		return nil
	}
	logger.Noticef("restoring default SELinux context of %v", snapUserHome)

	if err := selinuxRestoreContext(snapUserHome, selinux.RestoreMode{Recursive: true}); err != nil {
		return fmt.Errorf("cannot restore SELinux context of %v: %v", snapUserHome, err)
	}
	return nil
}

func snapRunApp(snapApp string, args []string) error {
	snapName, appName := snap.SplitSnapApp(snapApp)
	info, err := getSnapInfo(snapName, snap.R(0))
	if err != nil {
		return err
	}

	app := info.Apps[appName]
	if app == nil {
		return fmt.Errorf(i18n.G("cannot find app %q in %q"), appName, snapName)
	}

	return runSnapConfine(info, app.SecurityTag(), snapApp, args)
}

var osReadlink = os.Readlink

// snapdHelperPath return the path of a helper like "snap-confine" or
// "snap-exec" based on if snapd is re-execed or not
func snapdHelperPath(toolName string) (string, error) {
	exe, err := osReadlink("/proc/self/exe")
	if err != nil {
		return "", fmt.Errorf("cannot read /proc/self/exe: %v", err)
	}
	// no re-exec
	if !strings.HasPrefix(exe, dirs.SnapMountDir) {
		return filepath.Join(dirs.DistroLibExecDir, toolName), nil
	}
	// The logic below only works if the last two path components
	// are /usr/bin
	// FIXME: use a snap warning?
	if !strings.HasSuffix(exe, "/usr/bin/"+filepath.Base(exe)) {
		logger.Noticef("(internal error): unexpected exe input in snapdHelperPath: %v", exe)
		return filepath.Join(dirs.DistroLibExecDir, toolName), nil
	}
	// snapBase will be "/snap/{core,snapd}/$rev/" because
	// the snap binary is always at $root/usr/bin/snap
	snapBase := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", ".."))
	// Run snap-confine from the core/snapd snap.  The tools in
	// core/snapd snap are statically linked, or mostly
	// statically, with the exception of libraries such as libudev
	// and libc.
	return filepath.Join(snapBase, dirs.CoreLibExecDir, toolName), nil
}

func needsXauthority() bool {
	xauthPath := osGetenv("XAUTHORITY")
	return len(xauthPath) != 0 || osutil.FileExists(xauthPath)
}

func needsDocumentPortal(info *snap.Info, snapApp string) bool {
	// only check whether the app or hook plugs the desktop interface
	_, appName := snap.SplitSnapApp(snapApp)
	for _, plug := range info.Apps[appName].Plugs {
		if plug.Interface == "desktop" {
			return true
		}
	}
	return false
}

type envForExecFunc func(extra map[string]string) []string

func runSnapConfine(info *snap.Info, securityTag, snapApp string, args []string) error {
	if needsXauthority() {
		// run full snap
		return ErrNeedsFullSnapCommand
	}

	if needsDocumentPortal(info, snapApp) {
		// run full snap
		return ErrNeedsFullSnapCommand
	}
	if info.NeedsClassic() {
		// run full snap
		return ErrNeedsFullSnapCommand
	}

	snapConfine, err := snapdHelperPath("snap-confine")
	if err != nil {
		return err
	}
	if !osutil.FileExists(snapConfine) {
		return fmt.Errorf(i18n.G("missing snap-confine: try updating your core/snapd package"))
	}

	if err := createUserDataDirs(info); err != nil {
		logger.Noticef("WARNING: cannot create user data directory: %s", err)
	}

	cmd := []string{snapConfine}
	// this should never happen since we validate snaps with "base: none" and do not allow hooks/apps
	if info.Base == "none" {
		return fmt.Errorf(`cannot run hooks / applications with base "none"`)
	}
	logger.Debugf("base: %q", info.Base)
	if info.Base != "" {
		cmd = append(cmd, "--base", info.Base)
	} else {
		if info.Type() == snap.TypeKernel {
			// can't snapd pass this?
			return ErrNeedsFullSnapCommand
		}
	}

	cmd = append(cmd, securityTag)

	// when under confinement, snap-exec is run from 'core' snap rootfs
	snapExecPath := filepath.Join(dirs.CoreLibExecDir, "snap-exec")

	cmd = append(cmd, snapExecPath)

	// snap-exec is POSIXly-- options must come before positionals.
	cmd = append(cmd, snapApp)
	cmd = append(cmd, args...)

	env, err := osutil.OSEnvironment()
	if err != nil {
		return err
	}
	snapenv.ExtendEnvForRun(env, info)

	// on each run variant path this will be used once to get
	// the environment plus additions in the right form
	envForExec := func(extra map[string]string) []string {
		for varName, value := range extra {
			env[varName] = value
		}
		if !info.NeedsClassic() {
			return env.ForExec()
		}
		// For a classic snap, environment variables that are
		// usually stripped out by ld.so when starting a
		// setuid process are presevered by being renamed by
		// prepending PreservedUnsafePrefix -- which snap-exec
		// will remove, restoring the variables to their
		// original names.
		return env.ForExecEscapeUnsafe(snapenv.PreservedUnsafePrefix)
	}

	// Systemd automatically places services under a unique cgroup encoding the
	// security tag, but for apps and hooks we need to create a transient scope
	// with similar purpose ourselves.
	//
	// The way this happens is as follows:
	//
	// 1) Services are implemented using systemd service units. Starting a
	// unit automatically places it in a cgroup named after the service unit
	// name. Snapd controls the name of the service units thus indirectly
	// controls the cgroup name.
	//
	// 2) Non-services, including hooks, are started inside systemd
	// transient scopes. Scopes are a systemd unit type that are defined
	// programmatically and are meant for groups of processes started and
	// stopped by an _arbitrary process_ (ie, not systemd). Systemd
	// requires that each scope is given a unique name. We employ a scheme
	// where random UUID is combined with the name of the security tag
	// derived from snap application or hook name. Multiple concurrent
	// invocations of "snap run" will use distinct UUIDs.
	//
	// Transient scopes allow launched snaps to integrate into
	// the systemd design. See:
	// https://www.freedesktop.org/wiki/Software/systemd/ControlGroupInterface/
	//
	// Programs running as root, like system-wide services and programs invoked
	// using tools like sudo are placed under system.slice. Programs running as
	// a non-root user are placed under user.slice, specifically in a scope
	// specific to a logind session.
	//
	// This arrangement allows for proper accounting and control of resources
	// used by snap application processes of each type.
	//
	// For more information about systemd cgroups, including unit types, see:
	// https://www.freedesktop.org/wiki/Software/systemd/ControlGroupInterface/
	_, appName := snap.SplitSnapApp(snapApp)
	needsTracking := true
	if app := info.Apps[appName]; app != nil && app.IsService() {
		// If we are running a service app then we do not need to use
		// application tracking. Services, both in the system and user scope,
		// do not need tracking because systemd already places them in a
		// tracking cgroup, named after the systemd unit name, and those are
		// sufficient to identify both the snap name and the app name.
		needsTracking = false
	}
	// Allow using the session bus for all apps
	allowSessionBus := true
	// Track, or confirm existing tracking from systemd.
	var trackingErr error
	if needsTracking {
		opts := &cgroup.TrackingOptions{AllowSessionBus: allowSessionBus}
		trackingErr = cgroupCreateTransientScopeForTracking(securityTag, opts)
	} else {
		trackingErr = cgroupConfirmSystemdServiceTracking(securityTag)
	}
	if trackingErr != nil {
		if trackingErr != cgroup.ErrCannotTrackProcess {
			return trackingErr
		}
		// If we cannot track the process then log a debug message.
		// TODO: if we could, create a warning. Currently this is not possible
		// because only snapd can create warnings, internally.
		logger.Debugf("snapd cannot track the started application")
		logger.Debugf("snap refreshes will not be postponed by this process")
	}
	logger.Debugf("command: %q", cmd)
	return syscallExec(cmd[0], cmd, envForExec(nil))
}

var cgroupCreateTransientScopeForTracking = cgroup.CreateTransientScopeForTracking
var cgroupConfirmSystemdServiceTracking = cgroup.ConfirmSystemdServiceTracking

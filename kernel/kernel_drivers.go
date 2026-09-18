// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2024 Canonical Ltd
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

package kernel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/release"
	"github.com/snapcore/snapd/snap"
)

// For testing purposes
var osSymlink = os.Symlink

// doSync is a mockable wrapper around syscall.Sync, so tests can observe
// ordering/call-count without needing real disk durability.
var doSync = syscall.Sync

// kernelDriversTreeGeneratorVersion identifies the logic that produced a
// kernel drivers tree (the on-disk symlinks/files under
// <destDir>/lib/{modules,firmware}). It is written to <destDir>/snapd.meta
// after every successful build and compared against the current value on
// snapd startup so that a fix to this generation logic can be applied
// retroactively to an already-installed kernel snap, without requiring a
// kernel snap revision bump (see overlord/snapstate's
// check-kernel-drivers-tree task).
//
// IMPORTANT: bump this whenever a change to EnsureKernelDriversTree, or
// anything it calls (createModulesSubtree, createKernelModulesSymlinks,
// createFirmwareSymlinks, setupModsFromComp), could produce a different
// on-disk tree for the same inputs (same kernel snap content + same
// currently active kernel-modules components). Do NOT bump it for pure
// refactors with no behavior change — bumping is what forces every
// already-provisioned device to redo a (cheap, but not free) check, so it
// should only happen when the output can actually differ. Do NOT tie this
// to the snapd version/build-id: a snapd release with no changes to this
// code path must not force a fleet-wide recheck.
var kernelDriversTreeGeneratorVersion = 1

// driversTreeMeta is the content of the <destDir>/snapd.meta marker file
// written after every successful kernel drivers tree build.
type driversTreeMeta struct {
	GeneratorVersion int `json:"generator-version"`
}

func driversTreeMetaPath(destDir string) string {
	return filepath.Join(destDir, "snapd.meta")
}

// writeDriversTreeMeta records the generator version that produced destDir.
func writeDriversTreeMeta(destDir string) error {
	meta := driversTreeMeta{GeneratorVersion: kernelDriversTreeGeneratorVersion}
	data, err := json.Marshal(&meta)
	if err != nil {
		return err
	}
	return osutil.AtomicWriteFile(driversTreeMetaPath(destDir), data, 0644, 0)
}

// readDriversTreeGeneratorMeta returns the generator metadata recorded for
// destDir. If no marker value is present a default zero value is returned.
func readDriversTreeGeneratorMeta(destDir string) (driversTreeMeta, error) {
	data, err := os.ReadFile(driversTreeMetaPath(destDir))
	if errors.Is(err, fs.ErrNotExist) {
		return driversTreeMeta{}, nil
	}
	if err != nil {
		return driversTreeMeta{}, err
	}
	var meta driversTreeMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		// Treat unparseable metadata the same as missing: needs a check, or
		// could be corrupted?
		return driversTreeMeta{}, nil
	}
	return meta, nil
}

// DriversTreeNeedsCheck reports whether destDir's recorded generator
// version is strictly older than the version of the code currently
// running.
func DriversTreeNeedsCheck(destDir string) (bool, error) {
	v, err := readDriversTreeGeneratorMeta(destDir)
	if err != nil {
		return false, err
	}
	logger.Debugf("checking kernel tree generator version, current %v, on disk %v",
		kernelDriversTreeGeneratorVersion, v.GeneratorVersion)
	// Only care about older (lower) versions. The tree may have been build by a
	// newer snapd.
	return kernelDriversTreeGeneratorVersion > v.GeneratorVersion, nil
}

// We expect as a minimum something that starts with three numbers
// separated by dots for the kernel version.
var utsRelease = regexp.MustCompile(`^([0-9]+\.){2}[0-9]+`)

// KernelVersionFromModulesDir returns the kernel version for a mounted kernel
// snap (this would be the output if "uname -r" for a running kernel). It
// assumes that there is a folder named modules/$(uname -r) inside the snap.
func KernelVersionFromModulesDir(mountPoint string) (string, error) {
	modsDir := filepath.Join(mountPoint, "modules")
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		return "", err
	}

	kversion := ""
	for _, node := range entries {
		if !node.Type().IsDir() {
			continue
		}
		if !utsRelease.MatchString(node.Name()) {
			continue
		}
		if kversion != "" {
			return "", fmt.Errorf("more than one modules directory in %q", modsDir)
		}
		kversion = node.Name()
	}
	if kversion == "" {
		return "", fmt.Errorf("no modules directory found in %q", modsDir)
	}

	return kversion, nil
}

// firmwareSymlinkTarget describes the symlink entry that should exist for
// one entry found in a kernel/component mount's firmware/ directory.
type firmwareSymlinkTarget struct {
	name   string
	target string
}

// firmwareSymlinkTargets derives the symlinks that createFirmwareSymlinks
// (or the live per-entry sync used by Regenerate mode) needs to create for
// the given mount's firmware/ directory. It never returns the reserved
// "updates" entry, which is handled separately (component modules/firmware).
func firmwareSymlinkTargets(fwMount MountPoints, fwDest string) ([]firmwareSymlinkTarget, error) {
	fwOrig := fwMount.UnderCurrentPath("firmware")
	entries, err := os.ReadDir(fwOrig)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debugf("no firmware found in %q", fwOrig)
			return nil, nil
		}
		return nil, err
	}

	fwTarget := fwMount.UnderTargetPath("firmware")
	var targets []firmwareSymlinkTarget
	for _, node := range entries {
		switch node.Type() {
		case 0, fs.ModeDir:
			// "updates" is included in (latest) kernel snaps but
			// is empty, and we use if for firmware shipped in
			// components, so we ignore it.
			if node.Name() == "updates" {
				continue
			}
			// Create link for regular files or directories
			targets = append(targets, firmwareSymlinkTarget{
				name:   node.Name(),
				target: filepath.Join(fwTarget, node.Name()),
			})
		case fs.ModeSymlink:
			// Replicate link (it should be relative)
			// TODO check this in snap pack
			lpath := filepath.Join(fwDest, node.Name())
			dest, err := os.Readlink(filepath.Join(fwOrig, node.Name()))
			if err != nil {
				return nil, err
			}
			if filepath.IsAbs(dest) {
				return nil, fmt.Errorf("symlink %q points to absolute path %q", lpath, dest)
			}
			targets = append(targets, firmwareSymlinkTarget{name: node.Name(), target: dest})
		default:
			return nil, fmt.Errorf("%q has unexpected file type: %s",
				node.Name(), node.Type())
		}
	}

	return targets, nil
}

func createFirmwareSymlinks(fwMount MountPoints, fwDest string) error {
	if err := os.MkdirAll(fwDest, 0755); err != nil {
		return err
	}

	// Symbolic links inside firmware folder - it cannot be directly a
	// symlink to "firmware" as we will use firmware/updates/ subfolder for
	// components.
	targets, err := firmwareSymlinkTargets(fwMount, fwDest)
	if err != nil {
		return err
	}
	for _, t := range targets {
		lpath := filepath.Join(fwDest, t.name)
		if err := os.Symlink(t.target, lpath); err != nil {
			return err
		}
	}

	return nil
}

func createModulesSubtree(kMntPts MountPoints, kernelTree, kversion string, compsMntPts []ModulesCompMountPoints) error {
	// Although empty we need "lib" because "depmod" always appends
	// "/lib/modules/<kernel_version>" to the directory passed with option
	// "-b".
	modsRoot := filepath.Join(kernelTree, "lib", "modules", kversion)
	if err := os.MkdirAll(modsRoot, 0755); err != nil {
		return err
	}

	// Discover the content of the modules directory of the current mount
	// once. The target mount may not exist yet (for example, during
	// install/preseed the target is the future runtime mount), so it must
	// not be read for discovery; only the current mount is guaranteed to be
	// available. The discovered directory names are reused for both the
	// current and the target symlinks (see createKernelModulesSymlinks).
	currentMntDir := kMntPts.UnderCurrentPath("modules", kversion)
	entries, err := os.ReadDir(currentMntDir)
	if err != nil {
		return err
	}

	// Copy modinfo files (modules.*) from the snap; these might be
	// overwritten if kernel-modules components are installed (see
	// setupModsFromComp, which runs depmod). The files are copied from the
	// current mount only: their content is path-independent (module paths
	// are relative to the modules directory), so there is no need to
	// re-copy them for the target mount, which in any case may not exist
	// yet during install/preseed. Only the symlinks are re-pointed to the
	// target mount below, since they encode absolute mount paths.
	//
	// While scanning the modules tree, also collect the directories found
	// under it, to be set up as symlinks below, skipping the ones that are
	// either reserved or not useful in the drivers tree.

	modDirs := map[string]bool{
		"kernel": true, // the default kernel drivers tree
		"vdso":   true, // the expected vdso libs tree
	}
	for _, e := range entries {
		switch {
		case !e.Type().IsDir(): // files & symlinks
			// Copy modprobe artifacts (modules.*).
			if strings.HasPrefix(e.Name(), "modules.") {
				target := filepath.Join(modsRoot, e.Name())
				if err := osutil.CopyFile(filepath.Join(currentMntDir, e.Name()), target, osutil.CopyFlagDefault); err != nil {
					return err
				}
			}
		case e.IsDir():
			n := e.Name()
			switch n {
			// Drop and log entries which would cause conflicts. We are
			// expecting those to have raised an error during snap pack.
			case "updates":
				// Reserved for modules coming from kernel-modules
				// components; do not link it back to the kernel snap.
				logger.Debugf("skipping directory %q in the kernel modules tree, reserved for components", n)
			case "build":
				// Typically a symlink to the kernel source tree; not
				// useful in the drivers tree.
				logger.Debugf("skipping directory %q in the kernel modules tree, typically the kernel source tree", n)
			default:
				modDirs[n] = true
			}
		}
	}

	// Symbolic links to current mount of the kernel snap
	if err := createKernelModulesSymlinks(modsRoot, currentMntDir, modDirs); err != nil {
		return err
	}

	// If necessary, add modules from components and run depmod
	if err := setupModsFromComp(kernelTree, kversion, compsMntPts); err != nil {
		return err
	}

	// Change symlinks to target ones when needed. Reuse the directories
	// discovered from the current mount: the target mount holds the same
	// kernel snap content, just mounted at a different path.
	if !kMntPts.CurrentEqualsTarget() {
		targetMntDir := kMntPts.UnderTargetPath("modules", kversion)
		if err := createKernelModulesSymlinks(modsRoot, targetMntDir, modDirs); err != nil {
			return err
		}
	}

	return nil
}

func createKernelModulesSymlinks(modsRoot, kMntPt string, dirs map[string]bool) error {
	for d := range dirs {
		lname := filepath.Join(modsRoot, d)
		to := filepath.Join(kMntPt, d)

		os.Remove(lname)
		if err := osSymlink(to, lname); err != nil {
			return err
		}
	}

	return nil
}

func setupModsFromComp(kernelTree, kversion string, compsMntPts []ModulesCompMountPoints) error {
	// This folder needs to exist always to allow for directory swapping
	// in the future, even if right now we don't have components.
	compsRoot := filepath.Join(kernelTree, "lib", "modules", kversion, "updates")
	if err := os.MkdirAll(compsRoot, 0755); err != nil {
		return err
	}

	if len(compsMntPts) == 0 {
		return nil
	}

	// Symbolic links to components
	for _, cmp := range compsMntPts {
		lname := filepath.Join(compsRoot, cmp.LinkName)
		to := cmp.UnderCurrentPath("modules", kversion)
		if err := osSymlink(to, lname); err != nil {
			return err
		}
	}

	// Run depmod
	stdout, stderr, err := osutil.RunSplitOutput("depmod", "-b", kernelTree, kversion)
	if err != nil {
		return osutil.OutputErrCombine(stdout, stderr, err)
	}
	logger.Noticef("depmod output:\n%s\n", string(osutil.CombineStdOutErr(stdout, stderr)))

	// Change symlinks to target ones when needed
	for _, cmp := range compsMntPts {
		if cmp.CurrentEqualsTarget() {
			continue
		}
		lname := filepath.Join(compsRoot, cmp.LinkName)
		to := cmp.UnderTargetPath("modules", kversion)
		// remove old link
		os.Remove(lname)
		if err := osSymlink(to, lname); err != nil {
			return err
		}
	}

	return nil
}

// modulesTreesEqual compares two lib/modules/<kversion> directories
// (excluding the "updates" subtree, reserved for kernel-modules components
// and out of scope for this comparison, as it is unaffected when components
// have not changed) entry-by-entry: same set of names, same symlink
// targets, same regular file contents.
// modulesTreesEqual compares two lib/modules/<kversion> directories
// (excluding the "updates" subtree, reserved for kernel-modules components
// and out of scope for this comparison, as it is unaffected when components
// have not changed) entry-by-entry: same set of names, same symlink
// targets, same regular file contents.
func modulesTreesEqual(oldDir, newDir string) (bool, error) {
	return dirContentsEqual(oldDir, newDir, "updates")
}

// dirContentsEqual compares two directories entry-by-entry (optionally
// skipping one entry name in both, e.g. a nested reserved subtree): same
// set of names, same symlink targets, same regular file contents.
func dirContentsEqual(oldDir, newDir, skip string) (bool, error) {
	oldEntries, err := readDirExcept(oldDir, skip)
	if err != nil {
		return false, err
	}
	newEntries, err := readDirExcept(newDir, skip)
	if err != nil {
		return false, err
	}
	if len(oldEntries) != len(newEntries) {
		return false, nil
	}

	for name, oldInfo := range oldEntries {
		newInfo, ok := newEntries[name]
		if !ok {
			return false, nil
		}
		if oldInfo.Type() != newInfo.Type() {
			return false, nil
		}
		switch {
		case oldInfo.Type()&fs.ModeSymlink != 0:
			oldTarget, err := os.Readlink(filepath.Join(oldDir, name))
			if err != nil {
				return false, err
			}
			newTarget, err := os.Readlink(filepath.Join(newDir, name))
			if err != nil {
				return false, err
			}
			if oldTarget != newTarget {
				return false, nil
			}
		case oldInfo.Type().IsRegular():
			oldContent, err := os.ReadFile(filepath.Join(oldDir, name))
			if err != nil {
				return false, err
			}
			newContent, err := os.ReadFile(filepath.Join(newDir, name))
			if err != nil {
				return false, err
			}
			if !bytes.Equal(oldContent, newContent) {
				return false, nil
			}
		default:
			// Any other structural difference (e.g. an unexpected
			// directory): be conservative and treat it as changed
			// rather than silently ignoring it.
			return false, nil
		}
	}

	return true, nil
}

func readDirExcept(dir, except string) (map[string]fs.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]fs.DirEntry, len(entries))
	for _, e := range entries {
		if except != "" && e.Name() == except {
			continue
		}
		out[e.Name()] = e
	}
	return out, nil
}

// syncFirmwareTopLevelSymlinks applies live, per-entry changes to the
// top-level entries of an already-mounted lib/firmware directory. This is
// used by Regenerate mode instead of a scratch-build-and-swap because
// lib/firmware is itself a bind-mount source (see createKernelMounts in
// cmd/snap-bootstrap): renaming/exchanging it (or lib/modules, or destDir
// itself) as a whole would not be visible to an already-established mount,
// silently leaving it showing stale content forever. Only children of an
// already-mounted directory are live when changed, so each top-level entry
// is created/updated/removed individually, directly against the live
// directory, using an atomic symlink replace so the destination path is
// never briefly unresolvable to a concurrent reader (e.g. a kernel
// request_firmware() call landing on lib/firmware). "updates" (reserved
// for kernel-modules components) and any unrecognized directory are left
// untouched. Returns whether anything was changed.
func syncFirmwareTopLevelSymlinks(kMntPts MountPoints, liveFwDir string) (changed bool, err error) {
	if err := os.MkdirAll(liveFwDir, 0755); err != nil {
		return false, err
	}

	desired, err := firmwareSymlinkTargets(kMntPts, liveFwDir)
	if err != nil {
		return false, err
	}

	desiredNames := make(map[string]bool, len(desired))
	for _, d := range desired {
		desiredNames[d.name] = true
		lpath := filepath.Join(liveFwDir, d.name)
		cur, readErr := os.Readlink(lpath)
		if readErr == nil && cur == d.target {
			// Already correct, nothing to do for this entry.
			continue
		}
		// Either missing, wrong target, or not a symlink at all: replace it.
		// This only ever touches a child of the lib/firmware mount point,
		// never lib/firmware itself, so it is safe to do live. Use an atomic
		// symlink replace (rename over the destination) so lpath is never
		// briefly unresolvable to a concurrent reader.
		if err := osutil.AtomicSymlink(d.target, lpath); err != nil {
			if errors.Is(err, syscall.EEXIST) {
				// A real directory (empty or not) sitting where we want to
				// place a symlink is presumed to be something the user
				// placed directly on the live tree, not generator-created
				// content. Do not destroy it: log and leave this entry
				// alone, but keep processing the rest.
				logger.Noticef("cannot replace %q with a firmware symlink: a directory is already present, leaving it as is", lpath)
				continue
			}
			return changed, err
		}
		changed = true
	}

	// Remove stale entries that used to be created by an older (buggy)
	// generator but should no longer exist. Conservative: only ever
	// remove symlinks, never a directory we don't recognize.
	entries, err := os.ReadDir(liveFwDir)
	if err != nil {
		return changed, err
	}
	for _, e := range entries {
		if e.Name() == "updates" || desiredNames[e.Name()] {
			continue
		}
		if e.Type()&fs.ModeSymlink == 0 {
			logger.Noticef("unexpected entry %q found in %q, leaving it alone", e.Name(), liveFwDir)
			continue
		}
		if err := os.Remove(filepath.Join(liveFwDir, e.Name())); err != nil {
			return changed, err
		}
		changed = true
	}

	return changed, nil
}

// DriversTreeDir returns the directory for a given kernel and revision under
// rootdir.
func DriversTreeDir(rootdir, kernelName string, rev snap.Revision) string {
	return filepath.Join(dirs.SnapKernelDriversTreesDirUnder(rootdir),
		kernelName, rev.String())
}

// RemoveKernelDriversTree cleans-up the writable kernel tree in snapd data
// folder, under kernelSubdir/<rev> (kernelSubdir is usually the snap name).
// When called from the kernel package <rev> might be <rev>_tmp.
func RemoveKernelDriversTree(treeRoot string) (err error) {
	return os.RemoveAll(treeRoot)
}

type KernelDriversTreeOptions struct {
	// Set if we are building the tree for a kernel we are installing right now
	KernelInstall bool
	// Regenerate requests a check against an already-active, installed
	// kernel tree (as opposed to a fresh install, or a routine
	// kernel-modules-component change): build a candidate
	// lib/modules/<kversion>, compare it in full against what is live,
	// and only swap it in if genuinely different; separately, sync the
	// top-level lib/firmware symlinks live, per-entry (see comment on
	// why this can't be a whole-directory swap). Must only be used with
	// KernelInstall: false, against an already-installed, currently
	// active kernel+revision - never for a fresh install.
	Regenerate bool
}

// MountPoints describes mount points for a snap or a component.
type MountPoints struct {
	// Current is where the container to be installed is currently
	// available
	Current string
	// Target is where the container will be found in a running system
	Target string
}

func (mp *MountPoints) UnderCurrentPath(dirs ...string) string {
	return filepath.Join(append([]string{mp.Current}, dirs...)...)
}

func (mp *MountPoints) UnderTargetPath(dirs ...string) string {
	return filepath.Join(append([]string{mp.Target}, dirs...)...)
}

func (mp *MountPoints) CurrentEqualsTarget() bool {
	return mp.Current == mp.Target
}

// ModulesCompMountPoints contains mount points for a component plus its name.
type ModulesCompMountPoints struct {
	// LinkName is the name of the symlink in the drivers tree that will
	// point to the component modules.
	LinkName string
	MountPoints
}

// EnsureKernelDriversTree creates a drivers tree that can include modules/fw
// from kernel-modules components. opts.KernelInstall tells the function if
// this is a kernel install (which might be installing components at the same
// time) or an only components install.
//
// For kernel installs, this function creates a tree in destDir (should be of
// the form <somedir>/var/lib/snapd/kernel/<ksnapName>/<rev>), which is
// bind-mounted after a reboot to /usr/lib/{modules,firmware} (the currently
// active kernel is using a different path as it has a different revision).
// This tree contains files from the kernel snap content in kSnapRoot, as well
// as symlinks to it. Information from modules is found by looking at
// comps slice.
//
// For components-only install, we want the components to be available without
// rebooting. For this, we work on a temporary tree, and after finishing it we
// swap atomically the affected modules/firmware folders with those of the
// currently active kernel drivers tree.
//
// To make this work in all cases we need to know the current mounts of the
// kernel snap / components to be installed and the final mounts when the
// system is run after installation (as the installing system might be classic
// while the installed system could be hybrid or UC, or we could be installing
// from the initramfs). To consider all cases, we need to run depmod with links
// to the currently available content, and then replace those links with the
// expected mounts in the running system.
// NOTE: changes here that can alter the resulting tree must bump
// kernelDriversTreeGeneratorVersion above.
func EnsureKernelDriversTree(kMntPts MountPoints, compsMntPts []ModulesCompMountPoints, destDir string, opts *KernelDriversTreeOptions) (changed bool, err error) {
	// The temporal dir when installing only components can be fixed as a
	// task installing/updating a kernel-modules component must conflict
	// with changes containing this same task. This helps with clean-ups if
	// something goes wrong. Note that this folder needs to be in the same
	// filesystem as the final one so we can atomically switch the folders.
	destDir = strings.TrimSuffix(destDir, "/")
	targetDir := destDir + "_tmp"
	if opts.KernelInstall {
		targetDir = destDir
		exists, isDir, _ := osutil.DirExists(targetDir)
		if exists && isDir {
			logger.Debugf("device tree %q already created on installation, not re-creating",
				targetDir)
			// Nothing was built here, so the existing marker (if any) is
			// left untouched.
			return false, nil
		}
	}
	// Initial clean-up to make the function idempotent
	if rmErr := RemoveKernelDriversTree(targetDir); rmErr != nil &&
		!errors.Is(err, fs.ErrNotExist) {
		logger.Noticef("while removing old kernel tree: %v", rmErr)
	}

	defer func() {
		// Remove on return if error or if temporary tree
		if err == nil && opts.KernelInstall {
			return
		}
		if rmErr := RemoveKernelDriversTree(targetDir); rmErr != nil &&
			!errors.Is(err, fs.ErrNotExist) {
			logger.Noticef("while cleaning up kernel tree: %v", rmErr)
		}
	}()

	// Create drivers tree
	kversion, err := KernelVersionFromModulesDir(kMntPts.Current)
	if err == nil {
		if err := createModulesSubtree(kMntPts, targetDir,
			kversion, compsMntPts); err != nil {
			return false, err
		}
	} else {
		logger.Debugf("no modules found in %q", kMntPts.Current)
	}

	fwDir := filepath.Join(targetDir, "lib", "firmware")
	if opts.KernelInstall {
		// symlinks in /lib/firmware are not affected by components
		if err := createFirmwareSymlinks(kMntPts, fwDir); err != nil {
			return false, err
		}
	}
	updateFwDir := filepath.Join(fwDir, "updates")
	// This folder needs to exist always to allow for directory swapping
	// in the future, even if right now we don't have components.
	if err := os.MkdirAll(updateFwDir, 0755); err != nil {
		return false, err
	}
	for _, cmp := range compsMntPts {
		if err := createFirmwareSymlinks(cmp.MountPoints, updateFwDir); err != nil {
			return false, err
		}
	}

	// Sync before returning successfully (install kernel case) and also
	// for swapping case so we have consistent content before swapping
	// folder.
	doSync()

	if opts.KernelInstall {
		// A fresh install always counts as a change (destDir did not
		// exist as a proper directory beforehand, see the early return
		// above).
		if err := writeDriversTreeMeta(targetDir); err != nil {
			return false, err
		}
		return true, nil
	}

	// There is a (very small) chance of a poweroff/reboot while
	// having swapped only one of these two folders. If that
	// happens, snapd will re-run the task on the next boot, but
	// with mismatching modules/fw for the installed components. As
	// modules shipped by components should not be that critical,
	// in principle the system should recover.

	// Swap modules directories
	oldRoot := destDir

	// Swap updates directory inside firmware dir. In Regenerate mode this
	// is left alone entirely: managing kernel-modules-component firmware
	// is not this mode's job (it only re-derives what components already
	// active), so there is nothing to compare or swap here regardless of
	// content.
	oldFwUpdates := filepath.Join(oldRoot, "lib", "firmware", "updates")
	fwUpdatesChanged := false
	if !opts.Regenerate {
		fwUpdatesChanged = true
		if err := osutil.SwapDirs(oldFwUpdates, updateFwDir); err != nil {
			return false, fmt.Errorf("while swapping %q <-> %q: %w", oldFwUpdates, updateFwDir, err)
		}
	}

	newMods := filepath.Join(targetDir, "lib", "modules", kversion)
	oldMods := filepath.Join(oldRoot, "lib", "modules", kversion)

	// In Regenerate mode, only swap the modules subtree in if it is
	// actually different from what is live: depmod's output isn't
	// knowable without building the candidate (already done above), so
	// we compare the built candidate against the live tree before
	// deciding. Outside Regenerate mode (routine components change),
	// always swap unconditionally, as today. A kernel with no modules at
	// all (kversion == "") is valid; there is nothing to compare or swap
	// in that case, so skip this entirely.
	modsChanged := false
	if kversion != "" {
		modsChanged = true
		if opts.Regenerate {
			same, err := modulesTreesEqual(oldMods, newMods)
			if err != nil {
				// Undo firmware swap
				if fwUpdatesChanged {
					if err := osutil.SwapDirs(oldFwUpdates, updateFwDir); err != nil {
						logger.Noticef("while reverting firmware updates swap: %v", err)
					}
				}
				return false, err
			}
			modsChanged = !same
		}

		if modsChanged {
			if err := osutil.SwapDirs(oldMods, newMods); err != nil {
				// Undo firmware swap
				if fwUpdatesChanged {
					if err := osutil.SwapDirs(oldFwUpdates, updateFwDir); err != nil {
						logger.Noticef("while reverting modules swap: %v", err)
					}
				}
				return false, fmt.Errorf("while swapping %q <-> %q: %w", newMods, oldMods, err)
			}
		}
	}

	// Make sure that changes are written
	doSync()

	changed = modsChanged || fwUpdatesChanged

	if opts.Regenerate {
		// Top-level lib/firmware entries cannot be built in scratch and
		// swapped as a unit: lib/firmware (like lib/modules and destDir
		// itself) is a bind-mount source, and renaming/exchanging it (or
		// an ancestor) does not propagate to an already-established
		// mount. Only children of the mount point are live when changed,
		// so these are synced live, per entry, directly against the live
		// destination.
		liveFwDir := filepath.Join(oldRoot, "lib", "firmware")
		fwChanged, err := syncFirmwareTopLevelSymlinks(kMntPts, liveFwDir)
		if err != nil {
			return changed, err
		}
		changed = changed || fwChanged

		// Sync before writing the marker: these firmware changes were
		// made live, directly against the mounted destination, and must
		// be on disk before the marker claims the tree is current, or a
		// crash could persist the marker while losing the symlink change.
		doSync()
	}

	if err := writeDriversTreeMeta(oldRoot); err != nil {
		return changed, err
	}

	return changed, nil
}

// NeedsKernelDriversTree returns true if we need a kernel drivers tree for this model.
func NeedsKernelDriversTree(mod *asserts.Model) bool {
	// Checking if it has modeenv - it must be UC20+ or hybrid
	if mod.Grade() == asserts.ModelGradeUnset {
		return false
	}

	// We assume core24/hybrid 24.04 onwards have the generator, for older
	// boot bases we return false.
	switch mod.Base() {
	case "core22":
		if mod.Classic() {
			// This is a workaround for LP#2104933. The base should
			// never have been core22 in 24.04/24.10.
			return classic24ModelWithWrongBase()
		}
		return false
	case "core20", "core22-desktop":
		return false
	default:
		return true
	}
}

// This is a workaround for LP#2104933. The base should never have been core22
// in classic 24.04/24.10.
func classic24ModelWithWrongBase() bool {
	return release.ReleaseInfo.ID == "ubuntu" &&
		(release.ReleaseInfo.VersionID == "24.04" || release.ReleaseInfo.VersionID == "24.10")
}

// -*- Mode: Go; indent-tabs-mode: t -*-

package kernel_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/kernel"
	"github.com/snapcore/snapd/testutil"
)

func writePendingDriversTreeMarker(c *C, dest string, mounts kernel.MountPoints, comps []kernel.ModulesCompMountPoints) {
	data, err := json.Marshal(map[string]any{
		"generator-version": kernel.KernelDriversTreeGeneratorVersion(),
		"kernel":            mounts,
		"components":        comps,
		"destination":       dest,
	})
	c.Assert(err, IsNil)
	c.Assert(os.WriteFile(filepath.Join(dest, "kernel.json.built"), data, 0644), IsNil)
}

// Model an interrupted reconciliation: firmware/updates reflects the new
// inputs, while modules and top-level firmware still reflect the old inputs.
func (s *kernelDriversTestSuite) TestEnsureRepairsPartiallyAppliedTree(c *C) {
	depmod := testutil.MockCommand(c, "depmod", "")
	defer depmod.Restore()

	root := c.MkDir()
	mountDir := filepath.Join(root, "mounted-kernel")
	version := "6.5.4-test"
	createKernelSnapFiles(c, version, mountDir, createKernelSnapFilesOpts{})
	oldDir := filepath.Join(root, "old-component")
	newDir := filepath.Join(root, "new-component")
	createKernelModulesCompFiles(c, version, oldDir, "old")
	createKernelModulesCompFiles(c, version, newDir, "new")
	oldComp := kernel.ModulesCompMountPoints{LinkName: "old", MountPoints: kernel.MountPoints{Current: oldDir, Target: oldDir}}
	newComp := kernel.ModulesCompMountPoints{LinkName: "new", MountPoints: kernel.MountPoints{Current: newDir, Target: newDir}}
	mounts := kernel.MountPoints{Current: mountDir, Target: mountDir}
	dest := filepath.Join(root, "tree")
	c.Assert(kernel.EnsureKernelDriversTree(mounts, []kernel.ModulesCompMountPoints{oldComp}, dest), IsNil)

	c.Assert(os.Remove(filepath.Join(mountDir, "firmware", "blob1")), IsNil)
	c.Assert(os.WriteFile(filepath.Join(mountDir, "firmware", "extra.bin"), nil, 0644), IsNil)
	fwParent := filepath.Join(dest, "lib", "firmware")
	fwUpdates := filepath.Join(fwParent, "updates")
	c.Assert(os.RemoveAll(fwUpdates), IsNil)
	c.Assert(os.Mkdir(fwUpdates, 0755), IsNil)
	c.Assert(os.Symlink(filepath.Join(newDir, "firmware", "new.bin"), filepath.Join(fwUpdates, "new.bin")), IsNil)
	writePendingDriversTreeMarker(c, dest, mounts, []kernel.ModulesCompMountPoints{newComp})
	outdated, err := kernel.DriversTreeOutdated(dest)
	c.Assert(err, IsNil)
	c.Check(outdated, Equals, true)

	c.Assert(kernel.EnsureKernelDriversTree(mounts, []kernel.ModulesCompMountPoints{newComp}, dest), IsNil)
	modsUpdates := filepath.Join(dest, "lib", "modules", version, "updates")
	c.Check(filepath.Join(modsUpdates, "old"), testutil.FileAbsent)
	c.Check(filepath.Join(modsUpdates, "new"), testutil.SymlinkTargetEquals, filepath.Join(newDir, "modules", version))
	c.Check(filepath.Join(fwUpdates, "new.bin"), testutil.SymlinkTargetEquals, filepath.Join(newDir, "firmware", "new.bin"))
	c.Check(filepath.Join(fwParent, "blob1"), testutil.FileAbsent)
	c.Check(filepath.Join(fwParent, "extra.bin"), testutil.SymlinkTargetEquals, filepath.Join(mountDir, "firmware", "extra.bin"))
	meta, err := kernel.ReadDriversTreeMeta(dest)
	c.Assert(err, IsNil)
	c.Check(meta.Components, DeepEquals, []kernel.ModulesCompMountPoints{newComp})
	c.Check(filepath.Join(dest, "kernel.json.in"), testutil.FileAbsent)
	c.Check(filepath.Join(dest, "kernel.json.built"), testutil.FileAbsent)
}

// Model a new tree moved into place before its built marker became final.
func (s *kernelDriversTestSuite) TestEnsureCompletesUnfinishedNewTree(c *C) {
	root := c.MkDir()
	mountDir := filepath.Join(root, "source")
	createKernelSnapFilesOnlyFw(c, mountDir)
	mounts := kernel.MountPoints{Current: mountDir, Target: filepath.Join(root, "future-mount")}
	dest := filepath.Join(root, "tree")
	fwDir := filepath.Join(dest, "lib", "firmware")
	c.Assert(os.MkdirAll(filepath.Join(fwDir, "updates"), 0755), IsNil)
	fwTarget := filepath.Join(mounts.Target, "firmware", "wifi_fw.bin")
	c.Assert(os.Symlink(fwTarget, filepath.Join(fwDir, "wifi_fw.bin")), IsNil)
	writePendingDriversTreeMarker(c, dest, mounts, nil)

	c.Assert(kernel.EnsureKernelDriversTree(mounts, nil, dest), IsNil)
	c.Check(filepath.Join(fwDir, "wifi_fw.bin"), testutil.SymlinkTargetEquals, fwTarget)
	meta, err := kernel.ReadDriversTreeMeta(dest)
	c.Assert(err, IsNil)
	c.Check(*meta.Kernel, DeepEquals, mounts)
	c.Check(filepath.Join(dest, "kernel.json.built"), testutil.FileAbsent)
	c.Check(dest+"_tmp", testutil.FileAbsent)
}

func (s *kernelDriversTestSuite) TestEnsureMarkerLegacyAndForward(c *C) {
	root := c.MkDir()
	mountDir := filepath.Join(root, "kernel")
	createKernelSnapFilesOnlyFw(c, mountDir)
	mounts := kernel.MountPoints{Current: mountDir, Target: mountDir}
	dest := filepath.Join(root, "tree")
	c.Assert(kernel.EnsureKernelDriversTree(mounts, nil, dest), IsNil)
	marker := filepath.Join(dest, "kernel.json")
	c.Assert(os.WriteFile(marker, []byte(`{"generator-version":1}`), 0644), IsNil)
	outdated, err := kernel.DriversTreeOutdated(dest)
	c.Assert(err, IsNil)
	c.Check(outdated, Equals, true)
	c.Assert(kernel.EnsureKernelDriversTree(mounts, nil, dest), IsNil)
	meta, err := kernel.ReadDriversTreeMeta(dest)
	c.Assert(err, IsNil)
	c.Check(meta.Destination, Equals, dest)
	c.Check(meta.GeneratorVersion, Equals, kernel.KernelDriversTreeGeneratorVersion())

	forward := []byte(`{"generator-version":99}`)
	c.Assert(os.WriteFile(marker+".in", forward, 0644), IsNil)
	err = kernel.EnsureKernelDriversTree(mounts, nil, dest)
	c.Assert(err, ErrorMatches, `cannot replace kernel drivers tree .* from newer generator 99`)
	data, err := os.ReadFile(marker + ".in")
	c.Assert(err, IsNil)
	c.Check(string(data), Equals, string(forward))
	c.Assert(os.Remove(marker+".in"), IsNil)
	c.Assert(os.WriteFile(marker, forward, 0644), IsNil)
	err = kernel.EnsureKernelDriversTree(mounts, nil, dest)
	c.Assert(err, ErrorMatches, `cannot replace kernel drivers tree .* from newer generator 99`)
	c.Check(strings.Contains(err.Error(), dest), Equals, true)
}

func (s *kernelDriversTestSuite) TestEnsureMissingSourcesCannotComplete(c *C) {
	root := c.MkDir()
	mountDir := filepath.Join(root, "kernel")
	c.Assert(os.MkdirAll(mountDir, 0755), IsNil)
	dest := filepath.Join(root, "tree")
	mounts := kernel.MountPoints{Current: mountDir, Target: mountDir}
	c.Assert(kernel.EnsureKernelDriversTree(mounts, nil, dest), ErrorMatches, `cannot read kernel drivers source .*no mounted kernel content`)
	c.Check(filepath.Join(dest, "kernel.json"), testutil.FileAbsent)
	createKernelSnapFilesOnlyFw(c, mountDir)
	cmpDir := filepath.Join(root, "unmounted-component")
	c.Assert(os.MkdirAll(cmpDir, 0755), IsNil)
	cmp := kernel.ModulesCompMountPoints{LinkName: "missing", MountPoints: kernel.MountPoints{Current: cmpDir, Target: cmpDir}}
	c.Assert(kernel.EnsureKernelDriversTree(mounts, []kernel.ModulesCompMountPoints{cmp}, dest), ErrorMatches, `cannot read kernel component source .*no mounted component content`)
	c.Check(filepath.Join(dest, "kernel.json"), testutil.FileAbsent)
}

// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2021 Canonical Ltd
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

package devicestate_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/boot"
	"github.com/snapcore/snapd/bootloader"
	"github.com/snapcore/snapd/bootloader/bootloadertest"
	"github.com/snapcore/snapd/overlord/devicestate"
	"github.com/snapcore/snapd/seed/seedtest"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/snap/snaptest"
	"github.com/snapcore/snapd/testutil"
)

type createSystemSuite struct {
	deviceMgrBaseSuite

	ss *seedtest.SeedSnaps
}

var _ = Suite(&createSystemSuite{})

var (
	genericSnapYaml = "name: %s\nversion: 1.0\n%s"
	snapYamls       = map[string]string{
		"pc-kernel":      "name: pc-kernel\nversion: 1.0\ntype: kernel",
		"pc":             "name: pc\nversion: 1.0\ntype: gadget\nbase: core20",
		"core20":         "name: core20\nversion: 20.1\ntype: base",
		"core18":         "name: core18\nversion: 18.1\ntype: base",
		"snapd":          "name: snapd\nversion: 2.2.2\ntype: snapd",
		"other-required": fmt.Sprintf(genericSnapYaml, "other-required", "base: core20"),
		"other-present":  fmt.Sprintf(genericSnapYaml, "other-present", "base: core20"),
		"other-core18":   fmt.Sprintf(genericSnapYaml, "other-present", "base: core18"),
	}
	snapFiles = map[string][][]string{
		"pc": {
			{"meta/gadget.yaml", gadgetYaml},
			{"cmdline.extra", "args from gadget"},
		},
	}
)

func (s *createSystemSuite) SetUpTest(c *C) {
	s.deviceMgrBaseSuite.SetUpTest(c)

	s.ss = &seedtest.SeedSnaps{
		StoreSigning: s.storeSigning,
		Brands:       s.brands,
	}
	s.AddCleanup(func() { bootloader.Force(nil) })
}

func (s *createSystemSuite) makeSnap(c *C, name string, rev snap.Revision) *snap.Info {
	si := &snap.SideInfo{
		RealName: name,
		SnapID:   s.ss.AssertedSnapID(name),
		Revision: rev,
	}
	// asserted?
	where, info := snaptest.MakeTestSnapInfoWithFiles(c, snapYamls[name], snapFiles[name], si)
	c.Assert(os.MkdirAll(filepath.Dir(info.MountFile()), 0755), IsNil)
	c.Assert(os.Rename(where, info.MountFile()), IsNil)
	s.setupSnapDecl(c, info, "my-brand")
	s.setupSnapRevision(c, info, "my-brand", rev)
	return info
}

func (s *createSystemSuite) TestCreateSystemFromAssertedSnaps(c *C) {
	bl := bootloadertest.Mock("trusted", c.MkDir()).WithRecoveryAwareTrustedAssets()
	// make it simple for now, no assets
	bl.TrustedAssetsList = nil
	bl.StaticCommandLine = "mock static"
	bl.CandidateStaticCommandLine = "unused"
	bootloader.Force(bl)
	infos := map[string]*snap.Info{}

	s.state.Lock()
	defer s.state.Unlock()
	s.setupBrands(c)
	infos["pc-kernel"] = s.makeSnap(c, "pc-kernel", snap.R(1))
	infos["pc"] = s.makeSnap(c, "pc", snap.R(2))
	infos["core20"] = s.makeSnap(c, "core20", snap.R(3))
	infos["snapd"] = s.makeSnap(c, "snapd", snap.R(4))
	infos["other-present"] = s.makeSnap(c, "other-present", snap.R(5))
	infos["other-required"] = s.makeSnap(c, "other-required", snap.R(6))
	infos["other-core18"] = s.makeSnap(c, "other-core18", snap.R(7))
	infos["core18"] = s.makeSnap(c, "core18", snap.R(8))

	model := s.makeModelAssertionInState(c, "my-brand", "pc", map[string]interface{}{
		"architecture": "amd64",
		"grade":        "dangerous",
		"base":         "core20",
		"snaps": []interface{}{
			map[string]interface{}{
				"name":            "pc-kernel",
				"id":              s.ss.AssertedSnapID("pc-kernel"),
				"type":            "kernel",
				"default-channel": "20",
			},
			map[string]interface{}{
				"name":            "pc",
				"id":              s.ss.AssertedSnapID("pc"),
				"type":            "gadget",
				"default-channel": "20",
			},
			map[string]interface{}{
				"name": "snapd",
				"id":   s.ss.AssertedSnapID("snapd"),
				"type": "snapd",
			},
			// optional but not present
			map[string]interface{}{
				"name":     "other-not-present",
				"id":       s.ss.AssertedSnapID("other-not-present"),
				"presence": "optional",
			},
			// optional and present
			map[string]interface{}{
				"name":     "other-present",
				"id":       s.ss.AssertedSnapID("other-present"),
				"presence": "optional",
			},
			// required
			map[string]interface{}{
				"name":     "other-required",
				"id":       s.ss.AssertedSnapID("other-required"),
				"presence": "required",
			},
			// different base
			map[string]interface{}{
				"name": "other-core18",
				"id":   s.ss.AssertedSnapID("other-core18"),
			},
			// and the actual base for that snap
			map[string]interface{}{
				"name": "core18",
				"id":   s.ss.AssertedSnapID("core18"),
				"type": "base",
			},
		},
	})

	infoGetter := func(name string) (*snap.Info, bool, error) {
		c.Logf("called for: %q", name)
		info, present := infos[name]
		return info, present, nil
	}

	newFiles, dir, err := devicestate.CreateSystemForModelFromValidatedSnaps(infoGetter, s.db, "1234", model)
	c.Assert(err, IsNil)
	c.Check(newFiles, DeepEquals, []string{
		filepath.Join(boot.InitramfsUbuntuSeedDir, "snaps/snapd_4.snap"),
		filepath.Join(boot.InitramfsUbuntuSeedDir, "snaps/pc-kernel_1.snap"),
		filepath.Join(boot.InitramfsUbuntuSeedDir, "snaps/core20_3.snap"),
		filepath.Join(boot.InitramfsUbuntuSeedDir, "snaps/pc_2.snap"),
		filepath.Join(boot.InitramfsUbuntuSeedDir, "snaps/other-present_5.snap"),
		filepath.Join(boot.InitramfsUbuntuSeedDir, "snaps/other-required_6.snap"),
		filepath.Join(boot.InitramfsUbuntuSeedDir, "snaps/other-core18_7.snap"),
		filepath.Join(boot.InitramfsUbuntuSeedDir, "snaps/core18_8.snap"),
	})
	for _, info := range infos {
		c.Check(filepath.Join(boot.InitramfsUbuntuSeedDir, "snaps", filepath.Base(info.MountFile())),
			testutil.FileEquals,
			testutil.ReferenceFile(info.MountFile()))
	}
	c.Check(dir, Equals, filepath.Join(boot.InitramfsUbuntuSeedDir, "systems/1234"))
	c.Check(bl.RecoverySystemDir, Equals, "/systems/1234")
	c.Check(bl.RecoverySystemBootVars, DeepEquals, map[string]string{
		"snapd_full_cmdline_args":  "",
		"snapd_extra_cmdline_args": "args from gadget",
		"snapd_recovery_kernel":    "/snaps/pc-kernel_1.snap",
	})
}

// func (s *createSystemSuite) TestCreateSystemWithTrustedAssets(c *C) {
// 	bl := bootloadertest.Mock("trusted", c.MkDir()).TrustedAssets()
// 	// make it simple for now, no assets
// 	bl.TrustedAssetsList = nil
// 	bl.StaticCommandLine = "mock static"
// 	bl.CandidateStaticCommandLine = "unused"
// 	infos := map[string]*snap.Info{}

// 	s.state.Lock()
// 	defer s.state.Unlock()
// 	s.setupBrands(c)
// 	infos["pc-kernel"] = s.makeSnap(c, "pc-kernel", snap.R(1))
// 	infos["pc"] = s.makeSnap(c, "pc", snap.R(2))
// 	infos["core20"] = s.makeSnap(c, "core20", snap.R(3))
// 	infos["snapd"] = s.makeSnap(c, "snapd", snap.R(4))

// 	model := s.makeModelAssertionInState(c, "my-brand", "pc", map[string]interface{}{
// 		"architecture": "amd64",
// 		"grade":        "dangerous",
// 		"base":         "core20",
// 		"snaps": []interface{}{
// 			map[string]interface{}{
// 				"name":            "pc-kernel",
// 				"id":              s.ss.AssertedSnapID("pc-kernel"),
// 				"type":            "kernel",
// 				"default-channel": "20",
// 			},
// 			map[string]interface{}{
// 				"name":            "pc",
// 				"id":              s.ss.AssertedSnapID("pc"),
// 				"type":            "gadget",
// 				"default-channel": "20",
// 			},
// 			map[string]interface{}{
// 				"name": "snapd",
// 				"id":   s.ss.AssertedSnapID("snapd"),
// 				"type": "snapd",
// 			},
// 		},
// 	})

// 	infoGetter := func(name string) (*snap.Info, bool, error) {
// 		c.Logf("called for: %q", name)
// 		info, present := infos[name]
// 		return info, present, nil
// 	}

// 	newFiles, dir, err := devicestate.CreateSystemForModelFromValidatedSnaps(infoGetter, s.db, "1234", model)
// 	c.Assert(err, IsNil)
// 	c.Check(newFiles, HasLen, 0)
// 	c.Check(dir, Equals, filepath.Join(boot.InitramfsUbuntuSeedDir, "systems/1234"))
// }

// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2020 Canonical Ltd
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

package boot_test

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/boot"
	"github.com/snapcore/snapd/bootloader"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/secboot"
	"github.com/snapcore/snapd/testutil"
)

type sealSuite struct {
	testutil.BaseTest
}

var _ = Suite(&sealSuite{})

func (s *sealSuite) TestSealKeyToModeenv(c *C) {
	for _, tc := range []struct {
		sealErr error
		err     string
	}{
		{sealErr: nil, err: ""},
		{sealErr: errors.New("seal error"), err: "cannot seal the encryption key: seal error"},
	} {
		tmpDir := c.MkDir()
		dirs.SetRootDir(tmpDir)
		defer dirs.SetRootDir("")

		err := createMockGrubCfg(filepath.Join(tmpDir, "run/mnt/ubuntu-seed"))
		c.Assert(err, IsNil)

		err = createMockGrubCfg(filepath.Join(tmpDir, "run/mnt/ubuntu-boot"))
		c.Assert(err, IsNil)

		modeenv := &boot.Modeenv{
			RecoverySystem: "20200825",
		}

		// set encryption key
		myKey := secboot.EncryptionKey{}
		for i := range myKey {
			myKey[i] = byte(i)
		}

		model := makeMockUC20Model()

		// set mock key sealing
		sealKeyCalls := 0
		restore := boot.MockSecbootSealKey(func(key secboot.EncryptionKey, params *secboot.SealKeyParams) error {
			sealKeyCalls++
			c.Check(key, DeepEquals, myKey)
			c.Assert(params.ModelParams, HasLen, 1)
			c.Assert(params.ModelParams[0].Model.DisplayName(), Equals, "My Model")
			c.Assert(params.ModelParams[0].EFILoadChains, DeepEquals, [][]bootloader.BootFile{
				{
					bootloader.NewBootFile("", filepath.Join(tmpDir, "run/mnt/ubuntu-seed/EFI/boot/bootx64.efi"), bootloader.RoleRecovery),
					bootloader.NewBootFile("", filepath.Join(tmpDir, "run/mnt/ubuntu-seed/EFI/boot/grubx64.efi"), bootloader.RoleRecovery),
					bootloader.NewBootFile("", filepath.Join(tmpDir, "run/mnt/ubuntu-boot/EFI/boot/grubx64.efi"), bootloader.RoleRunMode),
					bootloader.NewBootFile("", filepath.Join(tmpDir, "run/mnt/ubuntu-boot/EFI/ubuntu/kernel.efi"), bootloader.RoleRunMode),
				},
			})
			c.Assert(params.ModelParams[0].KernelCmdlines, DeepEquals, []string{
				"snapd_recovery_mode=run console=ttyS0 console=tty1 panic=-1",
				"snapd_recovery_mode=recover snapd_recovery_system=20200825 console=ttyS0 console=tty1 panic=-1",
			})
			return tc.sealErr
		})
		defer restore()

		err = boot.SealKeyToModeenv(myKey, model, modeenv)
		c.Assert(sealKeyCalls, Equals, 1)
		if tc.err == "" {
			c.Assert(err, IsNil)
		} else {
			c.Assert(err, ErrorMatches, tc.err)
		}
	}
}

func createMockGrubCfg(baseDir string) error {
	cfg := filepath.Join(baseDir, "EFI/ubuntu/grub.cfg")
	if err := os.MkdirAll(filepath.Dir(cfg), 0755); err != nil {
		return err
	}
	return ioutil.WriteFile(cfg, []byte("# Snapd-Boot-Config-Edition: 1\n"), 0644)
}

func (s *sealSuite) TestBootAssetsSort(c *C) {
	// by role
	d := []boot.BootAsset{
		{Role: "run", Name: "1ist", Hashes: []string{"b", "c"}},
		{Role: "recovery", Name: "1ist", Hashes: []string{"b", "c"}},
	}
	sort.Sort(boot.ByBootAssetOrder(d))
	c.Check(d, DeepEquals, []boot.BootAsset{
		{Role: "recovery", Name: "1ist", Hashes: []string{"b", "c"}},
		{Role: "run", Name: "1ist", Hashes: []string{"b", "c"}},
	})

	// by name
	d = []boot.BootAsset{
		{Role: "recovery", Name: "shim", Hashes: []string{"d", "e"}},
		{Role: "recovery", Name: "loader", Hashes: []string{"d", "e"}},
	}
	sort.Sort(boot.ByBootAssetOrder(d))
	c.Check(d, DeepEquals, []boot.BootAsset{
		{Role: "recovery", Name: "loader", Hashes: []string{"d", "e"}},
		{Role: "recovery", Name: "shim", Hashes: []string{"d", "e"}},
	})

	// by hash list length
	d = []boot.BootAsset{
		{Role: "run", Name: "1ist", Hashes: []string{"a", "f"}},
		{Role: "run", Name: "1ist", Hashes: []string{"d"}},
	}
	sort.Sort(boot.ByBootAssetOrder(d))
	c.Check(d, DeepEquals, []boot.BootAsset{
		{Role: "run", Name: "1ist", Hashes: []string{"d"}},
		{Role: "run", Name: "1ist", Hashes: []string{"a", "f"}},
	})

	// hash list entries
	d = []boot.BootAsset{
		{Role: "run", Name: "1ist", Hashes: []string{"b", "d"}},
		{Role: "run", Name: "1ist", Hashes: []string{"b", "c"}},
	}
	sort.Sort(boot.ByBootAssetOrder(d))
	c.Check(d, DeepEquals, []boot.BootAsset{
		{Role: "run", Name: "1ist", Hashes: []string{"b", "c"}},
		{Role: "run", Name: "1ist", Hashes: []string{"b", "d"}},
	})

	d = []boot.BootAsset{
		{Role: "run", Name: "loader", Hashes: []string{"z"}},
		{Role: "recovery", Name: "shim", Hashes: []string{"b"}},
		{Role: "run", Name: "loader", Hashes: []string{"c", "d"}},
		{Role: "run", Name: "1oader", Hashes: []string{"d", "e"}},
		{Role: "recovery", Name: "loader", Hashes: []string{"d", "e"}},
		{Role: "run", Name: "0oader", Hashes: []string{"x", "z"}},
	}
	sort.Sort(boot.ByBootAssetOrder(d))
	c.Check(d, DeepEquals, []boot.BootAsset{
		{Role: "recovery", Name: "loader", Hashes: []string{"d", "e"}},
		{Role: "recovery", Name: "shim", Hashes: []string{"b"}},
		{Role: "run", Name: "0oader", Hashes: []string{"x", "z"}},
		{Role: "run", Name: "1oader", Hashes: []string{"d", "e"}},
		{Role: "run", Name: "loader", Hashes: []string{"z"}},
		{Role: "run", Name: "loader", Hashes: []string{"c", "d"}},
	})
}

func (s *sealSuite) TestBootChainMarshalOnlyAssets(c *C) {
	bc := &boot.BootChain{
		AssetChain: []boot.BootAsset{
			{Role: "run", Name: "loader", Hashes: []string{"z"}},
			{Role: "recovery", Name: "shim", Hashes: []string{"b"}},
			{Role: "run", Name: "loader", Hashes: []string{"d", "c"}},
			{Role: "run", Name: "1oader", Hashes: []string{"e", "d"}},
			{Role: "recovery", Name: "loader", Hashes: []string{"e", "d"}},
			{Role: "run", Name: "0oader", Hashes: []string{"z", "x"}},
		},
	}

	d, err := json.Marshal(bc)
	c.Assert(err, IsNil)
	// boot assets sorted, hash lists in respective boot asset are sorted as
	// well
	c.Check(string(d), Equals, `{"model":"","brand-id":"","grade":"","model-sign-key-id":"","asset-chain":[{"role":"recovery","name":"loader","hashes":["d","e"]},{"role":"recovery","name":"shim","hashes":["b"]},{"role":"run","name":"0oader","hashes":["x","z"]},{"role":"run","name":"1oader","hashes":["d","e"]},{"role":"run","name":"loader","hashes":["z"]},{"role":"run","name":"loader","hashes":["c","d"]}],"kernel":"","kernel-revision":"","kernel-cmdline":""}`)
}

func (s *sealSuite) TestBootChainMarshalFull(c *C) {
	bc := &boot.BootChain{
		Model:          "foo",
		BrandID:        "mybrand",
		Grade:          "dangerous",
		ModelSignKeyID: "my-key-id",
		// asset chain will get sorted when marshaling
		AssetChain: []boot.BootAsset{
			{Role: "run", Name: "loader", Hashes: []string{"c", "d"}},
			// hash list will get sorted
			{Role: "recovery", Name: "shim", Hashes: []string{"b", "a"}},
			{Role: "recovery", Name: "loader", Hashes: []string{"d"}},
		},
		Kernel:         "pc-kernel",
		KernelRevision: "1234",
		KernelCmdline:  `foo=bar baz=0x123`,
	}

	d, err := json.Marshal(bc)
	c.Assert(err, IsNil)
	c.Check(string(d), Equals, `{"model":"foo","brand-id":"mybrand","grade":"dangerous","model-sign-key-id":"my-key-id","asset-chain":[{"role":"recovery","name":"loader","hashes":["d"]},{"role":"recovery","name":"shim","hashes":["a","b"]},{"role":"run","name":"loader","hashes":["c","d"]}],"kernel":"pc-kernel","kernel-revision":"1234","kernel-cmdline":"foo=bar baz=0x123"}`)
	// original structure has not been modified
	c.Check(bc, DeepEquals, &boot.BootChain{
		Model:          "foo",
		BrandID:        "mybrand",
		Grade:          "dangerous",
		ModelSignKeyID: "my-key-id",
		// asset chain will get sorted when marshaling
		AssetChain: []boot.BootAsset{
			{Role: "run", Name: "loader", Hashes: []string{"c", "d"}},
			// hash list will get sorted
			{Role: "recovery", Name: "shim", Hashes: []string{"b", "a"}},
			{Role: "recovery", Name: "loader", Hashes: []string{"d"}},
		},
		Kernel:         "pc-kernel",
		KernelRevision: "1234",
		KernelCmdline:  `foo=bar baz=0x123`,
	})
}

func (s *sealSuite) TestBootChainEqualForResealComplex(c *C) {
	bc := &boot.BootChain{
		Model:          "foo",
		BrandID:        "mybrand",
		Grade:          "dangerous",
		ModelSignKeyID: "my-key-id",
		AssetChain: []boot.BootAsset{
			{Role: "run", Name: "loader", Hashes: []string{"c", "d"}},
			// hash list will get sorted
			{Role: "recovery", Name: "shim", Hashes: []string{"b", "a"}},
			{Role: "recovery", Name: "loader", Hashes: []string{"d"}},
		},
		Kernel:         "pc-kernel",
		KernelRevision: "1234",
		KernelCmdline:  `foo=bar baz=0x123`,
	}
	// sorted variant
	bcOther := &boot.BootChain{
		Model:          "foo",
		BrandID:        "mybrand",
		Grade:          "dangerous",
		ModelSignKeyID: "my-key-id",
		AssetChain: []boot.BootAsset{
			{Role: "recovery", Name: "loader", Hashes: []string{"d"}},
			{Role: "recovery", Name: "shim", Hashes: []string{"a", "b"}},
			{Role: "run", Name: "loader", Hashes: []string{"c", "d"}},
		},
		Kernel:         "pc-kernel",
		KernelRevision: "1234",
		KernelCmdline:  `foo=bar baz=0x123`,
	}

	eq := bc.EqualForReseal(bcOther)
	c.Check(eq, Equals, true, Commentf("not equal\none: %v\nother: %v", bc, bcOther))
	// original structure is unodified
	c.Check(bc, DeepEquals, &boot.BootChain{
		Model:          "foo",
		BrandID:        "mybrand",
		Grade:          "dangerous",
		ModelSignKeyID: "my-key-id",
		AssetChain: []boot.BootAsset{
			{Role: "run", Name: "loader", Hashes: []string{"c", "d"}},
			// hash list will get sorted
			{Role: "recovery", Name: "shim", Hashes: []string{"b", "a"}},
			{Role: "recovery", Name: "loader", Hashes: []string{"d"}},
		},
		Kernel:         "pc-kernel",
		KernelRevision: "1234",
		KernelCmdline:  `foo=bar baz=0x123`,
	})
}

func (s *sealSuite) TestBootChainEqualForResealSimple(c *C) {
	var bcNil *boot.BootChain

	bc := &boot.BootChain{
		Model:          "foo",
		BrandID:        "mybrand",
		Grade:          "dangerous",
		ModelSignKeyID: "my-key-id",
		AssetChain: []boot.BootAsset{
			{Role: "run", Name: "loader", Hashes: []string{"c", "d"}},
		},
		Kernel:         "pc-kernel-other",
		KernelRevision: "1234",
		KernelCmdline:  `foo`,
	}
	c.Check(bc.EqualForReseal(bc), Equals, true)

	c.Check(bc.EqualForReseal(bcNil), Equals, false)
	c.Check(bcNil.EqualForReseal(bc), Equals, false)

	c.Check(bcNil.EqualForReseal(nil), Equals, true)

	bcOtherGrade := &boot.BootChain{
		Model:          "foo",
		BrandID:        "mybrand",
		Grade:          "signed",
		ModelSignKeyID: "my-key-id",
		AssetChain: []boot.BootAsset{
			{Role: "run", Name: "loader", Hashes: []string{"c", "d"}},
		},
		Kernel:         "pc-kernel-other",
		KernelRevision: "1234",
		KernelCmdline:  `foo`,
	}
	c.Check(bcOtherGrade.EqualForReseal(bc), Equals, false)
	c.Check(bc.EqualForReseal(bcOtherGrade), Equals, false)

	bcOtherAssets := &boot.BootChain{
		Model:          "foo",
		BrandID:        "mybrand",
		Grade:          "signed",
		ModelSignKeyID: "my-key-id",
		AssetChain: []boot.BootAsset{
			// one asset hash differs
			{Role: "run", Name: "loader", Hashes: []string{"c", "f"}},
		},
		Kernel:         "pc-kernel-other",
		KernelRevision: "1234",
		KernelCmdline:  `foo`,
	}
	c.Check(bcOtherAssets.EqualForReseal(bc), Equals, false)
	c.Check(bc.EqualForReseal(bcOtherAssets), Equals, false)
}

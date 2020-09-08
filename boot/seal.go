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

package boot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/bootloader"
	"github.com/snapcore/snapd/secboot"
)

var (
	secbootSealKey = secboot.SealKey
)

// sealKeyToModeenv seals the supplied key to the parameters specified
// in modeenv.
func sealKeyToModeenv(key secboot.EncryptionKey, model *asserts.Model, modeenv *Modeenv) error {
	// TODO:UC20: binaries are EFI/bootloader-specific, hardcoded for now
	loadChain := []bootloader.BootFile{
		// the path to the shim EFI binary
		bootloader.NewBootFile("", filepath.Join(InitramfsUbuntuSeedDir, "EFI/boot/bootx64.efi"), bootloader.RoleRecovery),
		// the path to the recovery grub EFI binary
		bootloader.NewBootFile("", filepath.Join(InitramfsUbuntuSeedDir, "EFI/boot/grubx64.efi"), bootloader.RoleRecovery),
		// the path to the run mode grub EFI binary
		bootloader.NewBootFile("", filepath.Join(InitramfsUbuntuBootDir, "EFI/boot/grubx64.efi"), bootloader.RoleRunMode),
	}
	kernelPath := filepath.Join(InitramfsUbuntuBootDir, "EFI/ubuntu/kernel.efi")
	loadChain = append(loadChain, bootloader.NewBootFile("", kernelPath, bootloader.RoleRunMode))

	// Get the expected kernel command line for the system that is currently being installed
	cmdline, err := ComposeCandidateCommandLine(model)
	if err != nil {
		return fmt.Errorf("cannot obtain kernel command line: %v", err)
	}

	// Get the expected kernel command line of the recovery system we're installing from
	recoveryCmdline, err := ComposeRecoveryCommandLine(model, modeenv.RecoverySystem)
	if err != nil {
		return fmt.Errorf("cannot obtain recovery kernel command line: %v", err)
	}

	kernelCmdlines := []string{
		cmdline,
		recoveryCmdline,
	}

	sealKeyParams := secboot.SealKeyParams{
		ModelParams: []*secboot.SealKeyModelParams{
			{
				Model:          model,
				KernelCmdlines: kernelCmdlines,
				EFILoadChains:  [][]bootloader.BootFile{loadChain},
			},
		},
		KeyFile:                 filepath.Join(InitramfsEncryptionKeyDir, "ubuntu-data.sealed-key"),
		TPMPolicyUpdateDataFile: filepath.Join(InstallHostFDEDataDir, "policy-update-data"),
		TPMLockoutAuthFile:      filepath.Join(InstallHostFDEDataDir, "tpm-lockout-auth"),
	}

	if err := secbootSealKey(key, &sealKeyParams); err != nil {
		return fmt.Errorf("cannot seal the encryption key: %v", err)
	}

	return nil
}

type bootChain struct {
	Model          string      `json:"model"`
	BrandID        string      `json:"brand-id"`
	Grade          string      `json:"grade"`
	ModelSignKeyID string      `json:"model-sign-key-id"`
	AssetChain     []bootAsset `json:"asset-chain"`
	Kernel         string      `json:"kernel"`
	// KernelRevision is the revision of the kernel snap. It is empty if
	// kernel is unasserted, in which case always reseal.
	KernelRevision string `json:"kernel-revision"`
	KernelCmdline  string `json:"kernel-cmdline"`
}

type bootAsset struct {
	Role   string   `json:"role"`
	Name   string   `json:"name"`
	Hashes []string `json:"hashes"`
}

// helper types
type sortedHashesBootAsset bootAsset
type sortedAssetsBootChain bootChain
type predictableBootChains []bootChain

// copyForMarshalling copies the contents of boot asset and sorts the hash list.
func (b *bootAsset) copyForMarshalling() *sortedHashesBootAsset {
	newB := *b
	newB.Hashes = make([]string, len(b.Hashes))
	copy(newB.Hashes, b.Hashes)
	sort.Strings(newB.Hashes)
	return (*sortedHashesBootAsset)(&newB)
}

func (b *bootAsset) less(other *bootAsset) bool {
	byRole := b.Role < other.Role
	byName := b.Name < other.Name
	// sort order: role -> name -> hash list (len -> lexical)
	if b.Role != other.Role {
		return byRole
	}
	if b.Name != other.Name {
		return byName
	}
	return lessByHashList(b.Hashes, other.Hashes)
}

// // MarshalJSON marshals the boot asset into a form that is deterministic and
// // suitable for equivalence tests.
// func (b *bootAsset) MarshalJSON() ([]byte, error) {
// 	s := (*sortedHashesBootAsset)(b.copyForMarshalling())
// 	return json.Marshal(s)
// }

type byBootAssetOrder []bootAsset

func (b byBootAssetOrder) Len() int      { return len(b) }
func (b byBootAssetOrder) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byBootAssetOrder) Less(i, j int) bool {
	return b[i].less(&(b[j]))
}

func lessByHashList(h1, h2 []string) bool {
	if len(h1) != len(h2) {
		return len(h1) < len(h2)
	}
	for idx := range h1 {
		if h1[idx] < h2[idx] {
			return true
		}
	}
	return false
}

// copyForMarshalling copies the contents of bootChain and sorts boot assets.
func (b *bootChain) copyForMarshalling() *sortedAssetsBootChain {
	newB := *b
	newB.AssetChain = make([]bootAsset, len(b.AssetChain))
	for i := range b.AssetChain {
		newB.AssetChain[i] = bootAsset(*b.AssetChain[i].copyForMarshalling())
	}
	return (*sortedAssetsBootChain)(&newB)
}

// MarshalJSON marshals the boot chain into a form that is deterministic and
// suitable for equivalence tests.
func (b *bootChain) MarshalJSON() ([]byte, error) {
	s := b.copyForMarshalling()
	sort.Sort(byBootAssetOrder(s.AssetChain))
	return json.Marshal(s)
}

// equal returns true when boot chains are equivalent for reseal.
func (b *bootChain) equalForReseal(other *bootChain) bool {
	bJSON, err := json.Marshal(b)
	if err != nil {
		return false
	}
	otherJSON, err := json.Marshal(other)
	if err != nil {
		return false
	}
	return bytes.Equal(bJSON, otherJSON)
}

func lessSortedBootAssets(b1, b2 []bootAsset) bool {
	if len(b1) != len(b2) {
		return len(b1) < len(b2)
	}
	for i := range b1 {
		if b1[i].less(&(b2[i])) {
			return true
		}
	}
	return false
}

type byBootChainOrder []sortedAssetsBootChain

func (b byBootChainOrder) Len() int      { return len(b) }
func (b byBootChainOrder) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byBootChainOrder) Less(i, j int) bool {
	// sort order: role -> name -> hash list (len -> lexical)
	if b[i].Model != b[j].Model {
		return b[i].Model < b[j].Model
	}
	if b[i].BrandID != b[j].BrandID {
		return b[i].BrandID < b[j].BrandID
	}
	if b[i].Grade != b[j].Grade {
		return b[i].Grade < b[j].Grade
	}
	if b[i].ModelSignKeyID != b[j].ModelSignKeyID {
		return b[i].ModelSignKeyID < b[j].ModelSignKeyID
	}
	if b[i].Kernel != b[j].Kernel {
		return b[i].Kernel < b[j].Kernel
	}
	if b[i].KernelRevision != b[j].KernelRevision {
		return b[i].KernelRevision < b[j].KernelRevision
	}
	if b[i].KernelCmdline != b[j].KernelCmdline {
		return b[i].KernelCmdline < b[j].KernelCmdline
	}
	return lessSortedBootAssets(b[i].AssetChain, b[j].AssetChain)
}

func (s predictableBootChains) MarshalJSON() ([]byte, error) {
	bootChains := make([]sortedAssetsBootChain, len(s))
	for i := range s {
		bootChains[i] = *(s[i].copyForMarshalling())
	}
	sort.Sort(byBootChainOrder(bootChains))
	return json.Marshal(bootChains)
}

// -*- Mode: Go; indent-tabs-mode: t -*-
// +build !nosecboot

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

package install

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/snapcore/snapd/boot"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/kernel/fde"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/secboot"
)

var (
	secbootFormatEncryptedDevice = secboot.FormatEncryptedDevice
	secbootAddRecoveryKey        = secboot.AddRecoveryKey

	bootRunFDESetupHook = boot.RunFDESetupHook
)

// encryptedDeviceCryptsetup represents a encrypted block device.
type encryptedDevice interface {
	Node() string
	AddRecoveryKey(key secboot.EncryptionKey, rkey secboot.RecoveryKey) error
	Close() error
}

// encryptedDeviceLUKS represents a LUKS-backed encrypted block device.
type encryptedDeviceLUKS struct {
	parent *gadget.OnDiskStructure
	name   string
	node   string
}

// sanity
var _ = encryptedDevice(&encryptedDeviceLUKS{})

// newEncryptedDevice creates an encrypted device in the existing partition using the
// specified key.
func newEncryptedDeviceLUKS(part *gadget.OnDiskStructure, key secboot.EncryptionKey, name string) (encryptedDevice, error) {
	dev := &encryptedDeviceLUKS{
		parent: part,
		name:   name,
		// A new block device is used to access the encrypted data. Note that
		// you can't open an encrypted device under different names and a name
		// can't be used in more than one device at the same time.
		node: fmt.Sprintf("/dev/mapper/%s", name),
	}

	if err := secbootFormatEncryptedDevice(key, name+"-enc", part.Node); err != nil {
		return nil, fmt.Errorf("cannot format encrypted device: %v", err)
	}

	if err := cryptsetupOpen(key, part.Node, name); err != nil {
		return nil, fmt.Errorf("cannot open encrypted device on %s: %s", part.Node, err)
	}

	return dev, nil
}

func (dev *encryptedDeviceLUKS) AddRecoveryKey(key secboot.EncryptionKey, rkey secboot.RecoveryKey) error {
	return secbootAddRecoveryKey(key, rkey, dev.parent.Node)
}

func (dev *encryptedDeviceLUKS) Node() string {
	return dev.node
}

func (dev *encryptedDeviceLUKS) Close() error {
	return cryptsetupClose(dev.name)
}

func cryptsetupOpen(key secboot.EncryptionKey, node, name string) error {
	cmd := exec.Command("cryptsetup", "open", "--key-file", "-", node, name)
	cmd.Stdin = bytes.NewReader(key[:])
	if output, err := cmd.CombinedOutput(); err != nil {
		return osutil.OutputErr(output, err)
	}
	return nil
}

func cryptsetupClose(name string) error {
	if output, err := exec.Command("cryptsetup", "close", name).CombinedOutput(); err != nil {
		return osutil.OutputErr(output, err)
	}
	return nil
}

// encryptedDeviceWithSetupHook represents a block device that is setup using
// the "device-setup" hook.
type encryptedDeviceWithSetupHook struct {
	parent *gadget.OnDiskStructure
	name   string
	node   string
}

// sanity
var _ = encryptedDevice(&encryptedDeviceWithSetupHook{})

// newEncryptedDeviceWithSetupHook creates an encrypted device in the
// existing partition using the specified key using the fde-setup hook
func newEncryptedDeviceWithSetupHook(part *gadget.OnDiskStructure, key secboot.EncryptionKey, name string) (encryptedDevice, error) {
	dev := &encryptedDeviceWithSetupHook{
		parent: part,
		name:   name,
		node:   fmt.Sprintf("/dev/mapper/%s", name),
	}

	// 1. create linear mapper device: 1M offset in 512 byte blocks
	offsetInBlocks := 1 * 1024 * 1024 / 512
	sizeWithoutOffsetInBlocks := (int(part.Size) / 512) - offsetInBlocks
	dmTable := fmt.Sprintf("0 %v linear %s %v", sizeWithoutOffsetInBlocks, part.Node, offsetInBlocks)
	cmd := exec.Command("dmsetup", "create", name, "--table", dmTable)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("cannot create mapping device on %v: %v", part.Node, osutil.OutputErr(output, err))
	}

	// 2. run fde-setup "device-setup" on it
	// TODO: We may need a different way to run the fde-setup hook
	//       here. The hook right now runs with a locked state. But
	//       when this runs the state will be unlocked but our hook
	//       mechanism needs a locked state. This means we either need
	//       something like "boot.RunFDE*Device*SetupHook" or we run
	//       the entire install with the state locked (which may not
	//       be as terrible as it sounds as this is a rare situation).
	runHook := bootRunFDESetupHook
	params := &fde.DeviceSetupParams{
		Key:    key,
		Device: name,
	}
	if err := fde.DeviceSetup(runHook, params); err != nil {
		return nil, err
	}

	return dev, nil
}

func (dev *encryptedDeviceWithSetupHook) Close() error {
	if output, err := exec.Command("dmsetup", "remove", dev.name).CombinedOutput(); err != nil {
		return osutil.OutputErr(output, err)
	}
	return nil
}

func (dev *encryptedDeviceWithSetupHook) Node() string {
	return dev.node
}

func (dev *encryptedDeviceWithSetupHook) AddRecoveryKey(key secboot.EncryptionKey, rkey secboot.RecoveryKey) error {
	return fmt.Errorf("recovery keys are not supported on devices that use the device-setup hook")
}

// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2022 Canonical Ltd
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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"

	"github.com/jessevdk/go-flags"

	"github.com/snapcore/snapd/secboot/keymgr"
	"github.com/snapcore/snapd/secboot/keys"
)

var osStdin io.Reader = os.Stdin

type commonDeviceMixin struct {
	// TODO: support for multiple devices in the command line
	Device string `long:"device" description:"encrypted device" required:"yes"`
}

type cmdAddRecoveryKey struct {
	commonDeviceMixin
	KeyFile string `long:"key-file" description:"path to recovery key file" required:"yes"`
}

type cmdRemoveRecoveryKey struct {
	commonDeviceMixin
	KeyFile string `long:"key-file" description:"path to recovery key file" required:"yes"`
}

type cmdChangeEncryptionKey struct {
	commonDeviceMixin
}

type options struct {
	CmdAddRecoveryKey      cmdAddRecoveryKey      `command:"add-recovery-key"`
	CmdRemoveRecoveryKey   cmdRemoveRecoveryKey   `command:"remove-recovery-key"`
	CmdChangeEncryptionKey cmdChangeEncryptionKey `command:"change-encryption-key"`
}

var (
	keymgrAddRecoveryKeyToLUKSDevice      = keymgr.AddRecoveryKeyToLUKSDevice
	keymgrRemoveRecoveryKeyFromLUKSDevice = keymgr.RemoveRecoveryKeyFromLUKSDevice
	keymgrChangeLUKSDeviceEncryptionKey   = keymgr.ChangeLUKSDeviceEncryptionKey
)

func writeIfNotExists(p string, data []byte) (alreadyExists bool, err error) {
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			// assuming that the file is identical
			return true, nil
		}
		return false, err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return false, err
	}
	return false, f.Close()
}

var keyslotFull = regexp.MustCompile(`^.* cryptsetup failed with: Key slot [0-9]+ is full, please select another one\.$`)

func isKeyslotFull(err error) bool {
	if err == nil {
		return false
	}
	return keyslotFull.MatchString(err.Error())
}

func (c *cmdAddRecoveryKey) Execute(args []string) error {
	recoveryKey, err := keys.NewRecoveryKey()
	if err != nil {
		return fmt.Errorf("cannot create recovery key: %v", err)
	}
	alreadyExists, err := writeIfNotExists(c.KeyFile, recoveryKey[:])
	if err != nil {
		return fmt.Errorf("cannot write recovery key to file: %v", err)
	}
	if alreadyExists {
		// we already have the recovery key, read it back
		maybeKey, err := ioutil.ReadFile(c.KeyFile)
		if err != nil {
			return fmt.Errorf("cannot read existing file: %v", err)
		}
		// TODO: verify that the size if non 0 and try again otherwise?
		if len(maybeKey) != len(recoveryKey) {
			return fmt.Errorf("cannot use current recovery key of size %v", len(maybeKey))
		}
		copy(recoveryKey[:], maybeKey[:len(recoveryKey)])
	}
	if err := keymgrAddRecoveryKeyToLUKSDevice(recoveryKey, c.Device); err != nil {
		if !alreadyExists || !isKeyslotFull(err) {
			return fmt.Errorf("cannot add recovery key to LUKS device: %v", err)
		}
	}
	return nil
}

func (c *cmdRemoveRecoveryKey) Execute(args []string) error {
	if err := keymgrRemoveRecoveryKeyFromLUKSDevice(c.Device); err != nil {
		return fmt.Errorf("cannot remove recovery key from LUKS device: %v", err)
	}
	if err := os.Remove(c.KeyFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove recovery key file: %v", err)
	}
	return nil
}

type newKey struct {
	Key []byte `json:"key"`
}

func (c *cmdChangeEncryptionKey) Execute(args []string) error {
	// TODO: encryption key from stdin
	var newEncryptionKeyData newKey
	// TODO read from stdin
	dec := json.NewDecoder(osStdin)
	if err := dec.Decode(&newEncryptionKeyData); err != nil {
		return fmt.Errorf("cannot obtain new encryption key: %v", err)
	}
	if err := keymgrChangeLUKSDeviceEncryptionKey(newEncryptionKeyData.Key, c.Device); err != nil {
		return fmt.Errorf("cannot change LUKS device encryption key: %v", err)
	}
	return nil
}

func run(osArgs1 []string) error {
	var opts options
	p := flags.NewParser(&opts, flags.HelpFlag|flags.PassDoubleDash)
	if _, err := p.ParseArgs(osArgs1); err != nil {
		return err
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

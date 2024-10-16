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
package fdestate

import (
	"crypto"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/boot"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/gadget/device"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/osutil/disks"
	"github.com/snapcore/snapd/overlord/fdestate/backend"
	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/secboot"
)

var errNotImplemented = errors.New("not implemented")

var (
	disksDMCryptUUIDFromMountPoint = disks.DMCryptUUIDFromMountPoint
	secbootGetPrimaryKeyHash       = secboot.GetPrimaryKeyHash
	secbootVerifyPrimaryKeyHash    = secboot.VerifyPrimaryKeyHash
)

// EFISecureBootDBManagerStartup indicates that the local EFI key database
// manager has started.
func EFISecureBootDBManagerStartup(st *state.State) error {
	method, err := device.SealedKeysMethod(dirs.GlobalRootDir)
	if err == device.ErrNoSealedKeys {
		return nil
	}

	st.Lock()
	defer st.Unlock()

	chg, err := findEFISecurebootDBUpdateChange(st)
	if err != nil {
		return err
	}

	if chg == nil {
		logger.Debugf("no pending DBX update request")
		return nil
	}

	st.Unlock()
	err = postUpdateReseal(st, method, "efi-secureboot-update-startup")
	st.Lock()
	if err != nil {
		return fmt.Errorf("cannot complete post update reseal in startup action: %w", err)
	}

	return abortEFISecurebootDBUpdateChange(chg)
}

type EFISecurebootKeyDatabase int

const (
	EFISecurebootPK EFISecurebootKeyDatabase = iota
	EFISecurebootKEK
	EFISecurebootDB
	EFISecurebootDBX
)

// EFISecureBootDBUpdatePrepare notifies notifies that the local EFI key
// database manager is about to update the database.
func EFISecureBootDBUpdatePrepare(st *state.State, db EFISecurebootKeyDatabase, payload []byte) error {
	method, err := device.SealedKeysMethod(dirs.GlobalRootDir)
	if err != nil {
		if err == device.ErrNoSealedKeys {
			return nil
		}
		return err
	}

	st.Lock()
	defer st.Unlock()

	if err := addEFISecurebootDBUpdateChange(st, payload); err != nil {
		// TODO error could indicate conflict, perhaps use a dedicated error
		// value for this
		return err
	}

	st.Unlock()
	err = boot.WithModeenv(func(m *boot.Modeenv) error {
		bc, err := boot.BootChains(m)
		if err != nil {
			return err
		}

		logger.Debugf("attempting reseal")
		logger.Debugf("boot chains: %v\n", bc)
		logger.Debugf("DBX update payload: %x", payload)

		// TODO use a different helper for resealing
		return backend.ResealKeyForSignaturesDBUpdate(
			unlockedStateUpdater(st, "efi-secureboot-update-db-prepare"),
			method, dirs.GlobalRootDir, bc, payload,
		)
	})
	st.Lock()

	if err != nil {
		return err
	}

	return nil
}

// EFISecureBootDBUpdateCleanup notifies that the local EFI key database manager
// has reached a cleanup stage of the update process.
func EFISecureBootDBUpdateCleanup(st *state.State) error {
	method, err := device.SealedKeysMethod(dirs.GlobalRootDir)
	if err == device.ErrNoSealedKeys {
		return nil
	}

	st.Lock()
	defer st.Unlock()

	chg, err := findEFISecurebootDBUpdateChange(st)
	if err != nil {
		return err
	}

	if chg == nil {
		logger.Debugf("no pending DBX update request for cleanup")
		return nil
	}

	st.Unlock()
	err = postUpdateReseal(st, method, "efi-secureboot-update-db-cleanup")
	st.Lock()
	if err != nil {
		return fmt.Errorf("cannot complete post update reseal in cleanup action: %w", err)
	}

	return cleanupEFISecurebootDBUpdateChange(chg)
}

// Model is a json serializable secboot.ModelForSealing
type Model struct {
	SeriesValue    string             `json:"series"`
	BrandIDValue   string             `json:"brand-id"`
	ModelValue     string             `json:"model"`
	ClassicValue   bool               `json:"classic"`
	GradeValue     asserts.ModelGrade `json:"grade"`
	SignKeyIDValue string             `json:"sign-key-id"`
}

// Series implements secboot.ModelForSealing.Series
func (m *Model) Series() string {
	return m.SeriesValue
}

// BrandID implements secboot.ModelForSealing.BrandID
func (m *Model) BrandID() string {
	return m.BrandIDValue
}

// Model implements secboot.ModelForSealing.Model
func (m *Model) Model() string {
	return m.ModelValue
}

// Classic implements secboot.ModelForSealing.Classic
func (m *Model) Classic() bool {
	return m.ClassicValue
}

// Grade implements secboot.ModelForSealing.Grade
func (m *Model) Grade() asserts.ModelGrade {
	return m.GradeValue
}

// SignKeyID implements secboot.ModelForSealing.SignKeyID
func (m *Model) SignKeyID() string {
	return m.SignKeyIDValue
}

func newModel(m secboot.ModelForSealing) Model {
	return Model{
		SeriesValue:    m.Series(),
		BrandIDValue:   m.BrandID(),
		ModelValue:     m.Model(),
		ClassicValue:   m.Classic(),
		GradeValue:     m.Grade(),
		SignKeyIDValue: m.SignKeyID(),
	}
}

func toModels(sealingModels []secboot.ModelForSealing) (models []Model) {
	models = make([]Model, 0, len(sealingModels))

	for _, sm := range sealingModels {
		models = append(models, Model{
			SeriesValue:    sm.Series(),
			BrandIDValue:   sm.BrandID(),
			ModelValue:     sm.Model(),
			ClassicValue:   sm.Classic(),
			GradeValue:     sm.Grade(),
			SignKeyIDValue: sm.SignKeyID(),
		})
	}

	return models
}

var _ secboot.ModelForSealing = (*Model)(nil)

// KeyslotRoleParameters stores upgradeable parameters for a keyslot role
type KeyslotRoleParameters struct {
	// Models are the optional list of approved models
	Models []Model `json:"models,omitempty"`
	// BootModes are the optional list of approved modes (run, recover, ...)
	BootModes []string `json:"boot-modes,omitempty"`
	// TPM2PCRProfile is an optional serialized PCR profile
	TPM2PCRProfile secboot.SerializedPCRProfile `json:"tpm2-pcr-profile,omitempty"`
}

// KeyslotRoleInfo stores information about a key slot role
type KeyslotRoleInfo struct {
	// PrimaryKeyID is the ID for the primary key found in
	// PrimaryKeys field of FdeState
	PrimaryKeyID int `json:"primary-key-id"`
	// Parameters is indexed by container role name
	Parameters map[string]KeyslotRoleParameters `json:"params,omitempty"`
	// TPM2PCRPolicyRevocationCounter is the handle for the TPM
	// policy revocation counter.  A value of 0 means it is not
	// set.
	TPM2PCRPolicyRevocationCounter uint32 `json:"tpm2-pcr-policy-revocation-counter,omitempty"`
}

type hashAlg crypto.Hash

// UnmarshalJSON implements json.Unmarshaler.UnmarshalJSON
func (h *hashAlg) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case "sha256":
		*h = hashAlg(crypto.SHA256)
	default:
		return fmt.Errorf("unknown algorithm %s", s)
	}

	return nil
}

// MarshalJSON implements json.Marshaler.MarshalJSON
func (h hashAlg) MarshalJSON() ([]byte, error) {
	switch crypto.Hash(h) {
	case crypto.SHA256:
		return json.Marshal("sha256")
	default:
		return nil, fmt.Errorf("unknown algorithm %v", h)
	}
}

// KeyDigest stores a HMAC(key, salt) of a key
type KeyDigest struct {
	// Algorithm is the algorithm for
	Algorithm hashAlg `json:"alg"`
	// Salt is the salt for the HMAC digest
	Salt []byte `json:"salt"`
	// Digest is the result of `HMAC(key, salt)`
	Digest []byte `json:"digest"`
}

const defaultHashAlg = crypto.SHA256

func getPrimaryKeyDigest(devicePath string) (KeyDigest, error) {
	salt, digest, err := secbootGetPrimaryKeyHash(devicePath, crypto.Hash(defaultHashAlg))
	if err != nil {
		return KeyDigest{}, err
	}

	return KeyDigest{
		Algorithm: hashAlg(defaultHashAlg),
		Salt:      salt,
		Digest:    digest,
	}, nil
}

func (kd *KeyDigest) verifyPrimaryKeyDigest(devicePath string) (bool, error) {
	return secbootVerifyPrimaryKeyHash(devicePath, crypto.Hash(kd.Algorithm), kd.Salt, kd.Digest)
}

// PrimaryKeyInfo provides information about a primary key that is used to manage key slots
type PrimaryKeyInfo struct {
	Digest KeyDigest `json:"digest"`
}

type externalOperation struct {
	Kind     string           `json:"kind"`
	ChangeID string           `json:"change-id"`
	Context  *json.RawMessage `json:"context"`
}

// FdeState is the root persistent FDE state
type FdeState struct {
	// PrimaryKeys are the keys on the system. Key with ID 0 is
	// reserved for snapd and is populated on first boot. Other
	// IDs are for externally managed keys.
	PrimaryKeys map[int]PrimaryKeyInfo `json:"primary-keys"`

	// KeyslotRoles are all keyslot roles indexed by the role name
	KeyslotRoles map[string]KeyslotRoleInfo `json:"keyslot-roles"`

	// PendingExternalOperations keeps a list of changes that capture FDE
	// related operations running outside of snapd.
	PendingExternalOperations []externalOperation `json:"pending-external-operations"`
}

const fdeStateKey = "fde"

func initializeState(st *state.State) error {
	var s FdeState
	err := st.Get(fdeStateKey, &s)
	if err == nil {
		// TODO: Do we need to do something in recover?
		return nil
	}

	if !errors.Is(err, state.ErrNoState) {
		return err
	}

	dataUUID, dataErr := disksDMCryptUUIDFromMountPoint(dirs.SnapdStateDir(dirs.GlobalRootDir))
	saveUUID, saveErr := disksDMCryptUUIDFromMountPoint(dirs.SnapSaveDir)
	logger.Debugf("data UUID: %q data err: %v", dataUUID, dataErr)
	logger.Debugf("save UUID: %q save err: %v", saveUUID, saveErr)
	if errors.Is(saveErr, disks.ErrMountPointNotFound) {
		// TODO: do we need to care about old cases where there is no save partition?
		return nil
	}

	if errors.Is(dataErr, disks.ErrNoDmUUID) && errors.Is(saveErr, disks.ErrNoDmUUID) {
		// There is no encryption, so we ignore it.
		// TODO: we should verify the device "sealed key method"
		return nil
	}
	if dataErr != nil {
		return dataErr
	}
	if saveErr != nil {
		return saveErr
	}

	devpData := fmt.Sprintf("/dev/disk/by-uuid/%s", dataUUID)
	devpSave := fmt.Sprintf("/dev/disk/by-uuid/%s", saveUUID)
	digest, err := getPrimaryKeyDigest(devpData)
	if err != nil {
		return fmt.Errorf("cannot obtain primary key digest for data device %s: %w", devpData, err)
	}
	// TODO: restore key verification once we know that it is always added to
	// the keyring
	sameDigest, err := digest.verifyPrimaryKeyDigest(devpSave)
	if err != nil {
		if !errors.Is(err, secboot.ErrKernelKeyNotFound) {
			return fmt.Errorf("cannot verify primary key digest for save device %s: %w", devpSave, err)
		} else {
			logger.Noticef("cannot verify primary key digest for save device %s: %v", devpSave, err)
		}
	} else {
		if !sameDigest {
			return fmt.Errorf("primary key for data and save partition are not the same")
		}
	}

	s.PrimaryKeys = map[int]PrimaryKeyInfo{
		0: {
			Digest: digest,
		},
	}

	// Note that Parameters will be updated on first update
	s.KeyslotRoles = map[string]KeyslotRoleInfo{
		// TODO: use a constant
		"run+recover": {
			PrimaryKeyID: 0,
			// FIXME: this might be
			// AltRunObjectPCRPolicyCounterHandle after
			// factory reset, but this is supposed to be
			// removed.
			TPM2PCRPolicyRevocationCounter: secboot.RunObjectPCRPolicyCounterHandle,
		},
		// TODO: use a constant
		"recover": {
			PrimaryKeyID:                   0,
			TPM2PCRPolicyRevocationCounter: secboot.FallbackObjectPCRPolicyCounterHandle,
		},
	}

	st.Set(fdeStateKey, s)

	return nil
}

func updateParameters(st *state.State, role string, containerRole string, bootModes []string, models []secboot.ModelForSealing, tpmPCRProfile []byte) error {
	var s FdeState
	err := st.Get(fdeStateKey, &s)
	if err != nil {
		return err
	}

	roleInfo, hasRole := s.KeyslotRoles[role]
	if !hasRole {
		return fmt.Errorf("cannot find keyslot role %s", role)
	}

	if roleInfo.Parameters == nil {
		roleInfo.Parameters = make(map[string]KeyslotRoleParameters)
	}

	roleInfo.Parameters[containerRole] = KeyslotRoleParameters{
		Models:         toModels(models),
		BootModes:      bootModes,
		TPM2PCRProfile: tpmPCRProfile,
	}

	s.KeyslotRoles[role] = roleInfo

	st.Set(fdeStateKey, s)

	return nil
}

func MockDMCryptUUIDFromMountPoint(f func(mountpoint string) (string, error)) (restore func()) {
	osutil.MustBeTestBinary("mocking DMCryptUUIDFromMountPoint can be done only from tests")

	old := disksDMCryptUUIDFromMountPoint
	disksDMCryptUUIDFromMountPoint = f
	return func() {
		disksDMCryptUUIDFromMountPoint = old
	}
}

func MockGetPrimaryKeyHash(f func(devicePath string, alg crypto.Hash) ([]byte, []byte, error)) (restore func()) {
	osutil.MustBeTestBinary("mocking GetPrimaryKeyHash can be done only from tests")

	old := secbootGetPrimaryKeyHash
	secbootGetPrimaryKeyHash = f
	return func() {
		secbootGetPrimaryKeyHash = old
	}
}

func MockVerifyPrimaryKeyHash(f func(devicePath string, alg crypto.Hash, salt []byte, digest []byte) (bool, error)) (restore func()) {
	osutil.MustBeTestBinary("mocking VerifyPrimaryKeyHash can be done only from tests")

	old := secbootVerifyPrimaryKeyHash
	secbootVerifyPrimaryKeyHash = f
	return func() {
		secbootVerifyPrimaryKeyHash = old
	}
}

// addEFISecurebootDBUpdateChange adds a state change releated to the DBX
// update. The state must be locked by the caller
func addEFISecurebootDBUpdateChange(st *state.State, payload []byte) error {
	var s FdeState
	err := st.Get(fdeStateKey, &s)
	if err != nil {
		return err
	}

	// TODO be specific about kind
	if len(s.PendingExternalOperations) > 0 {
		return fmt.Errorf("conflict")
	}

	chg := st.NewChange("fde-efi-secureboot-db-update", "EFI secure boot key database update")
	t := st.NewTask("efi-secureboot-db-update", "External EFI secure boot key database update")
	chg.AddTask(t)

	data, err := json.Marshal(map[string]any{
		"payload": base64.StdEncoding.EncodeToString(payload),
	})
	if err != nil {
		return err
	}

	opCtxRaw := json.RawMessage(data)

	opDesc := externalOperation{
		Kind:     "fde-efi-secureboot-db-update",
		ChangeID: chg.ID(),
		Context:  &opCtxRaw,
	}

	s.PendingExternalOperations = append(s.PendingExternalOperations, opDesc)

	st.Set(fdeStateKey, &s)

	return nil
}

func findEFISecurebootDBUpdateChange(st *state.State) (*state.Change, error) {
	var s FdeState
	err := st.Get(fdeStateKey, &s)
	if err != nil {
		return nil, err
	}

	if len(s.PendingExternalOperations) == 0 {
		logger.Debugf("requested to complete external FDE operation, but none is running")
		return nil, nil
	}

	op := s.PendingExternalOperations[0]

	chg := st.Change(op.ChangeID)
	tasks := chg.Tasks()
	// sensibility check
	if len(tasks) != 1 {
		return nil, fmt.Errorf("internal error: unexpected task count: %v", len(tasks))
	}

	return chg, nil
}

func completeEFISecurebootDBUpdateChange(chg *state.Change) error {
	st := chg.State()

	st.EnsureBefore(0)

	st.Unlock()
	logger.Debugf("waiting for FDE DBX change %v to become ready", chg.ID())
	<-chg.Ready()
	logger.Debugf("change complete")
	st.Lock()

	var s FdeState
	err := st.Get(fdeStateKey, &s)
	if err != nil {
		return err
	}

	// TODO drop the right operation
	s.PendingExternalOperations = nil

	st.Set(fdeStateKey, s)

	return nil
}

func cleanupEFISecurebootDBUpdateChange(chg *state.Change) error {
	tasks := chg.Tasks()
	if len(tasks) != 1 {
		return fmt.Errorf("internal error: unexpected task count: %v", len(tasks))
	}

	// unblock the task by simply marking it as done
	tasks[0].SetStatus(state.DoneStatus)

	return completeEFISecurebootDBUpdateChange(chg)
}

func abortEFISecurebootDBUpdateChange(chg *state.Change) error {
	// TODO: is there a need to abort the change?
	chg.Abort()

	tasks := chg.Tasks()
	if len(tasks) != 1 {
		return fmt.Errorf("internal error: unexpected task count: %v", len(tasks))
	}

	t := tasks[0]
	// TODO should this do something more fancy?
	t.Errorf("task aborted due to external call")
	t.SetStatus(state.ErrorStatus)

	return cleanupEFISecurebootDBUpdateChange(chg)
}

// postUpdateReseal performs a reseal after a DBX update.
func postUpdateReseal(st *state.State, method device.SealingMethod, action string) error {
	return boot.WithModeenv(func(m *boot.Modeenv) error {
		bc, err := boot.BootChains(m)
		if err != nil {
			return err
		}

		// unlock the state before resealing
		logger.Debugf("attempting post update reseal")

		// TODO use a different helper for resealing
		const expectReseal = true
		return backend.ResealKeyForBootChains(
			unlockedStateUpdater(st, action),
			method, dirs.GlobalRootDir, bc, expectReseal,
		)
	})
}

func unlockedStateUpdater(st *state.State, action string) backend.StateUpdater {
	return func(
		role string, containerRole string, bootModes []string,
		models []secboot.ModelForSealing, tpmPCRProfile []byte,
	) error {
		logger.Noticef("FDE state updater for action %q, role %q, container role %q",
			action, role, containerRole)

		st.Lock()
		defer st.Unlock()

		return updateParameters(st, role, containerRole, bootModes, models, tpmPCRProfile)
	}
}

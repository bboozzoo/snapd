// Code generated by MockGen. DO NOT EDIT.
// Source: .. (interfaces: SecbootBackend)
//
// Generated by this command:
//
//	mockgen -package secboottest -destination backend.go .. SecbootBackend
//

// Package secboottest is a generated GoMock package.
package secboottest

import (
	reflect "reflect"

	secboot "github.com/snapcore/secboot"
	tpm2 "github.com/snapcore/secboot/tpm2"
	secboot0 "github.com/snapcore/snapd/secboot"
	gomock "go.uber.org/mock/gomock"
)

// MockSecbootBackend is a mock of SecbootBackend interface.
type MockSecbootBackend struct {
	ctrl     *gomock.Controller
	recorder *MockSecbootBackendMockRecorder
	isgomock struct{}
}

// MockSecbootBackendMockRecorder is the mock recorder for MockSecbootBackend.
type MockSecbootBackendMockRecorder struct {
	mock *MockSecbootBackend
}

// NewMockSecbootBackend creates a new mock instance.
func NewMockSecbootBackend(ctrl *gomock.Controller) *MockSecbootBackend {
	mock := &MockSecbootBackend{ctrl: ctrl}
	mock.recorder = &MockSecbootBackendMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSecbootBackend) EXPECT() *MockSecbootBackendMockRecorder {
	return m.recorder
}

// AddLUKS2ContainerUnlockKey mocks base method.
func (m *MockSecbootBackend) AddLUKS2ContainerUnlockKey(devicePath, keyslotName string, existingKey, newKey secboot.DiskUnlockKey) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddLUKS2ContainerUnlockKey", devicePath, keyslotName, existingKey, newKey)
	ret0, _ := ret[0].(error)
	return ret0
}

// AddLUKS2ContainerUnlockKey indicates an expected call of AddLUKS2ContainerUnlockKey.
func (mr *MockSecbootBackendMockRecorder) AddLUKS2ContainerUnlockKey(devicePath, keyslotName, existingKey, newKey any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddLUKS2ContainerUnlockKey", reflect.TypeOf((*MockSecbootBackend)(nil).AddLUKS2ContainerUnlockKey), devicePath, keyslotName, existingKey, newKey)
}

// ListLUKS2ContainerUnlockKeyNames mocks base method.
func (m *MockSecbootBackend) ListLUKS2ContainerUnlockKeyNames(dev string) ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListLUKS2ContainerUnlockKeyNames", dev)
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListLUKS2ContainerUnlockKeyNames indicates an expected call of ListLUKS2ContainerUnlockKeyNames.
func (mr *MockSecbootBackendMockRecorder) ListLUKS2ContainerUnlockKeyNames(dev any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListLUKS2ContainerUnlockKeyNames", reflect.TypeOf((*MockSecbootBackend)(nil).ListLUKS2ContainerUnlockKeyNames), dev)
}

// NewFileKeyDataReader mocks base method.
func (m *MockSecbootBackend) NewFileKeyDataReader(kf string) (secboot.KeyDataReader, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewFileKeyDataReader", kf)
	ret0, _ := ret[0].(secboot.KeyDataReader)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewFileKeyDataReader indicates an expected call of NewFileKeyDataReader.
func (mr *MockSecbootBackendMockRecorder) NewFileKeyDataReader(kf any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewFileKeyDataReader", reflect.TypeOf((*MockSecbootBackend)(nil).NewFileKeyDataReader), kf)
}

// NewFileKeyDataWriter mocks base method.
func (m *MockSecbootBackend) NewFileKeyDataWriter(kf string) secboot.KeyDataWriter {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewFileKeyDataWriter", kf)
	ret0, _ := ret[0].(secboot.KeyDataWriter)
	return ret0
}

// NewFileKeyDataWriter indicates an expected call of NewFileKeyDataWriter.
func (mr *MockSecbootBackendMockRecorder) NewFileKeyDataWriter(kf any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewFileKeyDataWriter", reflect.TypeOf((*MockSecbootBackend)(nil).NewFileKeyDataWriter), kf)
}

// NewFileSealedKeyObjectWriter mocks base method.
func (m *MockSecbootBackend) NewFileSealedKeyObjectWriter(path string) secboot.KeyDataWriter {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewFileSealedKeyObjectWriter", path)
	ret0, _ := ret[0].(secboot.KeyDataWriter)
	return ret0
}

// NewFileSealedKeyObjectWriter indicates an expected call of NewFileSealedKeyObjectWriter.
func (mr *MockSecbootBackendMockRecorder) NewFileSealedKeyObjectWriter(path any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewFileSealedKeyObjectWriter", reflect.TypeOf((*MockSecbootBackend)(nil).NewFileSealedKeyObjectWriter), path)
}

// NewKeyData mocks base method.
func (m *MockSecbootBackend) NewKeyData(kd secboot0.SecbootKeyDataGetter) (secboot0.SecbootHooksKeyDataSetter, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewKeyData", kd)
	ret0, _ := ret[0].(secboot0.SecbootHooksKeyDataSetter)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewKeyData indicates an expected call of NewKeyData.
func (mr *MockSecbootBackendMockRecorder) NewKeyData(kd any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewKeyData", reflect.TypeOf((*MockSecbootBackend)(nil).NewKeyData), kd)
}

// NewKeyDataFromSealedKeyObjectFile mocks base method.
func (m *MockSecbootBackend) NewKeyDataFromSealedKeyObjectFile(kf string) (secboot0.SecbootKeyDataActor, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewKeyDataFromSealedKeyObjectFile", kf)
	ret0, _ := ret[0].(secboot0.SecbootKeyDataActor)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewKeyDataFromSealedKeyObjectFile indicates an expected call of NewKeyDataFromSealedKeyObjectFile.
func (mr *MockSecbootBackendMockRecorder) NewKeyDataFromSealedKeyObjectFile(kf any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewKeyDataFromSealedKeyObjectFile", reflect.TypeOf((*MockSecbootBackend)(nil).NewKeyDataFromSealedKeyObjectFile), kf)
}

// NewLUKS2KeyDataReader mocks base method.
func (m *MockSecbootBackend) NewLUKS2KeyDataReader(dev, slot string) (secboot.KeyDataReader, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewLUKS2KeyDataReader", dev, slot)
	ret0, _ := ret[0].(secboot.KeyDataReader)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewLUKS2KeyDataReader indicates an expected call of NewLUKS2KeyDataReader.
func (mr *MockSecbootBackendMockRecorder) NewLUKS2KeyDataReader(dev, slot any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewLUKS2KeyDataReader", reflect.TypeOf((*MockSecbootBackend)(nil).NewLUKS2KeyDataReader), dev, slot)
}

// NewLUKS2KeyDataWriter mocks base method.
func (m *MockSecbootBackend) NewLUKS2KeyDataWriter(dev, slot string) (secboot.KeyDataWriter, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewLUKS2KeyDataWriter", dev, slot)
	ret0, _ := ret[0].(secboot.KeyDataWriter)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewLUKS2KeyDataWriter indicates an expected call of NewLUKS2KeyDataWriter.
func (mr *MockSecbootBackendMockRecorder) NewLUKS2KeyDataWriter(dev, slot any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewLUKS2KeyDataWriter", reflect.TypeOf((*MockSecbootBackend)(nil).NewLUKS2KeyDataWriter), dev, slot)
}

// NewSealedKeyData mocks base method.
func (m *MockSecbootBackend) NewSealedKeyData(kd secboot0.SecbootKeyDataActor) (secboot0.SecbootSealedKeyDataActor, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewSealedKeyData", kd)
	ret0, _ := ret[0].(secboot0.SecbootSealedKeyDataActor)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewSealedKeyData indicates an expected call of NewSealedKeyData.
func (mr *MockSecbootBackendMockRecorder) NewSealedKeyData(kd any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewSealedKeyData", reflect.TypeOf((*MockSecbootBackend)(nil).NewSealedKeyData), kd)
}

// ReadKeyData mocks base method.
func (m *MockSecbootBackend) ReadKeyData(arg0 secboot.KeyDataReader) (secboot0.SecbootKeyDataActor, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadKeyData", arg0)
	ret0, _ := ret[0].(secboot0.SecbootKeyDataActor)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReadKeyData indicates an expected call of ReadKeyData.
func (mr *MockSecbootBackendMockRecorder) ReadKeyData(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadKeyData", reflect.TypeOf((*MockSecbootBackend)(nil).ReadKeyData), arg0)
}

// ReadSealedKeyObjectFromFile mocks base method.
func (m *MockSecbootBackend) ReadSealedKeyObjectFromFile(kf string) (secboot0.SecbootSealedKeyObjectActor, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadSealedKeyObjectFromFile", kf)
	ret0, _ := ret[0].(secboot0.SecbootSealedKeyObjectActor)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReadSealedKeyObjectFromFile indicates an expected call of ReadSealedKeyObjectFromFile.
func (mr *MockSecbootBackendMockRecorder) ReadSealedKeyObjectFromFile(kf any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadSealedKeyObjectFromFile", reflect.TypeOf((*MockSecbootBackend)(nil).ReadSealedKeyObjectFromFile), kf)
}

// RenameLUKS2ContainerKey mocks base method.
func (m *MockSecbootBackend) RenameLUKS2ContainerKey(devicePath, oldName, newName string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RenameLUKS2ContainerKey", devicePath, oldName, newName)
	ret0, _ := ret[0].(error)
	return ret0
}

// RenameLUKS2ContainerKey indicates an expected call of RenameLUKS2ContainerKey.
func (mr *MockSecbootBackendMockRecorder) RenameLUKS2ContainerKey(devicePath, oldName, newName any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RenameLUKS2ContainerKey", reflect.TypeOf((*MockSecbootBackend)(nil).RenameLUKS2ContainerKey), devicePath, oldName, newName)
}

// UpdateKeyDataPCRProtectionPolicy mocks base method.
func (m *MockSecbootBackend) UpdateKeyDataPCRProtectionPolicy(tpm *tpm2.Connection, authKey secboot.PrimaryKey, pcrProfile *tpm2.PCRProtectionProfile, policyVersionOption tpm2.PCRPolicyVersionOption, keys ...secboot0.SecbootKeyDataActor) error {
	m.ctrl.T.Helper()
	varargs := []any{tpm, authKey, pcrProfile, policyVersionOption}
	for _, a := range keys {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "UpdateKeyDataPCRProtectionPolicy", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateKeyDataPCRProtectionPolicy indicates an expected call of UpdateKeyDataPCRProtectionPolicy.
func (mr *MockSecbootBackendMockRecorder) UpdateKeyDataPCRProtectionPolicy(tpm, authKey, pcrProfile, policyVersionOption any, keys ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{tpm, authKey, pcrProfile, policyVersionOption}, keys...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateKeyDataPCRProtectionPolicy", reflect.TypeOf((*MockSecbootBackend)(nil).UpdateKeyDataPCRProtectionPolicy), varargs...)
}

// UpdateKeyPCRProtectionPolicyMultiple mocks base method.
func (m *MockSecbootBackend) UpdateKeyPCRProtectionPolicyMultiple(tpm *tpm2.Connection, keys []secboot0.SecbootSealedKeyObjectActor, authKey secboot.PrimaryKey, pcrProfile *tpm2.PCRProtectionProfile) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateKeyPCRProtectionPolicyMultiple", tpm, keys, authKey, pcrProfile)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateKeyPCRProtectionPolicyMultiple indicates an expected call of UpdateKeyPCRProtectionPolicyMultiple.
func (mr *MockSecbootBackendMockRecorder) UpdateKeyPCRProtectionPolicyMultiple(tpm, keys, authKey, pcrProfile any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateKeyPCRProtectionPolicyMultiple", reflect.TypeOf((*MockSecbootBackend)(nil).UpdateKeyPCRProtectionPolicyMultiple), tpm, keys, authKey, pcrProfile)
}

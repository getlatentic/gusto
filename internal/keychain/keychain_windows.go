package keychain

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"
	"unsafe"
)

// credentialManager is the person's Windows Credential Manager, through advapi32: a generic credential for each provider,
// kept for this user on this computer.
type credentialManager struct{}

func system() (Store, error) { return credentialManager{}, nil }

var (
	advapi32   = syscall.NewLazyDLL("advapi32.dll")
	credWrite  = advapi32.NewProc("CredWriteW")
	credRead   = advapi32.NewProc("CredReadW")
	credDelete = advapi32.NewProc("CredDeleteW")
	credFree   = advapi32.NewProc("CredFree")
)

// credential is CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	errorNotFound           = syscall.Errno(1168)
)

func target(provider string) (*uint16, error) {
	return syscall.UTF16PtrFromString(service + ":" + provider)
}

func (credentialManager) Keep(provider, key string) error {
	name, err := target(provider)
	if err != nil {
		return err
	}
	user, err := syscall.UTF16PtrFromString(provider)
	if err != nil {
		return err
	}
	blob := []byte(key)
	if len(blob) == 0 {
		return errors.New("an empty key is not kept")
	}
	held := credential{Type: credTypeGeneric, TargetName: name, UserName: user, Persist: credPersistLocalMachine,
		CredentialBlobSize: uint32(len(blob)), CredentialBlob: &blob[0]}
	if ok, _, err := credWrite.Call(uintptr(unsafe.Pointer(&held)), 0); ok == 0 {
		return fmt.Errorf("the Credential Manager did not keep the key: %w", err)
	}
	return nil
}

func (credentialManager) Read(provider string) (string, error) {
	name, err := target(provider)
	if err != nil {
		return "", err
	}
	var found *credential
	if ok, _, err := credRead.Call(uintptr(unsafe.Pointer(name)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&found))); ok == 0 {
		if errors.Is(err, errorNotFound) {
			return "", fmt.Errorf("no key for %s in the Credential Manager: %w", provider, fs.ErrNotExist)
		}
		return "", err
	}
	defer credFree.Call(uintptr(unsafe.Pointer(found)))
	return string(unsafe.Slice(found.CredentialBlob, found.CredentialBlobSize)), nil
}

func (credentialManager) Forget(provider string) error {
	name, err := target(provider)
	if err != nil {
		return err
	}
	if ok, _, err := credDelete.Call(uintptr(unsafe.Pointer(name)), credTypeGeneric, 0); ok == 0 && !errors.Is(err, errorNotFound) {
		return err
	}
	return nil
}

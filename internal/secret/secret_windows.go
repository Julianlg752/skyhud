package secret

import (
	"errors"
	"syscall"
	"unsafe"
)

// system uses the Windows Credential Manager (advapi32), no cgo.
type system struct{}

var (
	advapi32   = syscall.NewLazyDLL("advapi32.dll")
	credWrite  = advapi32.NewProc("CredWriteW")
	credRead   = advapi32.NewProc("CredReadW")
	credDelete = advapi32.NewProc("CredDeleteW")
	credFree   = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	errorNotFound           = 1168
)

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

func target(name string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(service + "/" + name)
	return p
}

func (system) Get(name string) (string, error) {
	var c *credential
	r, _, err := credRead.Call(uintptr(unsafe.Pointer(target(name))), credTypeGeneric, 0, uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		if errno, ok := err.(syscall.Errno); ok && errno == errorNotFound {
			return "", ErrNotFound
		}
		return "", errors.New("credential manager: could not read the key")
	}
	defer credFree.Call(uintptr(unsafe.Pointer(c)))
	return string(unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)), nil
}

func (system) Set(name, value string) error {
	blob := []byte(value)
	if len(blob) == 0 {
		return errors.New("credential manager: empty key")
	}
	c := credential{
		Type:               credTypeGeneric,
		TargetName:         target(name),
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
	}
	if r, _, _ := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0); r == 0 {
		return errors.New("credential manager: could not store the key")
	}
	return nil
}

func (system) Delete(name string) error {
	if r, _, _ := credDelete.Call(uintptr(unsafe.Pointer(target(name))), credTypeGeneric, 0); r == 0 {
		return errors.New("credential manager: could not delete the key")
	}
	return nil
}

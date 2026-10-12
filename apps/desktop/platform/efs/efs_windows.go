//go:build windows

package efs

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32                  = windows.NewLazySystemDLL("advapi32.dll")
	procEncryptFileW          = advapi32.NewProc("EncryptFileW")
	procDecryptFileW          = advapi32.NewProc("DecryptFileW")
	procFileEncryptionStatusW = advapi32.NewProc("FileEncryptionStatusW")
)

// FileEncryptionStatusW's answers, from winbase.h.
const (
	fileEncryptable = 0
	fileIsEncrypted = 1
)

func platformCheck(path string) (Status, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Unsupported, err
	}
	if err := procFileEncryptionStatusW.Find(); err != nil {
		return Unsupported, ErrUnsupported
	}
	var status uint32
	ok, _, callErr := procFileEncryptionStatusW.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&status)))
	if ok == 0 {
		return Unsupported, fmt.Errorf("Windows could not say whether %s can be encrypted: %w", path, callErr)
	}
	switch status {
	case fileEncryptable:
		return Encryptable, nil
	case fileIsEncrypted:
		return Encrypted, nil
	default:
		return Unsupported, nil
	}
}

func platformEncrypt(path string) error {
	return call(procEncryptFileW, path, "encrypt")
}

func platformDecrypt(path string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := procDecryptFileW.Find(); err != nil {
		return ErrUnsupported
	}
	// The second argument is reserved and must be zero.
	if ok, _, callErr := procDecryptFileW.Call(uintptr(unsafe.Pointer(name)), 0); ok == 0 {
		return fmt.Errorf("Windows could not decrypt %s: %w", path, callErr)
	}
	return nil
}

func call(proc *windows.LazyProc, path, verb string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := proc.Find(); err != nil {
		return ErrUnsupported
	}
	if ok, _, callErr := proc.Call(uintptr(unsafe.Pointer(name))); ok == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_SUPPORTED) {
			return ErrUnsupported
		}
		return fmt.Errorf("Windows could not %s %s: %w", verb, path, callErr)
	}
	return nil
}

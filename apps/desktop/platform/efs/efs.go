// Package efs encrypts files with Windows' Encrypting File System (ADR-0063).
//
// Windows keeps the key in the signed-in account. A file it encrypts reads as
// usual to that account and to programs running in it, and as noise to every
// other account, administrators included, and to anyone reading the disk from
// another system. Windows encrypts a file only while nothing has it open.
package efs

import "errors"

// Status is what Windows says about encrypting a path.
type Status int

const (
	// Unsupported is a path this Windows will not encrypt: an edition
	// without EFS, a volume that cannot hold it, or a system location.
	Unsupported Status = iota
	// Encryptable is a path that is not encrypted and could be.
	Encryptable
	// Encrypted is a path Windows already encrypts.
	Encrypted
)

// ErrUnsupported is returned where there is no Windows file encryption.
var ErrUnsupported = errors.New("this system has no Windows file encryption")

// Check asks Windows whether path is encrypted, or could be.
func Check(path string) (Status, error) { return platformCheck(path) }

// Encrypt encrypts path. A folder is marked so that what is created in it is
// encrypted too; what it already holds is encrypted one file at a time.
func Encrypt(path string) error { return platformEncrypt(path) }

// Decrypt decrypts path, or for a folder, stops new files being encrypted.
func Decrypt(path string) error { return platformDecrypt(path) }

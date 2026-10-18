package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"non24.app/desktop/platform/efs"
)

// ZeitBoard's files encrypted by Windows, by choice (ADR-0063). The choice is
// kept beside the database and applied when ZeitBoard starts, before the store
// opens, because Windows encrypts only a file nothing has open. Windows keeps
// the key in the signed-in account.

const (
	storageEncryptionFileName = "storage-encryption.json"
	storageEncryptionSchema   = "v1"
)

// fileCrypt is the part of Windows file encryption ZeitBoard uses. Tests
// stand in for it: encrypting for real makes a key in the account running
// them.
type fileCrypt interface {
	Check(path string) (efs.Status, error)
	Encrypt(path string) error
	Decrypt(path string) error
}

type windowsFileCrypt struct{}

func (windowsFileCrypt) Check(path string) (efs.Status, error) { return efs.Check(path) }
func (windowsFileCrypt) Encrypt(path string) error             { return efs.Encrypt(path) }
func (windowsFileCrypt) Decrypt(path string) error             { return efs.Decrypt(path) }

type storageEncryptionChoice struct {
	Schema  string `json:"schema"`
	Encrypt bool   `json:"encrypt"`
}

type StorageEncryptionInput struct {
	Encrypt bool `json:"encrypt"`
}

// StorageEncryptionDTO is the choice and how far Windows has carried it out.
type StorageEncryptionDTO struct {
	// Supported is false on a Windows without file encryption, such as Home,
	// and on every other system.
	Supported bool `json:"supported"`
	Chosen    bool `json:"chosen"`

	// Platform is the operating system: windows, macos or linux, or another
	// as Go names it. Where ZeitBoard cannot encrypt its files itself, the
	// window points to that system's own disk encryption.
	Platform string `json:"platform"`

	// EncryptedFiles of Files are encrypted now. Pending says the files do
	// not match the choice yet, which the next start puts right.
	EncryptedFiles int  `json:"encryptedFiles"`
	Files          int  `json:"files"`
	Pending        bool `json:"pending"`

	// Problem is why the last start could not carry the choice out.
	Problem string `json:"problem,omitempty"`
}

// readStorageEncryptionChoice reads the saved choice, and whether there is
// one. Without one ZeitBoard leaves the files as they are: a folder the owner
// encrypted themselves is not ZeitBoard's to decrypt.
func readStorageEncryptionChoice(dir string) (encrypt, saved bool, err error) {
	data, err := os.ReadFile(filepath.Join(dir, storageEncryptionFileName))
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	var choice storageEncryptionChoice
	if err := json.Unmarshal(data, &choice); err != nil || choice.Schema != storageEncryptionSchema {
		return false, false, errors.New("The saved encryption choice could not be read.")
	}
	return choice.Encrypt, true, nil
}

func writeStorageEncryptionChoice(dir string, encrypt bool) error {
	data, err := json.Marshal(storageEncryptionChoice{Schema: storageEncryptionSchema, Encrypt: encrypt})
	if err != nil {
		return err
	}
	return writePrivateFileAtomic(filepath.Join(dir, storageEncryptionFileName), data)
}

// storagePaths lists the data folder and everything in it, folders before the
// files they hold.
func storagePaths(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type().IsRegular() {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

// applyStorageEncryption brings the data folder in line with the saved
// choice. Folders are encrypted before the files in them and decrypted after,
// so a file written meanwhile follows its folder. What could not be done is
// reported, and the next start tries again.
func applyStorageEncryption(dir string, crypt fileCrypt) error {
	encrypt, saved, err := readStorageEncryptionChoice(dir)
	if err != nil || !saved {
		return err
	}
	if status, err := crypt.Check(dir); err != nil || status == efs.Unsupported {
		if !encrypt {
			return nil
		}
		return noFileEncryption(runtime.GOOS)
	}
	paths, err := storagePaths(dir)
	if err != nil {
		return err
	}
	if !encrypt {
		slices.Reverse(paths)
	}
	failed := 0
	for _, path := range paths {
		status, err := crypt.Check(path)
		if err != nil {
			failed++
			continue
		}
		switch {
		case encrypt && status == efs.Encryptable:
			err = crypt.Encrypt(path)
		case !encrypt && status == efs.Encrypted:
			err = crypt.Decrypt(path)
		}
		if err != nil {
			failed++
		}
	}
	if failed > 0 {
		verb := "encrypted"
		if !encrypt {
			verb = "decrypted"
		}
		return fmt.Errorf("%d of ZeitBoard's %d files could not be %s. ZeitBoard tries again when it next starts.", failed, len(paths), verb)
	}
	return nil
}

// storageEncryptionStatus reads back what Windows says about each file.
func storageEncryptionStatus(dir string, crypt fileCrypt, problem string) StorageEncryptionDTO {
	status := StorageEncryptionDTO{Problem: problem, Platform: platformName(runtime.GOOS)}
	chosen, saved, err := readStorageEncryptionChoice(dir)
	if err != nil && status.Problem == "" {
		status.Problem = "The saved encryption choice could not be read."
	}
	status.Chosen = chosen
	if folder, err := crypt.Check(dir); err == nil && folder != efs.Unsupported {
		status.Supported = true
	}
	paths, err := storagePaths(dir)
	if err != nil {
		return status
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		status.Files++
		if file, err := crypt.Check(path); err == nil && file == efs.Encrypted {
			status.EncryptedFiles++
		}
	}
	status.Pending = saved && status.Supported && status.Files > 0 &&
		(chosen && status.EncryptedFiles < status.Files || !chosen && status.EncryptedFiles > 0)
	return status
}

// SetStorageEncryption saves the choice to encrypt ZeitBoard's files. The
// folder follows at once, so files made from now on match the choice; the
// files already there follow when ZeitBoard next starts.
func (a *App) SetStorageEncryption(input StorageEncryptionInput) (StorageProtectionDTO, error) {
	dir, err := desktopDataDir()
	if err != nil {
		return StorageProtectionDTO{}, errors.New("ZeitBoard could not find its data folder.")
	}
	crypt := a.storageCrypt()
	if status, err := crypt.Check(dir); input.Encrypt && (err != nil || status == efs.Unsupported) {
		return StorageProtectionDTO{}, noFileEncryption(runtime.GOOS)
	}
	if err := writeStorageEncryptionChoice(dir, input.Encrypt); err != nil {
		return StorageProtectionDTO{}, errors.New("The choice could not be saved. Nothing was changed.")
	}
	if input.Encrypt {
		_ = crypt.Encrypt(dir)
	} else {
		_ = crypt.Decrypt(dir)
	}
	a.storageEncryptionProblem = ""
	return a.GetStorageProtection()
}

// platformName names an operating system as the window words it.
func platformName(goos string) string {
	if goos == "darwin" {
		return "macos"
	}
	return goos
}

// noFileEncryption says why encrypting ZeitBoard's files was refused: a
// Windows without file encryption, or a system ZeitBoard does not encrypt
// files on, where encrypting the disk protects them instead.
func noFileEncryption(goos string) error {
	if goos == "windows" {
		return errors.New("Windows on this computer cannot encrypt files.")
	}
	return errors.New("ZeitBoard encrypts its files on Windows only. Encrypting this computer's disk protects them here.")
}

func (a *App) storageCrypt() fileCrypt {
	if a.storageCryptSource != nil {
		return a.storageCryptSource
	}
	return windowsFileCrypt{}
}

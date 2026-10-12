//go:build windows

package efs

import (
	"os"
	"path/filepath"
	"testing"
)

// A round trip through Windows' file encryption: a folder and a file are
// encrypted, still read as before by this account, and decrypted again.
//
// It runs only where ZEITBOARD_TEST_EFS is set, as in CI. Encrypting a file
// the first time creates an encryption key in the Windows account running
// it, which is not something a test may do to a developer's account.
func TestAFolderIsEncryptedAndDecryptedAgain(t *testing.T) {
	if os.Getenv("ZEITBOARD_TEST_EFS") == "" {
		t.Skip("set ZEITBOARD_TEST_EFS to encrypt files in this Windows account")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "synthetic.db")
	if err := os.WriteFile(file, []byte("synthetic contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	if status, err := Check(file); err != nil || status != Encryptable {
		t.Fatalf("before: %v, %v", status, err)
	}
	for _, path := range []string{dir, file} {
		if err := Encrypt(path); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{dir, file} {
		if status, err := Check(path); err != nil || status != Encrypted {
			t.Fatalf("%s after encrypting: %v, %v", path, status, err)
		}
	}
	// A file made in an encrypted folder is encrypted from the start.
	later := filepath.Join(dir, "synthetic.db-wal")
	if err := os.WriteFile(later, []byte("later"), 0o600); err != nil {
		t.Fatal(err)
	}
	if status, err := Check(later); err != nil || status != Encrypted {
		t.Fatalf("a new file in the folder: %v, %v", status, err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "synthetic contents" {
		t.Fatalf("this account read %q, %v", data, err)
	}
	for _, path := range []string{later, file, dir} {
		if err := Decrypt(path); err != nil {
			t.Fatal(err)
		}
	}
	if status, err := Check(file); err != nil || status != Encryptable {
		t.Fatalf("after decrypting: %v, %v", status, err)
	}
}

// Asking costs nothing and changes nothing, so it runs everywhere.
func TestCheckingAPathChangesNothing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "synthetic.db")
	if err := os.WriteFile(file, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := Check(file)
	if err != nil {
		t.Fatal(err)
	}
	if status == Encrypted {
		t.Fatal("a new file in a temporary folder read as encrypted")
	}
}

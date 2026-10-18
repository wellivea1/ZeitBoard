package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"non24.app/desktop/platform/efs"
)

// syntheticCrypt stands in for Windows file encryption: it remembers what is
// encrypted and in what order things were done, and can refuse a path.
type syntheticCrypt struct {
	unsupported bool
	encrypted   map[string]bool
	refuse      string
	done        []string
}

func newSyntheticCrypt() *syntheticCrypt { return &syntheticCrypt{encrypted: map[string]bool{}} }

func (c *syntheticCrypt) Check(path string) (efs.Status, error) {
	if c.unsupported {
		return efs.Unsupported, nil
	}
	if c.encrypted[path] {
		return efs.Encrypted, nil
	}
	return efs.Encryptable, nil
}

func (c *syntheticCrypt) Encrypt(path string) error {
	if path == c.refuse {
		return errors.New("in use")
	}
	c.encrypted[path] = true
	c.done = append(c.done, "encrypt "+filepath.Base(path))
	return nil
}

func (c *syntheticCrypt) Decrypt(path string) error {
	if path == c.refuse {
		return errors.New("in use")
	}
	delete(c.encrypted, path)
	c.done = append(c.done, "decrypt "+filepath.Base(path))
	return nil
}

func syntheticDataFolder(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ZeitBoard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{desktopDatabaseFile, desktopDatabaseFile + "-wal", "appearance.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("synthetic"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Without a saved choice nothing is touched, whatever is encrypted already:
// a folder the owner encrypted themselves is not ZeitBoard's to undo.
func TestWithoutAChoiceNothingIsTouched(t *testing.T) {
	dir := syntheticDataFolder(t)
	crypt := newSyntheticCrypt()
	crypt.encrypted[filepath.Join(dir, desktopDatabaseFile)] = true
	if err := applyStorageEncryption(dir, crypt); err != nil || len(crypt.done) != 0 {
		t.Fatalf("applied %v, %v", crypt.done, err)
	}
}

// Chosen, the folder is encrypted before the files in it; turned off, the
// files are decrypted before the folder. Each start finishes what is left.
func TestTheChoiceIsCarriedOutFolderFirst(t *testing.T) {
	dir := syntheticDataFolder(t)
	crypt := newSyntheticCrypt()
	if err := writeStorageEncryptionChoice(dir, true); err != nil {
		t.Fatal(err)
	}
	if err := applyStorageEncryption(dir, crypt); err != nil {
		t.Fatal(err)
	}
	if crypt.done[0] != "encrypt ZeitBoard" || len(crypt.done) != 5 {
		t.Fatalf("encrypted in the order %v", crypt.done)
	}
	status := storageEncryptionStatus(dir, crypt, "")
	if !status.Supported || !status.Chosen || status.Files != 4 || status.EncryptedFiles != 4 || status.Pending {
		t.Fatalf("after encrypting %+v", status)
	}

	// A second start finds nothing to do.
	crypt.done = nil
	if err := applyStorageEncryption(dir, crypt); err != nil || len(crypt.done) != 0 {
		t.Fatalf("a second start did %v, %v", crypt.done, err)
	}

	if err := writeStorageEncryptionChoice(dir, false); err != nil {
		t.Fatal(err)
	}
	if status := storageEncryptionStatus(dir, crypt, ""); !status.Pending {
		t.Fatalf("a changed choice is not pending: %+v", status)
	}
	if err := applyStorageEncryption(dir, crypt); err != nil {
		t.Fatal(err)
	}
	if last := crypt.done[len(crypt.done)-1]; last != "decrypt ZeitBoard" {
		t.Fatalf("decrypted in the order %v", crypt.done)
	}
	if status := storageEncryptionStatus(dir, crypt, ""); status.EncryptedFiles != 0 || status.Pending {
		t.Fatalf("after decrypting %+v", status)
	}
}

// A file Windows will not encrypt, here one in use, is counted and left for
// the next start.
func TestAFileThatCannotBeEncryptedIsReported(t *testing.T) {
	dir := syntheticDataFolder(t)
	crypt := newSyntheticCrypt()
	crypt.refuse = filepath.Join(dir, desktopDatabaseFile+"-wal")
	if err := writeStorageEncryptionChoice(dir, true); err != nil {
		t.Fatal(err)
	}
	err := applyStorageEncryption(dir, crypt)
	if err == nil || !strings.HasPrefix(err.Error(), "1 of ZeitBoard's 5 files could not be encrypted.") {
		t.Fatalf("reported %v", err)
	}
	if status := storageEncryptionStatus(dir, crypt, err.Error()); !status.Pending || status.EncryptedFiles != 3 || status.Problem == "" {
		t.Fatalf("status %+v", status)
	}
}

// Where Windows has no file encryption, choosing it is refused, and a choice
// saved elsewhere says why it was not carried out.
func TestWithoutWindowsEncryptionTheChoiceIsRefused(t *testing.T) {
	app := newTestApp(t)
	crypt := newSyntheticCrypt()
	crypt.unsupported = true
	app.storageCryptSource = crypt
	t.Setenv("ZEITBOARD_DATA_DIR", syntheticDataFolder(t))
	if _, err := app.SetStorageEncryption(StorageEncryptionInput{Encrypt: true}); err == nil ||
		err.Error() != noFileEncryption(runtime.GOOS).Error() {
		t.Fatalf("chose encryption without it: %v", err)
	}

	dir := syntheticDataFolder(t)
	if err := writeStorageEncryptionChoice(dir, true); err != nil {
		t.Fatal(err)
	}
	if err := applyStorageEncryption(dir, crypt); err == nil {
		t.Fatal("a choice Windows cannot carry out read as done")
	}
}

// Every system says which it is, and one ZeitBoard does not encrypt files on
// is pointed to its own disk encryption, not told about Windows.
func TestEachSystemIsToldWhatProtectsItsFiles(t *testing.T) {
	for goos, want := range map[string]string{"windows": "windows", "darwin": "macos", "linux": "linux"} {
		if got := platformName(goos); got != want {
			t.Errorf("%s is named %q", goos, got)
		}
	}
	if !strings.Contains(noFileEncryption("windows").Error(), "Windows on this computer") ||
		strings.Contains(noFileEncryption("linux").Error(), "Windows on this computer") {
		t.Fatalf("refusals: %v; %v", noFileEncryption("windows"), noFileEncryption("linux"))
	}
	status := storageEncryptionStatus(syntheticDataFolder(t), newSyntheticCrypt(), "")
	if status.Platform != platformName(runtime.GOOS) {
		t.Fatalf("this system reads as %q", status.Platform)
	}
}

// Choosing it from Settings saves the choice and encrypts the folder at once;
// the files follow at the next start, and the readout says so.
func TestChoosingEncryptionFromSettings(t *testing.T) {
	app := newTestApp(t)
	crypt := newSyntheticCrypt()
	app.storageCryptSource = crypt
	dir := syntheticDataFolder(t)
	t.Setenv("ZEITBOARD_DATA_DIR", dir)

	report, err := app.SetStorageEncryption(StorageEncryptionInput{Encrypt: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(crypt.done, []string{"encrypt ZeitBoard"}) {
		t.Fatalf("at once did %v", crypt.done)
	}
	if encryption := report.Encryption; !encryption.Chosen || !encryption.Pending || encryption.EncryptedFiles != 0 {
		t.Fatalf("readout %+v", encryption)
	}
	if strings.Contains(report.Detail, "encrypted by Windows") {
		t.Fatal("the readout claims encryption before the files are encrypted")
	}

	if err := applyStorageEncryption(dir, crypt); err != nil {
		t.Fatal(err)
	}
	report, err = app.GetStorageProtection()
	if err != nil || report.Encryption.Pending || !strings.Contains(report.Detail, "encrypted by Windows") {
		t.Fatalf("after the next start %+v, %v", report, err)
	}
	for _, file := range report.Files {
		if file.Name == "Local database and backend credential" && !file.Encrypted {
			t.Fatal("the database reads as not encrypted")
		}
	}
}

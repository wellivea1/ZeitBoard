package privatefile_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"non24.app/core/platform/privatefile"
)

func writeFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The point of this package is that the permission is read back, not assumed.
// os.Chmod(0o600) returns nil on Windows and leaves the file readable by
// whatever the parent directory allows, so a test that only checks the error is
// a test that passes while the file is exposed.
func TestARestrictedFileIsActuallyOwnerOnly(t *testing.T) {
	path := writeFile(t, t.TempDir(), "database.db")

	before, err := privatefile.Describe(path)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if err := privatefile.Restrict(path); err != nil {
		t.Fatalf("restrict: %v", err)
	}
	after, err := privatefile.Describe(path)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}

	if !after.OwnerOnly {
		t.Errorf("file is still reachable by another account: %s", after.Detail)
	}
	if after.Inherited {
		t.Errorf("file still inherits from its parent, which can widen it: %s", after.Detail)
	}
	if runtime.GOOS == "windows" && !before.Inherited {
		t.Log("the temporary directory was already protected; the inheritance assertion is weak here")
	}
}

func TestRestrictingIsIdempotent(t *testing.T) {
	path := writeFile(t, t.TempDir(), "token")
	for i := 0; i < 3; i++ {
		if err := privatefile.Restrict(path); err != nil {
			t.Fatalf("restrict %d: %v", i, err)
		}
	}
	access, err := privatefile.Describe(path)
	if err != nil {
		t.Fatal(err)
	}
	if !access.OwnerOnly || access.Inherited {
		t.Errorf("repeated restriction weakened the file: %s", access.Detail)
	}
	// The content survives; this changes permissions, not data.
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "private" {
		t.Errorf("content after restriction = %q, %v", data, err)
	}
}

// SQLite creates the write-ahead log and shared-memory file lazily, so the set
// of files carrying database content changes while the app runs. A helper that
// failed on a missing companion would make callers guess which exist.
func TestCompanionFilesAreRestrictedWhenTheyExist(t *testing.T) {
	dir := t.TempDir()
	database := writeFile(t, dir, "zeitboard.db")
	wal := writeFile(t, dir, "zeitboard.db-wal")
	missing := filepath.Join(dir, "zeitboard.db-shm")

	if err := privatefile.RestrictExisting(database, wal, missing); err != nil {
		t.Fatalf("restrict existing: %v", err)
	}
	for _, path := range []string{database, wal} {
		access, err := privatefile.Describe(path)
		if err != nil {
			t.Fatal(err)
		}
		if !access.OwnerOnly {
			t.Errorf("%s is not owner-only: %s", filepath.Base(path), access.Detail)
		}
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Error("the absent companion was created as a side effect")
	}
}

// A restricted directory has to keep working as a directory, and files created
// inside it afterwards must not be world-readable just because whoever wrote
// them did not know about this package.
func TestFilesCreatedInARestrictedDirectoryAreAlsoPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ZeitBoard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := privatefile.RestrictDir(dir); err != nil {
		t.Fatalf("restrict directory: %v", err)
	}

	path := writeFile(t, dir, "written-later.json")
	access, err := privatefile.Describe(path)
	if err != nil {
		t.Fatal(err)
	}
	if !access.OwnerOnly {
		t.Errorf("a file created in a restricted directory is exposed: %s", access.Detail)
	}
	if _, err := os.ReadFile(path); err != nil {
		t.Errorf("the restricted directory broke ordinary access: %v", err)
	}
}

// Describe has to be capable of saying no. Without this, every assertion above
// would pass against an implementation that reported OwnerOnly for anything —
// which is exactly the failure mode that let os.Chmod stand in for a DACL for
// as long as it did.
//
// On Windows a file written into a fresh temporary directory inherits the
// profile's entries, which include SYSTEM and BUILTIN\Administrators, so the
// honest answer is "not owner-only". Off Windows the mode is the mechanism, so
// the equivalent is a file written 0o644.
func TestDescribeReportsAnUnprotectedFileAsUnprotected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exposed")
	mode := os.FileMode(0o600)
	if runtime.GOOS != "windows" {
		mode = 0o644
	}
	if err := os.WriteFile(path, []byte("private"), mode); err != nil {
		t.Fatal(err)
	}
	access, err := privatefile.Describe(path)
	if err != nil {
		t.Fatal(err)
	}
	if access.OwnerOnly {
		t.Fatalf("an unprotected file was reported as owner-only: %s", access.Detail)
	}
}

func TestDescribingSomethingThatIsNotThereFails(t *testing.T) {
	if _, err := privatefile.Describe(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("describing a missing file succeeded, so a caller could believe it is protected")
	}
}

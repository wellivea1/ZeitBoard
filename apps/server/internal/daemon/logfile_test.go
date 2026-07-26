package daemon

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingLogBoundsCurrentFileAndKeepsOneBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zeitboardd.log")
	w, err := OpenRotatingLog(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "12345678"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "abcd"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	assertLogContents(t, path+".1", "12345678")
	assertLogContents(t, path, "abcd")
}

func TestRotatingLogSplitsOneOversizedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zeitboardd.log")
	w, err := OpenRotatingLog(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.WriteString(w, "abcdefghij")
	if err != nil {
		t.Fatal(err)
	}
	if n != 10 {
		t.Fatalf("Write returned %d bytes, want 10", n)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	assertLogContents(t, path+".1", "efgh")
	assertLogContents(t, path, "ij")
}

func TestRotatingLogRotatesOversizedFileOnOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zeitboardd.log")
	if err := os.WriteFile(path, []byte("oversized"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := OpenRotatingLog(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	assertLogContents(t, path+".1", "oversized")
	assertLogContents(t, path, "")
}

func TestRotatingLogRejectsNonPositiveLimit(t *testing.T) {
	if _, err := OpenRotatingLog(filepath.Join(t.TempDir(), "x.log"), 0); err == nil {
		t.Fatal("expected a non-positive limit to fail")
	}
}

func assertLogContents(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("%s = %q, want %q", path, raw, want)
	}
}

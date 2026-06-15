package fixtures

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"non24.app/tools/internal/repo"
)

// TestFixturesMatchCheckedIn is the drift guard: the Go generator must
// reproduce the committed testdata/v1 files byte-for-byte. This is the
// equivalent of the former `generate-testdata.py --check`.
func TestFixturesMatchCheckedIn(t *testing.T) {
	root, err := repo.Root()
	if err != nil {
		t.Fatal(err)
	}
	files, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures were generated")
	}
	for _, f := range files {
		path := filepath.Join(root, "testdata", "v1", f.Name)
		want, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", f.Name, err)
			continue
		}
		if !bytes.Equal(want, f.Data) {
			t.Errorf("generated %s differs from the checked-in fixture; run go run ./cmd/genfixtures", f.Name)
		}
	}
}

package fixtures

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"non24.app/tools/internal/repo"
)

// TestFixturesMatchCheckedIn is the drift guard: the Go generator must
// reproduce the committed versioned testdata files byte-for-byte. This is the
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
	manifest := Manifest()
	if len(files) != len(manifest) {
		t.Fatalf("generated %d fixtures for %d manifest entries", len(files), len(manifest))
	}

	expected := make(map[string]File, len(files))
	for i, file := range files {
		if file.ManifestEntry != manifest[i] {
			t.Fatalf("generated fixture %d does not match its manifest entry", i)
		}
		generatedPath := file.GeneratedPath()
		if _, duplicate := expected[generatedPath]; duplicate {
			t.Fatalf("duplicate generated fixture path %s", generatedPath)
		}
		expected[generatedPath] = file
	}

	testdataRoot := filepath.Join(root, "testdata")
	err = filepath.WalkDir(testdataRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		generatedPath := filepath.ToSlash(relative)
		file, ok := expected[generatedPath]
		if !ok {
			t.Errorf("unexpected generated fixture %s; remove it or add it to the fixture manifest", generatedPath)
			return nil
		}
		checkedIn, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(checkedIn, file.Data) {
			t.Errorf("generated %s differs from the checked-in fixture; run go run ./cmd/genfixtures", generatedPath)
		}
		delete(expected, generatedPath)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for generatedPath := range expected {
		t.Errorf("manifest fixture %s is missing from testdata; run go run ./cmd/genfixtures", generatedPath)
	}
}

func TestFixtureManifestRejectsRegistryDrift(t *testing.T) {
	validSpec := fixtureSpec{
		id:            "valid",
		ManifestEntry: ManifestEntry{Version: "v1", Name: "valid.json", Schema: "valid.schema.json"},
	}
	tests := map[string]struct {
		specs  []fixtureSpec
		values map[fixtureID]any
	}{
		"missing generated value": {
			specs:  []fixtureSpec{validSpec},
			values: map[fixtureID]any{},
		},
		"unexpected generated value": {
			specs:  []fixtureSpec{validSpec},
			values: map[fixtureID]any{"valid": map[string]any{}, "stale": map[string]any{}},
		},
		"duplicate logical id": {
			specs: []fixtureSpec{
				validSpec,
				{id: "valid", ManifestEntry: ManifestEntry{Version: "v1", Name: "other.json", Schema: "valid.schema.json"}},
			},
			values: map[fixtureID]any{"valid": map[string]any{}},
		},
		"duplicate generated path": {
			specs: []fixtureSpec{
				validSpec,
				{id: "other", ManifestEntry: validSpec.ManifestEntry},
			},
			values: map[fixtureID]any{"valid": map[string]any{}, "other": map[string]any{}},
		},
		"missing schema": {
			specs: []fixtureSpec{
				{id: "valid", ManifestEntry: ManifestEntry{Version: "v1", Name: "valid.json"}},
			},
			values: map[fixtureID]any{"valid": map[string]any{}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := encodeFixtureManifest(test.specs, test.values); err == nil {
				t.Fatal("registry drift was accepted")
			}
		})
	}
}

func TestManifestReturnsCopy(t *testing.T) {
	first := Manifest()
	if len(first) == 0 {
		t.Fatal("manifest is empty")
	}
	originalName := first[0].Name
	first[0].Name = "mutated.json"
	if Manifest()[0].Name != originalName {
		t.Fatal("Manifest exposed mutable package state")
	}
}

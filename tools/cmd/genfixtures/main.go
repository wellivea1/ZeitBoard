// Command genfixtures deterministically writes (or, with -check, verifies) the
// synthetic contract fixtures under testdata/v1. It replaces the former
// scripts/generate-testdata.py.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"non24.app/tools/internal/fixtures"
	"non24.app/tools/internal/repo"
)

func main() {
	check := flag.Bool("check", false, "verify checked-in fixtures match generated output instead of writing")
	flag.Parse()

	root, err := repo.Root()
	if err != nil {
		fail(err)
	}
	files, err := fixtures.Build()
	if err != nil {
		fail(err)
	}

	outDir := filepath.Join(root, "testdata", "v1")
	var mismatches []string
	for _, f := range files {
		path := filepath.Join(outDir, f.Name)
		if *check {
			existing, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(existing, f.Data) {
				mismatches = append(mismatches, f.Name)
			}
			continue
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			fail(err)
		}
		if err := os.WriteFile(path, f.Data, 0o644); err != nil {
			fail(err)
		}
	}

	if len(mismatches) > 0 {
		fmt.Fprintln(os.Stderr, "fixture drift detected:")
		for _, name := range mismatches {
			fmt.Fprintf(os.Stderr, "  %s\n", name)
		}
		fmt.Fprintln(os.Stderr, "regenerate with: go run ./cmd/genfixtures (from the tools module)")
		os.Exit(1)
	}

	action := "generated"
	if *check {
		action = "verified"
	}
	fmt.Printf("%s %d synthetic fixture files in %s\n", action, len(files), outDir)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

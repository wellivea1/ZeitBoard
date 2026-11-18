package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strconv"
	"strings"
	"testing"
)

// A settings file the table does not name is one the protection readout never
// checks. This reads the package's own source for every constant that names a
// .json file and holds each to the table.
func TestEverySettingsFileIsInTheTable(t *testing.T) {
	listed := map[string]bool{}
	for _, file := range settingsFiles {
		if file.Name == "" || file.Holds == "" || !strings.HasSuffix(file.FileName, ".json") || listed[file.FileName] {
			t.Fatalf("incomplete or repeated entry %+v", file)
		}
		listed[file.FileName] = true
	}
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, declaration := range file.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok || general.Tok != token.CONST {
					continue
				}
				for _, spec := range general.Specs {
					value := spec.(*ast.ValueSpec)
					for index, name := range value.Names {
						if index >= len(value.Values) {
							continue
						}
						literal, ok := value.Values[index].(*ast.BasicLit)
						if !ok || literal.Kind != token.STRING {
							continue
						}
						text, err := strconv.Unquote(literal.Value)
						if err != nil || !strings.HasSuffix(text, ".json") || strings.ContainsAny(text, "/ ") {
							continue
						}
						found++
						if !listed[text] {
							t.Errorf("%s names %q, which settingsFiles does not list", name.Name, text)
						}
					}
				}
			}
		}
	}
	if found < len(settingsFiles) {
		t.Fatalf("found %d settings file names in the source, fewer than the %d listed: the search is not reading the package", found, len(settingsFiles))
	}
}

// The readout checks each file and, where one is kept, its last good copy.
func TestTheReadoutChecksEachSettingsFileAndItsCopy(t *testing.T) {
	paths := settingsFilePaths("data")
	names := map[string]bool{}
	for _, entry := range paths {
		names[entry.name] = true
	}
	for _, want := range []string{"Status choices", "Status choices, last good copy", "Closed notices", "Encryption choice"} {
		if !names[want] {
			t.Fatalf("the readout does not check %q: %v", want, names)
		}
	}
	if names["Encryption choice, last good copy"] {
		t.Fatal("the encryption choice keeps no copy, and one was listed")
	}
}

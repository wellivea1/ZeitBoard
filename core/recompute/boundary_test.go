package recompute_test

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// TestTheSchedulerReachesNothingItShouldNot is P7's "runs no LLM" invariant,
// enforced structurally rather than by convention.
//
// A background loop that recomputes without anyone watching is exactly the place
// an assistant call would be easiest to add and hardest to notice: no screen, no
// user, no obvious cost. Asserting it on the import graph means adding one has
// to break this test on the way in.
func TestTheSchedulerReachesNothingItShouldNot(t *testing.T) {
	forbidden := []string{
		"provider",  // LLM clients
		"assistant", // the answer path
		"net/http",  // no network of any kind from the scheduler
		"net/url",
		"os/exec",
	}

	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}

	for name, pkg := range packages {
		if strings.HasSuffix(name, "_test") {
			continue
		}
		for path, file := range pkg.Files {
			for _, spec := range file.Imports {
				imported, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatalf("%s: unquote import: %v", path, err)
				}
				for _, banned := range forbidden {
					if imported == banned || strings.HasSuffix(imported, "/"+banned) {
						t.Errorf("%s imports %q; the recompute loop must not reach it", path, imported)
					}
				}
			}
		}
	}
}

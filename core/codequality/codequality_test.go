package codequality

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The count is gocyclo's: one, and one for each branch a reader must follow.
func TestComplexityCountsEachDecision(t *testing.T) {
	source := `package sample

func straight() int { return 1 }

func branching(values []int, ready bool, done chan struct{}) int {
	total := 0
	for _, value := range values {
		if value > 0 && ready || value < -10 {
			total++
		}
	}
	for i := 0; i < 3; i++ {
	}
	switch total {
	case 1, 2:
	case 3:
	default:
	}
	select {
	case <-done:
	default:
	}
	visit := func() {
		if ready {
			total--
		}
	}
	visit()
	return total
}

type box struct{}

func (b *box) method() {}
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample_test.go"), []byte("package sample\n\nfunc ignored() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	functions, err := Measure(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, function := range functions {
		got[function.Name] = function.Complexity
	}
	// branching: 1, range, if, &&, ||, for, two cases, one select case,
	// and the closure's if.
	want := map[string]int{"straight": 1, "branching": 10, "box.method": 1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("measured %v, want %v", got, want)
	}
}

// A function past a limit is reported; test files and test data are not
// measured.
func TestOverReportsWhatPassesALimit(t *testing.T) {
	dir := t.TempDir()
	var long strings.Builder
	long.WriteString("package sample\n\nfunc long() {\n")
	for range MaxLines {
		long.WriteString("\t_ = 0\n")
	}
	long.WriteString("}\n\nfunc tangled(a, b bool) {\n")
	for range MaxComplexity {
		long.WriteString("\tif a && b {\n\t}\n")
	}
	long.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(long.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "testdata"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "testdata", "fixture.go"), []byte(long.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	over, err := Over(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(over) != 2 || over[0].Name != "tangled" || over[0].Complexity != 1+2*MaxComplexity ||
		over[1].Name != "long" || over[1].Lines != MaxLines+2 {
		t.Fatalf("over the limits: %+v", over)
	}
}

// Core holds itself to the limits.
func TestCoreStaysWithinTheLimits(t *testing.T) {
	over, err := Over("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, function := range over {
		t.Errorf("%s (%s:%d) has complexity %d and %d lines; the limits are %d and %d",
			function.Name, function.File, function.Line, function.Complexity, function.Lines, MaxComplexity, MaxLines)
	}
}

package civilfmt

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// directTwelveHour is a twelve-hour time written straight through time.Format,
// past the reader's clock.
var directTwelveHour = regexp.MustCompile(`\.Format\("[^"]*\b(3:04 PM|3 PM|03:04 PM)|time\.Kitchen`)

// requireClockWrittenTimes fails for every source line under root, outside
// this package and tests, that writes a twelve-hour time directly (ADR-0057).
func requireClockWrittenTimes(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "civilfmt", "node_modules", "frontend", ".cache":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for number, line := range strings.Split(string(source), "\n") {
			if directTwelveHour.MatchString(line) {
				t.Errorf("%s:%d writes a twelve-hour time past the reader's clock: %s",
					path, number+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Core writes every time through a Clock its caller chose.
func TestCoreWritesTimesOnTheCallersClock(t *testing.T) {
	requireClockWrittenTimes(t, "..")
}

// The desktop writes every time on the window's clock. The server is left
// out: it writes for readers it does not know (ADR-0057).
func TestTheDesktopWritesTimesOnTheWindowsClock(t *testing.T) {
	desktop := filepath.Join("..", "..", "apps", "desktop")
	if _, err := os.Stat(desktop); err != nil {
		t.Skip("the desktop is not beside core in this checkout")
	}
	requireClockWrittenTimes(t, desktop)
}

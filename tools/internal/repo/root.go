// Package repo locates the repository root so tools work regardless of the
// directory they are invoked from.
package repo

import (
	"errors"
	"os"
	"path/filepath"
)

// Root walks up from the working directory until it finds the go.work file
// that marks the repository root.
func Root() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository root (go.work) not found")
		}
		dir = parent
	}
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// writePrivateFileAtomic keeps a previously valid destination intact until a
// complete, flushed replacement is ready in the same directory.
func writePrivateFileAtomic(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".zeitboard-export-*.tmp")
	if err != nil {
		return fmt.Errorf("create staged file: %w", err)
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("restrict staged file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write staged file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("flush staged file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close staged file: %w", err)
	}
	closed = true
	if err := replaceFileAtomic(tempPath, path); err != nil {
		return fmt.Errorf("publish staged file: %w", err)
	}
	return nil
}

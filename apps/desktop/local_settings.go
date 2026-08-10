package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"non24.app/core/platform/privatefile"
)

// Durable storage for the desktop's own settings files.
//
// These are preferences rather than health records, so they live beside the
// database as small JSON files rather than in it. What they still need is the
// same durability the observation log gets: a settings file half-written during
// a power cut must not come back as a silently different configuration, because
// a schedule the person cannot see is a schedule they cannot correct.
//
// The write is staged to a temporary file, the previous version becomes a
// backup, and the new one is published by rename. A read prefers the primary
// and falls back to the backup, reporting which it used so the caller can
// restore the primary. A file that fails validation is treated as absent rather
// than repaired, because guessing what a corrupt setting meant is worse than
// starting from the default.

type localSettingsFile[T any] struct {
	SchemaVersion string `json:"schema_version"`
	Revision      uint64 `json:"revision"`
	State         T      `json:"state"`
}

type localSettingsStore[T any] struct {
	// Path is the primary file; Path + ".bak" is its backup.
	Path string

	// Schema is the version this build writes and the only one it reads.
	Schema string

	// Name appears in temporary file names and error text.
	Name string

	// Validate rejects a stored state this build cannot honour.
	Validate func(T) error
}

// read returns the stored state, whether one was found, and whether it came
// from the backup rather than the primary.
func (s localSettingsStore[T]) read() (T, uint64, bool, bool, error) {
	var zero T
	backupPath := s.Path + ".bak"
	for index, candidate := range []string{s.Path, backupPath} {
		data, err := os.ReadFile(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return zero, 0, false, false, err
		}
		var stored localSettingsFile[T]
		if err := decodeStrictJSON(data, &stored); err != nil ||
			stored.SchemaVersion != s.Schema || stored.Revision == 0 {
			continue
		}
		if s.Validate != nil {
			if err := s.Validate(stored.State); err != nil {
				continue
			}
		}
		return stored.State, stored.Revision, true, index == 1, nil
	}
	// Nothing readable, but something is there: say so rather than reporting an
	// empty configuration as a fresh install.
	if _, err := os.Stat(s.Path); err == nil {
		return zero, 0, false, false, fmt.Errorf("stored %s settings are invalid", s.Name)
	}
	if _, err := os.Stat(backupPath); err == nil {
		return zero, 0, false, false, fmt.Errorf("stored %s backup is invalid", s.Name)
	}
	return zero, 0, false, false, nil
}

func (s localSettingsStore[T]) write(state T, revision uint64) error {
	tempPath, err := s.stage(state, revision)
	if err != nil {
		return err
	}
	defer os.Remove(tempPath)

	backupPath := s.Path + ".bak"
	hadPrevious := false
	if _, err := os.Stat(s.Path); err == nil {
		if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(s.Path, backupPath); err != nil {
			return err
		}
		hadPrevious = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tempPath, s.Path); err != nil {
		if hadPrevious {
			_ = os.Rename(backupPath, s.Path)
		}
		return fmt.Errorf("publish %s settings: %w", s.Name, err)
	}
	return nil
}

// restorePrimary rewrites the primary from a state recovered out of the backup.
// The unreadable primary is moved aside first, so a failure part way through
// cannot leave the directory with no settings file at all.
func (s localSettingsStore[T]) restorePrimary(state T, revision uint64) error {
	tempPath, err := s.stage(state, revision)
	if err != nil {
		return err
	}
	defer os.Remove(tempPath)

	displacedPath := ""
	if _, err := os.Stat(s.Path); err == nil {
		displaced, err := os.CreateTemp(filepath.Dir(s.Path), "."+s.Name+"-invalid-*")
		if err != nil {
			return err
		}
		displacedPath = displaced.Name()
		if err := displaced.Close(); err != nil {
			return err
		}
		if err := os.Remove(displacedPath); err != nil {
			return err
		}
		if err := os.Rename(s.Path, displacedPath); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tempPath, s.Path); err != nil {
		if displacedPath != "" {
			_ = os.Rename(displacedPath, s.Path)
		}
		return fmt.Errorf("restore %s settings: %w", s.Name, err)
	}
	if displacedPath != "" {
		_ = os.Remove(displacedPath)
	}
	return nil
}

func (s localSettingsStore[T]) stage(state T, revision uint64) (string, error) {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(localSettingsFile[T]{
		SchemaVersion: s.Schema,
		Revision:      revision,
		State:         state,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(s.Path), "."+s.Name+"-*.tmp")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return "", err
	}
	// Applied to the staged file so the published one is never briefly readable.
	if err := privatefile.Restrict(tempPath); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return "", err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return "", err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return "", err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	return tempPath, nil
}

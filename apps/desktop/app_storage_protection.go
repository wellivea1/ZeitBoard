package main

import (
	"path/filepath"

	"non24.app/core/platform/privatefile"
)

// desktopDatabaseFile is the local store. It is named here because the
// protection readout has to inspect the same file the store opens.
const desktopDatabaseFile = "zeitboard-desktop.db"

// What protects the local database, stated as something that was checked.
//
// The permission is read back from the operating system rather than inferred
// from the mode argument that was passed when the file was created, because on
// Windows that argument does not restrict anything. A claim nobody verifies is
// how "file permissions restricted to the owner" survived in docs/privacy.md
// while the database was reachable by every administrator on the machine.

type StorageProtectionDTO struct {
	// State is ok, at_risk, or unknown.
	State string `json:"state"`

	// Headline is the one-line answer.
	Headline string `json:"headline"`

	// Detail says what was checked and what it does not cover. It always says
	// the files are not encrypted, because they are not.
	Detail string `json:"detail"`

	// Files lists what was inspected, so "the database is protected" can be
	// checked rather than believed.
	Files []StorageProtectionFileDTO `json:"files"`
}

type StorageProtectionFileDTO struct {
	Name      string `json:"name"`
	OwnerOnly bool   `json:"ownerOnly"`
	Inherited bool   `json:"inherited"`
	Note      string `json:"note,omitempty"`
}

const storageProtectionDetail = "These files are restricted to your account, not encrypted. " +
	"Anyone who can read this disk from another operating system, and any program " +
	"running as you, can still read them."

// GetStorageProtection reports whether the local files really are private.
func (a *App) GetStorageProtection() (StorageProtectionDTO, error) {
	dir, err := desktopDataDir()
	if err != nil {
		return StorageProtectionDTO{
			State:    "unknown",
			Headline: "ZeitBoard could not find its own data folder to check.",
			Detail:   storageProtectionDetail,
			Files:    []StorageProtectionFileDTO{},
		}, nil
	}

	candidates := []struct{ name, path string }{
		{"Data folder", dir},
		{"Sleep database", filepath.Join(dir, desktopDatabaseFile)},
		{"Write-ahead log", filepath.Join(dir, desktopDatabaseFile+"-wal")},
		{"Backend token", filepath.Join(dir, backendSyncTokenFile)},
		{"Display settings", filepath.Join(dir, appearanceFileName)},
		{"Reaching hours", filepath.Join(dir, reachingFileName)},
	}

	report := StorageProtectionDTO{
		State:    "ok",
		Headline: "Local files are restricted to your account.",
		Detail:   storageProtectionDetail,
		Files:    make([]StorageProtectionFileDTO, 0, len(candidates)),
	}
	checked := 0
	for _, candidate := range candidates {
		access, describeErr := privatefile.Describe(candidate.path)
		if describeErr != nil {
			// Absent is not a failure: the token exists only once sync is on,
			// and the settings files only once something is saved.
			continue
		}
		checked++
		entry := StorageProtectionFileDTO{
			Name:      candidate.name,
			OwnerOnly: access.OwnerOnly,
			Inherited: access.Inherited,
		}
		if !access.OwnerOnly {
			entry.Note = "another account on this computer can read it"
			report.State = "at_risk"
		} else if access.Inherited {
			entry.Note = "a change to the folder's permissions would widen it"
			if report.State == "ok" {
				report.State = "at_risk"
			}
		}
		report.Files = append(report.Files, entry)
	}

	if checked == 0 {
		report.State = "unknown"
		report.Headline = "There is nothing stored on this computer yet."
		return report, nil
	}
	if report.State == "at_risk" {
		report.Headline = "Some local files are readable by other accounts on this computer."
	}
	return report, nil
}

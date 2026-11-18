package main

import "path/filepath"

// Every file beside the database that keeps a choice the owner made
// (settings-architecture-review-2026-11-15.md). The protection readout reads
// this table, and a test fails when the package names a settings file the
// table does not, so a new file cannot be left out of what is checked.

// settingsFile is one such file.
type settingsFile struct {
	// Name is what the protection readout calls it.
	Name string
	// FileName is its name in the data folder.
	FileName string
	// Holds says what is in it.
	Holds string
	// Backup says it is kept through localSettingsStore, which keeps the
	// last good copy beside it as FileName + ".bak". The copy holds the same
	// choices, so it is checked with the file.
	Backup bool
}

var settingsFiles = []settingsFile{
	{Name: "Display settings", FileName: appearanceFileName, Holds: "theme, reduced stimulation and the night rule", Backup: true},
	{Name: "Reaching hours", FileName: reachingFileName, Holds: "the people to reach, their hours and their zones", Backup: true},
	{Name: "Clock", FileName: regionalFileName, Holds: "the 12- or 24-hour clock", Backup: true},
	{Name: "Status choices", FileName: statusSettingsFileName, Holds: "whether a reported status is shared by default, and whether the sleep buttons report one", Backup: true},
	{Name: "Closed notices", FileName: noticesFileName, Holds: "the notes the owner closed for good", Backup: true},
	{Name: "Encryption choice", FileName: storageEncryptionFileName, Holds: "whether Windows encrypts ZeitBoard's files"},
}

// settingsFilePaths lists each settings file in dir under the name the
// readout gives it, with its backup copy after it.
func settingsFilePaths(dir string) []struct{ name, path string } {
	paths := make([]struct{ name, path string }, 0, len(settingsFiles)*2)
	for _, file := range settingsFiles {
		path := filepath.Join(dir, file.FileName)
		paths = append(paths, struct{ name, path string }{file.Name, path})
		if file.Backup {
			paths = append(paths, struct{ name, path string }{file.Name + ", last good copy", path + ".bak"})
		}
	}
	return paths
}

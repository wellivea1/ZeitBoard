package main

import (
	"errors"
	"path/filepath"
	"regexp"
)

// The notes the owner closed. A note explains how something works and is
// closed for good with its × (ui-refactor-plan.md §14). The list was kept only
// in the window's own storage, where clearing the webview's data brought every
// note back and nothing else knew the list existed. The desktop keeps it now;
// the window's copy only draws the first page.

const (
	noticesFileName = "notices.json"
	noticesSchema   = "closed-notices-v1"

	// maxClosedNotices bounds the file. The window's catalog has about ten.
	maxClosedNotices = 64
)

// A note's id as the window's catalog writes it: "home.setup".
var noticeIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]{0,31}(\.[a-z][a-z0-9]{0,31}){1,3}$`)

// closedNotices is the file's state.
type closedNotices struct {
	Hidden []string `json:"hidden"`
}

// ClosedNoticesDTO is the list, and whether the desktop has one stored. A
// window that remembers closed notes the desktop has no list of hands them
// over, so nothing the owner closed comes back.
type ClosedNoticesDTO struct {
	Hidden   []string `json:"hidden"`
	Revision uint64   `json:"revision"`
	Stored   bool     `json:"stored"`
}

// ClosedNoticesInput replaces the list.
type ClosedNoticesInput struct {
	BaseRevision uint64   `json:"baseRevision"`
	Hidden       []string `json:"hidden"`
}

func closedNoticesStore(path string) localSettingsStore[closedNotices] {
	return localSettingsStore[closedNotices]{
		Path: path, Schema: noticesSchema, Name: "closed notices",
		Validate: func(state closedNotices) error {
			_, err := tidyClosedNotices(state.Hidden)
			return err
		},
	}
}

// tidyClosedNotices drops repeats, in the order the notes were closed. The
// desktop does not know the catalog, so it holds the list to its shape: ids of
// the catalog's form, and no more of them than a catalog could have.
func tidyClosedNotices(hidden []string) ([]string, error) {
	tidy := make([]string, 0, len(hidden))
	seen := make(map[string]bool, len(hidden))
	for _, id := range hidden {
		if !noticeIDPattern.MatchString(id) {
			return nil, errors.New("That is not a note ZeitBoard shows.")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		tidy = append(tidy, id)
	}
	if len(tidy) > maxClosedNotices {
		return nil, errors.New("Too many notes were listed.")
	}
	return tidy, nil
}

// GetClosedNotices reads the notes the owner closed.
func (a *App) GetClosedNotices() ClosedNoticesDTO {
	a.noticesMu.Lock()
	defer a.noticesMu.Unlock()
	return a.closedNoticesLocked()
}

func (a *App) closedNoticesLocked() ClosedNoticesDTO {
	dto := ClosedNoticesDTO{Hidden: []string{}}
	if a.configDir == "" {
		if a.noticesRevision > 0 {
			dto.Hidden, dto.Revision, dto.Stored = append(dto.Hidden, a.closedNotices...), a.noticesRevision, true
		}
		return dto
	}
	stored, revision, found, _, err := closedNoticesStore(filepath.Join(a.configDir, noticesFileName)).read()
	if err != nil || !found {
		// An unreadable list shows every note again, which costs a click each.
		return dto
	}
	dto.Hidden, dto.Revision, dto.Stored = append(dto.Hidden, stored.Hidden...), revision, true
	return dto
}

// SaveClosedNotices replaces the list. A save made against a stale revision
// loses to what is stored, as the other settings files do.
func (a *App) SaveClosedNotices(input ClosedNoticesInput) (ClosedNoticesDTO, error) {
	a.noticesMu.Lock()
	defer a.noticesMu.Unlock()
	current := a.closedNoticesLocked()
	if input.BaseRevision != current.Revision {
		return current, nil
	}
	hidden, err := tidyClosedNotices(input.Hidden)
	if err != nil {
		return ClosedNoticesDTO{}, err
	}
	next := current.Revision + 1
	if a.configDir == "" {
		a.closedNotices, a.noticesRevision = hidden, next
		return a.closedNoticesLocked(), nil
	}
	if err := closedNoticesStore(filepath.Join(a.configDir, noticesFileName)).write(closedNotices{Hidden: hidden}, next); err != nil {
		return ClosedNoticesDTO{}, errors.New("The closed notes could not be stored, so they are remembered in this window only.")
	}
	return a.closedNoticesLocked(), nil
}

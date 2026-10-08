package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"non24.app/core/recompute"
	"non24.app/core/sleepv1"
	storage "non24.app/core/storage/sqlite"
)

// The sleep log's reads, each shaped for the view that asks: a numbered page
// of the log, the nights a range of days touches, and what the log is made
// of. None sends the whole history to the window (architecture review #16),
// which grows by a night a day for as long as the owner keeps it.

const (
	sleepLogPageSize = 50
	maxSleepRange    = 45 * 24 * time.Hour
	noSleepEntries   = "No sleep entries yet. Add a sleep interval to start a local estimate."
)

type SleepLogPageInput struct {
	// Page is zero-based. A page past the end reads as the last one.
	Page int `json:"page"`
}

type SleepLogPageDTO struct {
	Status   string          `json:"status"`
	Empty    bool            `json:"empty"`
	Message  string          `json:"message"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
	Entries  []SleepEntryDTO `json:"entries"`
}

type SleepRangeInput struct {
	StartAt string `json:"startAt"`
	EndAt   string `json:"endAt"`
}

// SleepSourcesDTO is what the sleep log is made of, per source, as the
// estimator sees it.
type SleepSourcesDTO struct {
	Status          string                  `json:"status"`
	Message         string                  `json:"message"`
	Total           int                     `json:"total"`
	CorrectedCount  int                     `json:"correctedCount"`
	SuppressedCount int                     `json:"suppressedCount"`
	Sources         []SleepSourceSummaryDTO `json:"sources"`
	// LatestCorrected is the newest night with an edit, for the correction
	// inspector.
	LatestCorrected *SleepEntryDTO `json:"latestCorrected,omitempty"`
}

type SleepSourceSummaryDTO struct {
	Source     string `json:"source"`
	Provenance string `json:"provenance"`
	Total      int    `json:"total"`
	Corrected  int    `json:"corrected"`
	Suppressed int    `json:"suppressed"`
}

// GetSleepLogPage reads one page of the log, newest night first.
func (a *App) GetSleepLogPage(input SleepLogPageInput) (SleepLogPageDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepLogPageDTO{
			Status: "unavailable", Empty: true, Message: "Local storage is unavailable: " + err.Error(),
			PageSize: sleepLogPageSize, Entries: []SleepEntryDTO{},
		}, nil
	}
	ctx := a.applicationContext()
	page := max(input.Page, 0)
	reviews, total, err := store.ReadSleepReviewPage(ctx, page*sleepLogPageSize, sleepLogPageSize)
	if err != nil {
		return SleepLogPageDTO{}, err
	}
	if len(reviews) == 0 && total > 0 {
		// Past the end, as after deleting the only night on the last page.
		page = (total - 1) / sleepLogPageSize
		if reviews, total, err = store.ReadSleepReviewPage(ctx, page*sleepLogPageSize, sleepLogPageSize); err != nil {
			return SleepLogPageDTO{}, err
		}
	}
	dto := SleepLogPageDTO{
		Status: "ready", Total: total, Page: page, PageSize: sleepLogPageSize,
		Message: fmt.Sprintf("%d local sleep %s stored on this device.", total, plural(total, "entry", "entries")),
		Entries: make([]SleepEntryDTO, 0, len(reviews)),
	}
	if total == 0 {
		dto.Status, dto.Empty, dto.Message = "empty", true, noSleepEntries
	}
	for _, review := range reviews {
		dto.Entries = append(dto.Entries, sleepEntryFromReview(review))
	}
	return dto, nil
}

// GetSleepEntriesBetween reads the nights that, as corrected, touch a range of
// at most 45 days, earliest first.
func (a *App) GetSleepEntriesBetween(input SleepRangeInput) (SleepEntriesDTO, error) {
	start, startErr := time.Parse(time.RFC3339, strings.TrimSpace(input.StartAt))
	end, endErr := time.Parse(time.RFC3339, strings.TrimSpace(input.EndAt))
	switch {
	case startErr != nil || endErr != nil:
		return SleepEntriesDTO{}, errors.New("a sleep range needs RFC 3339 start and end instants")
	case !start.Before(end) || end.Sub(start) > maxSleepRange:
		return SleepEntriesDTO{}, errors.New("a sleep range must run forward and span at most 45 days")
	}
	store, err := a.requireStore()
	if err != nil {
		return SleepEntriesDTO{
			Status: "unavailable", Empty: true, Message: "Local storage is unavailable: " + err.Error(), Entries: []SleepEntryDTO{},
		}, nil
	}
	reviews, err := store.ReadSleepReviewsBetween(a.applicationContext(), start, end)
	if err != nil {
		return SleepEntriesDTO{}, err
	}
	dto := SleepEntriesDTO{
		Status: "ready", Entries: make([]SleepEntryDTO, 0, len(reviews)),
		Message: fmt.Sprintf("%d %s in these days.", len(reviews), plural(len(reviews), "night", "nights")),
	}
	if len(reviews) == 0 {
		dto.Status, dto.Empty, dto.Message = "empty", true, "No sleep recorded in these days."
	}
	for _, review := range reviews {
		dto.Entries = append(dto.Entries, sleepEntryFromReview(review))
	}
	return dto, nil
}

// GetSleepSources says what the log is made of: per source, how many nights,
// how many corrected and how many excluded from estimates.
func (a *App) GetSleepSources() (SleepSourcesDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepSourcesDTO{
			Status: "unavailable", Message: "Local storage is unavailable: " + err.Error(), Sources: []SleepSourceSummaryDTO{},
		}, nil
	}
	reviews, err := store.ReadSleepReviews(a.applicationContext())
	if err != nil {
		return SleepSourcesDTO{}, err
	}
	dto := SleepSourcesDTO{Status: "ready", Total: len(reviews), Sources: []SleepSourceSummaryDTO{}}
	if len(reviews) == 0 {
		dto.Status, dto.Message = "empty", noSleepEntries
		return dto, nil
	}
	dto.Message = fmt.Sprintf("%d local sleep %s stored on this device.", len(reviews), plural(len(reviews), "entry", "entries"))
	// Newest night first, as the log lists them.
	sort.Slice(reviews, func(i, j int) bool {
		left, right := reviews[i].Observation, reviews[j].Observation
		if !left.StartAt.Equal(right.StartAt) {
			return left.StartAt.After(right.StartAt)
		}
		return left.ObservationID > right.ObservationID
	})
	bySource := map[[2]string]*SleepSourceSummaryDTO{}
	for _, review := range reviews {
		key := [2]string{review.Effective.SourceLabel, provenanceLabel(review.Observation.Provenance)}
		summary := bySource[key]
		if summary == nil {
			summary = &SleepSourceSummaryDTO{Source: key[0], Provenance: key[1]}
			bySource[key] = summary
		}
		summary.Total++
		if len(review.Corrections) > 0 {
			summary.Corrected++
			dto.CorrectedCount++
			if dto.LatestCorrected == nil {
				entry := sleepEntryFromReview(review)
				dto.LatestCorrected = &entry
			}
		}
		if review.Effective.Suppressed {
			summary.Suppressed++
			dto.SuppressedCount++
		}
	}
	for _, summary := range bySource {
		dto.Sources = append(dto.Sources, *summary)
	}
	sort.Slice(dto.Sources, func(i, j int) bool {
		left, right := dto.Sources[i], dto.Sources[j]
		if left.Total != right.Total {
			return left.Total > right.Total
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		return left.Provenance < right.Provenance
	})
	return dto, nil
}

type SleepUndoInput struct {
	ObservationID string `json:"observationId"`
	ReviewToken   string `json:"reviewToken"`
}

// UndoSleepCorrection takes back a night's latest change. Like every edit it
// adds a correction and removes none: the new one supersedes the latest and
// restores the night as it was before that change. Undoing again steps back
// further, to the night as recorded.
func (a *App) UndoSleepCorrection(input SleepUndoInput) (SleepEntryDTO, error) {
	defer a.requestLocalAnalysis(recompute.ReasonEvidence)
	store, err := a.requireStore()
	if err != nil {
		return SleepEntryDTO{}, err
	}
	ctx := a.applicationContext()
	review, err := store.SleepReview(ctx, input.ObservationID)
	if err != nil {
		return SleepEntryDTO{}, err
	}
	if input.ReviewToken != review.Token() {
		return SleepEntryDTO{}, errors.New("This sleep record changed. Reload the log and review its current source and edits before undoing.")
	}
	target, ok := undoTarget(review)
	if !ok {
		return SleepEntryDTO{}, errors.New("Nothing on this night is left to undo.")
	}
	record := storage.SleepCorrectionRecord{
		CorrectionID:            newLocalID("corr_sleep"),
		TargetObservationID:     input.ObservationID,
		SupersedesCorrectionIDs: review.CorrectionIDs(),
		BasedOnSourceRevision:   &review.SourceRevision,
		AcquisitionMethod:       storage.ProvenanceAcquisitionManual,
		CreatedAt:               a.currentTime().UTC(),
		Reason:                  storage.CorrectionReasonUserEdit,
		Changes:                 target.changes(),
	}
	if err := store.AppendSleepCorrection(ctx, record); err != nil {
		return SleepEntryDTO{}, err
	}
	return a.sleepEntryByID(input.ObservationID)
}

// nightState is a night as an edit leaves it.
type nightState struct {
	start, end     time.Time
	classification string
	excluded       bool
}

func (n nightState) changes() storage.SleepCorrectionChanges {
	start, end, classification, excluded := n.start, n.end, n.classification, n.excluded
	return storage.SleepCorrectionChanges{StartAt: &start, EndAt: &end, SleepClassification: &classification, Excluded: &excluded}
}

// over is the night an edit's changes leave, on top of the night as recorded.
func (n nightState) over(changes storage.SleepCorrectionChanges) nightState {
	if changes.StartAt != nil {
		n.start = changes.StartAt.UTC()
	}
	if changes.EndAt != nil {
		n.end = changes.EndAt.UTC()
	}
	if changes.SleepClassification != nil {
		n.classification = *changes.SleepClassification
	}
	if changes.Excluded != nil {
		n.excluded = *changes.Excluded
	}
	return n
}

func (n nightState) equal(other nightState) bool {
	return n.start.Equal(other.start) && n.end.Equal(other.end) &&
		n.classification == other.classification && n.excluded == other.excluded
}

// statedBy reports whether a correction states this night in full: every
// field given, and each as the night has it.
func (n nightState) statedBy(changes storage.SleepCorrectionChanges) bool {
	complete := changes.StartAt != nil && changes.EndAt != nil && changes.SleepClassification != nil && changes.Excluded != nil
	return complete && n.equal(n.over(changes))
}

// recordedNight is the night as its source has it, provider revisions
// included and no edit of the owner's.
func recordedNight(review sleepv1.ReviewContext) nightState {
	interval := review.Source.Intervals[0].Interval
	return nightState{start: interval.Start.UTC, end: interval.End.UTC, classification: review.Observation.Sleep.Classification}
}

// undoTarget replays a night's edits from the first to the latest. An edit
// that returns the night to the state before the current one is a step back,
// any other a step forward; undo goes to the state below the latest. It is
// false when the night has no single line of edits to undo.
func undoTarget(review sleepv1.ReviewContext) (nightState, bool) {
	if review.NeedsReview || len(review.ManualCorrections) != 1 {
		return nightState{}, false
	}
	recorded := recordedNight(review)
	manual := map[string]storage.SleepCorrectionRecord{}
	for _, correction := range review.Corrections {
		if correction.AcquisitionMethod != storage.ProvenanceAcquisitionHealthConnect {
			manual[correction.CorrectionID] = correction
		}
	}
	// The line of edits, latest first, back to the first. A merge of edits
	// made apart ends the line: there the earlier state is not one state.
	var line []storage.SleepCorrectionRecord
	seen := map[string]bool{}
	current := review.ManualCorrections[0]
	for !seen[current.CorrectionID] {
		seen[current.CorrectionID] = true
		line = append(line, current)
		var parents []storage.SleepCorrectionRecord
		for _, id := range current.SupersedesCorrectionIDs {
			if parent, ok := manual[id]; ok {
				parents = append(parents, parent)
			}
		}
		if len(parents) != 1 {
			break
		}
		current = parents[0]
	}
	states := []nightState{recorded}
	for index := len(line) - 1; index >= 0; index-- {
		next := recorded.over(line[index].Changes)
		if len(states) >= 2 && next.equal(states[len(states)-2]) {
			states = states[:len(states)-1]
			continue
		}
		states = append(states, next)
	}
	if len(states) < 2 {
		return nightState{}, false
	}
	return states[len(states)-2], true
}

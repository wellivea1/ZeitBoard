package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	calendarcore "non24.app/core/calendar"
	storage "non24.app/core/storage/sqlite"
)

// Writing accepted times to the owner's CalDAV calendar (ADR-0053). The store
// queues a write whenever an accepted time appears or goes; this worker carries
// the queue to the calendar and says how it went. What the owner changed in
// their calendar waits for them to settle.

const (
	calendarWritesChangedEvent = "zeitboard:calendar-writes"
	calendarWriteInterval      = time.Minute
	calendarWriteBatch         = 20
	calendarWriteRounds        = 5
)

// A failed write waits longer each time, up to two hours.
var calendarWriteRetryDelays = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

func calendarWriteRetryDelay(failures int) time.Duration {
	return calendarWriteRetryDelays[min(max(failures, 1), len(calendarWriteRetryDelays))-1]
}

type CalendarWriteBackDTO struct {
	On       bool                      `json:"on"`
	SourceID string                    `json:"sourceId,omitempty"`
	Label    string                    `json:"label,omitempty"`
	Username string                    `json:"username,omitempty"`
	Summary  string                    `json:"summary"`
	Problems []CalendarWriteProblemDTO `json:"problems"`
}

// CalendarWriteProblemDTO is a write that did not go through: a conflict for
// the owner to settle, or a failure ZeitBoard retries on its own. Instants are
// RFC 3339; the window words them relative to now, as Plan does.
type CalendarWriteProblemDTO struct {
	EventID  string `json:"eventId"`
	Title    string `json:"title"`
	StartAt  string `json:"startAt,omitempty"`
	Detail   string `json:"detail"`
	Conflict bool   `json:"conflict"`
	Removing bool   `json:"removing"`
	RetryAt  string `json:"retryAt,omitempty"`
}

type CalendarWriteBackInput struct {
	SourceID string `json:"sourceId"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type CalendarWriteConflictInput struct {
	EventID string `json:"eventId"`
	// Decision is "remove" (take ZeitBoard's event out of the calendar after
	// all) or "keep" (leave the calendar as it is).
	Decision string `json:"decision"`
}

func (a *App) startCalendarWriter(parent context.Context) {
	a.calendarWriter.start(parent, calendarWriteInterval, a.writeCalendarPass)
}

func (a *App) GetCalendarWriteBack() (CalendarWriteBackDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	return a.calendarWriteBack(a.applicationContext(), store)
}

// EnableCalendarWriteBack starts writing accepted times to an imported CalDAV
// calendar, once the sign-in is shown to open it and add events to it.
func (a *App) EnableCalendarWriteBack(input CalendarWriteBackInput) (CalendarWriteBackDTO, error) {
	ctx := a.applicationContext()
	store, err := a.requireStore()
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	sourceID, username := strings.TrimSpace(input.SourceID), strings.TrimSpace(input.Username)
	sources, err := store.ListCalendarSources(ctx)
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	var endpoint string
	for _, source := range sources {
		if source.SourceID == sourceID && source.Kind == calendarcore.SourceCalDAV {
			endpoint = source.Endpoint
		}
	}
	if endpoint == "" {
		return CalendarWriteBackDTO{}, storage.ErrCalendarWriteTargetNotCalDAV
	}
	writer, err := newCalDAVWriter(a.calendarHTTPClient, endpoint, username, input.Password)
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	if err := writer.probe(ctx); err != nil {
		return CalendarWriteBackDTO{}, err
	}
	if err := store.SetCalendarWriteTarget(ctx, storage.CalendarWriteTarget{
		SourceID: sourceID, Username: username, Password: input.Password,
	}, a.currentTime()); err != nil {
		return CalendarWriteBackDTO{}, err
	}
	a.calendarWriter.nudge()
	return a.calendarWriteBack(ctx, store)
}

type CalendarRefreshInput struct {
	SourceID string `json:"sourceId"`
}

// RefreshCalendarSource imports again the calendar accepted times are written
// to, with the sign-in kept for writing. Any other calendar is refreshed by
// adding it again, since its sign-in is never kept.
func (a *App) RefreshCalendarSource(input CalendarRefreshInput) (CalendarImportDTO, error) {
	ctx := a.applicationContext()
	store, err := a.requireStore()
	if err != nil {
		return CalendarImportDTO{}, err
	}
	target, on, err := store.CalendarWriteTarget(ctx)
	if err != nil {
		return CalendarImportDTO{}, err
	}
	if !on || target.SourceID != strings.TrimSpace(input.SourceID) {
		return CalendarImportDTO{}, errors.New("only the calendar accepted times are written to keeps a sign-in; add any other again with its address")
	}
	set, endpoint, err := a.fetchCalDAVCalendar(ctx, CalDAVInput{
		Endpoint: target.CollectionURL, Label: target.Label,
		Username: target.Username, Password: target.Password,
	}, a.currentTime().UTC())
	if err != nil {
		return CalendarImportDTO{}, err
	}
	return a.calendarImportResult(ctx, set, endpoint, true)
}

// DisableCalendarWriteBack stops writing and erases the sign-in. What was
// written stays in the owner's calendar.
func (a *App) DisableCalendarWriteBack() (CalendarWriteBackDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	if err := store.ClearCalendarWriteTarget(a.applicationContext()); err != nil {
		return CalendarWriteBackDTO{}, err
	}
	return a.calendarWriteBack(a.applicationContext(), store)
}

// RetryCalendarWrites tries every waiting write now.
func (a *App) RetryCalendarWrites() (CalendarWriteBackDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	if err := store.RetryCalendarWrites(a.applicationContext()); err != nil {
		return CalendarWriteBackDTO{}, err
	}
	a.calendarWriter.nudge()
	return a.calendarWriteBack(a.applicationContext(), store)
}

func (a *App) ResolveCalendarWriteConflict(input CalendarWriteConflictInput) (CalendarWriteBackDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	var remove bool
	switch input.Decision {
	case "remove":
		remove = true
	case "keep":
	default:
		return CalendarWriteBackDTO{}, errors.New(`decision must be "remove" or "keep"`)
	}
	if err := store.ResolveCalendarWriteConflict(a.applicationContext(), strings.TrimSpace(input.EventID), remove, a.currentTime()); err != nil {
		return CalendarWriteBackDTO{}, err
	}
	a.calendarWriter.nudge()
	return a.calendarWriteBack(a.applicationContext(), store)
}

// writeCalendarPass carries every due write to the calendar.
func (a *App) writeCalendarPass(ctx context.Context) {
	store, err := a.requireStore()
	if err != nil {
		return
	}
	target, on, err := store.CalendarWriteTarget(ctx)
	if err != nil || !on {
		return
	}
	writer, err := newCalDAVWriter(a.calendarHTTPClient, target.CollectionURL, target.Username, target.Password)
	if err != nil {
		return
	}
	changed := false
	for round := 0; round < calendarWriteRounds; round++ {
		due, err := store.DueCalendarWrites(ctx, a.currentTime(), calendarWriteBatch)
		if err != nil || len(due) == 0 {
			break
		}
		for _, write := range due {
			if ctx.Err() != nil {
				break
			}
			if a.carryCalendarWrite(ctx, store, writer, write) {
				changed = true
			}
		}
	}
	if changed {
		a.announce(calendarWritesChangedEvent)
	}
}

// carryCalendarWrite makes one write and records how it went. It reports
// whether anything was recorded.
func (a *App) carryCalendarWrite(ctx context.Context, store *storage.Store, writer calDAVWriter, write storage.CalendarWrite) bool {
	now := a.currentTime().UTC()
	var event calendarcore.Event
	var result calendarWriteResult
	switch write.State {
	case storage.CalendarWriteCreate:
		accepted, found, err := store.CalendarWriteEvent(ctx, write.EventID)
		if err != nil || !found {
			// Undone since it was listed: the undo dropped this write.
			return false
		}
		event = accepted
		result = writer.create(ctx, event, now.Truncate(time.Second))
	case storage.CalendarWriteDelete:
		result = writer.remove(ctx, write.Href, write.ETag)
	default:
		return false
	}
	if ctx.Err() != nil {
		// Stopped mid-write: not a failure, and not known to have happened.
		return false
	}
	var err error
	switch {
	case result.outcome == calendarWriteDone && write.State == storage.CalendarWriteCreate:
		err = store.MarkCalendarWritten(ctx, event, result.href, result.etag, now)
	case result.outcome == calendarWriteDone:
		err = store.MarkCalendarWriteRemoved(ctx, write.EventID)
	case result.outcome == calendarWriteConflict:
		err = store.MarkCalendarWriteConflict(ctx, write.EventID, result.detail, now)
	default:
		err = store.DeferCalendarWrite(ctx, write.EventID, result.detail, now.Add(calendarWriteRetryDelay(write.Attempts+1)), now)
	}
	return err == nil
}

// calendarWriteBack is what the owner sees: where accepted times go, how many
// are there, and anything that did not go through.
func (a *App) calendarWriteBack(ctx context.Context, store *storage.Store) (CalendarWriteBackDTO, error) {
	target, on, err := store.CalendarWriteTarget(ctx)
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	dto := CalendarWriteBackDTO{Problems: []CalendarWriteProblemDTO{}}
	if !on {
		dto.Summary = "Accepted times stay in ZeitBoard."
		return dto, nil
	}
	dto.On, dto.SourceID, dto.Label, dto.Username = true, target.SourceID, target.Label, target.Username
	writes, err := store.CalendarWrites(ctx)
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	written, waiting := 0, 0
	for _, write := range writes {
		switch {
		case write.State == storage.CalendarWriteWritten:
			written++
		case write.State == storage.CalendarWriteConflict || write.Attempts > 0:
			problem := CalendarWriteProblemDTO{
				EventID:  write.EventID,
				Title:    write.Title,
				StartAt:  rfc3339OrEmpty(write.StartAt),
				Detail:   write.Detail,
				Conflict: write.State == storage.CalendarWriteConflict,
				Removing: write.Href != "",
			}
			if !problem.Conflict {
				problem.RetryAt = rfc3339OrEmpty(write.NextAttemptAt)
			}
			dto.Problems = append(dto.Problems, problem)
		default:
			waiting++
		}
	}
	dto.Summary = calendarWriteSummary(target.Label, written, waiting)
	return dto, nil
}

func calendarWriteSummary(label string, written, waiting int) string {
	times := func(count int, verb string) string {
		return fmt.Sprintf("%d accepted %s %s", count, plural(count, "time", "times"), plural(count, "is", "are")) + verb
	}
	switch {
	case written == 0 && waiting == 0:
		return "The next time you accept is added to " + label + "."
	case written == 0:
		return times(waiting, " being written to "+label+".")
	case waiting == 0:
		return times(written, " in "+label+".")
	default:
		return times(written, " in "+label+".") + fmt.Sprintf(" %d more %s being written.", waiting, plural(waiting, "is", "are"))
	}
}

func rfc3339OrEmpty(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

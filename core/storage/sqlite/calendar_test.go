package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	calendarcore "non24.app/core/calendar"
)

func openCalendarTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "calendar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, context.Background()
}

func testCalendarSource(kind calendarcore.SourceKind) calendarcore.Source {
	return calendarcore.Source{
		SourceID:        "calendar_source_import_01",
		Label:           "Imported commitments",
		Kind:            kind,
		ReadOnly:        true,
		CoverageStartAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		CoverageEndAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		LastImportedAt:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}
}

func testImportedEvent(id, recordID, title string, start time.Time, busy bool) calendarcore.Event {
	end := start.Add(time.Hour)
	return calendarcore.Event{
		EventID:        id,
		SourceID:       "calendar_source_import_01",
		SourceRecordID: recordID,
		Title:          title,
		StartAt:        start,
		EndAt:          end,
		ZoneID:         "America/New_York",
		Busy:           busy,
		Ownership:      calendarcore.OwnershipImported,
		CreatedAt:      time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}
}

func TestReplaceImportedCalendarIsAtomicReadOnlyAndRevocable(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	start := time.Date(2026, 1, 5, 14, 0, 0, 0, time.UTC)
	source := testCalendarSource(calendarcore.SourceICS)
	first := testImportedEvent("calendar_event_import_01", "uid-1/20260105T140000Z", "Private title", start, true)
	transparent := testImportedEvent("calendar_event_import_02", "uid-2/20260105T160000Z", "Available", start.Add(2*time.Hour), false)

	if err := store.ReplaceImportedCalendar(ctx, source, []calendarcore.Event{first, transparent}, ""); err != nil {
		t.Fatal(err)
	}
	sources, err := store.ListCalendarSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || !sources[0].ReadOnly || sources[0].Endpoint != "" {
		t.Fatalf("sources = %#v", sources)
	}
	events, err := store.CalendarEvents(ctx, start.Add(-time.Hour), start.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Title != "Private title" {
		t.Fatalf("events = %#v", events)
	}
	if _, err := store.db.ExecContext(ctx,
		`UPDATE local_calendar_events SET title = 'mutated' WHERE event_id = ?`, first.EventID,
	); err == nil {
		t.Fatal("database allowed an imported event to be edited in place")
	}

	domainEvents, fingerprintBefore, err := store.BusyDomainEvents(ctx, start.Add(-time.Hour), start.Add(4*time.Hour), "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if len(domainEvents) != 1 || domainEvents[0].Title != "" || string(domainEvents[0].ID) != first.EventID {
		t.Fatalf("scheduler projection leaked text or included transparent event: %#v", domainEvents)
	}

	replacement := first
	replacement.Title = "Changed private title"
	if err := store.ReplaceImportedCalendar(ctx, source, []calendarcore.Event{replacement}, ""); err != nil {
		t.Fatal(err)
	}
	_, fingerprintAfter, err := store.BusyDomainEvents(ctx, start.Add(-time.Hour), start.Add(4*time.Hour), "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if fingerprintAfter != fingerprintBefore {
		t.Fatal("text-only refresh changed the scheduler fingerprint")
	}
	events, err = store.CalendarEvents(ctx, start.Add(-time.Hour), start.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Title != replacement.Title {
		t.Fatalf("snapshot replacement was not atomic: %#v", events)
	}

	if err := store.RemoveImportedCalendar(ctx, source.SourceID); err != nil {
		t.Fatal(err)
	}
	events, err = store.CalendarEvents(ctx, start.Add(-time.Hour), start.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("revoked source retained events: %#v", events)
	}
	if err := store.RemoveImportedCalendar(ctx, source.SourceID); !errors.Is(err, ErrCalendarSourceNotFound) {
		t.Fatalf("second removal error = %v", err)
	}
}

func TestReplaceCalDAVCalendarStoresOnlySanitizedEndpoint(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	source := testCalendarSource(calendarcore.SourceCalDAV)

	for name, endpoint := range map[string]string{
		"credentials": "https://user:secret@calendar.example.test/private/",
		"query":       "https://calendar.example.test/private/?token=secret",
		"remote http": "http://calendar.example.test/private/",
	} {
		t.Run(name, func(t *testing.T) {
			if err := store.ReplaceImportedCalendar(ctx, source, nil, endpoint); err == nil {
				t.Fatal("unsafe endpoint was accepted")
			}
		})
	}
	if err := store.ReplaceImportedCalendar(ctx, source, nil, "https://calendar.example.test/private/"); err != nil {
		t.Fatal(err)
	}
	sources, err := store.ListCalendarSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Endpoint != "https://calendar.example.test/private/" {
		t.Fatalf("stored source = %#v", sources)
	}
	if err := store.ReplaceImportedCalendar(ctx, source, nil, "http://127.0.0.1:8080/private/"); err != nil {
		t.Fatalf("loopback HTTP should be accepted: %v", err)
	}
}

func TestProposalApprovalMaterializesOwnedBlockAndUndoRemovesOnlyIt(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	created := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := store.AddTask(ctx, TaskRecord{
		TaskID:          "task_calendar_01",
		Title:           "Write planning review",
		DurationMinutes: 60,
		Status:          TaskStatusOpen,
		CreatedAt:       created,
	}); err != nil {
		t.Fatal(err)
	}
	importedStart := time.Date(2026, 1, 5, 14, 0, 0, 0, time.UTC)
	imported := testImportedEvent("calendar_event_import_01", "uid-1/20260105T140000Z", "Private source event", importedStart, true)
	if err := store.ReplaceImportedCalendar(ctx, testCalendarSource(calendarcore.SourceICS), []calendarcore.Event{imported}, ""); err != nil {
		t.Fatal(err)
	}
	snapshotStart := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	snapshotEnd := time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC)
	_, fingerprint, err := store.BusyDomainEvents(ctx, snapshotStart, snapshotEnd, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	decidedAt := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)
	input := ProposalDecisionInput{
		DecisionID:        "decision_calendar_01",
		ProposalID:        "proposal_calendar_01",
		TaskID:            "task_calendar_01",
		TaskRevision:      1,
		EstimateID:        "estimate_calendar_01",
		Decision:          ProposalApproved,
		DecidedAt:         decidedAt,
		SnapshotStartAt:   snapshotStart,
		SnapshotEndAt:     snapshotEnd,
		EventSnapshotHash: fingerprint,
	}
	owned := calendarcore.Event{
		EventID:        "calendar_event_owned_01",
		SourceID:       ZeitBoardCalendarSourceID,
		SourceRecordID: input.ProposalID,
		Title:          "Write planning review",
		StartAt:        time.Date(2026, 1, 6, 15, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 1, 6, 16, 0, 0, 0, time.UTC),
		ZoneID:         "America/New_York",
		Busy:           true,
		Ownership:      calendarcore.OwnershipAppOwned,
		CreatedAt:      decidedAt,
		TaskID:         input.TaskID,
		TaskRevision:   input.TaskRevision,
		ProposalID:     input.ProposalID,
	}
	record, err := store.DecideProposal(ctx, input, &owned)
	if err != nil {
		t.Fatal(err)
	}
	if record.EventID != owned.EventID || record.Decision != ProposalApproved {
		t.Fatalf("decision = %#v", record)
	}
	events, err := store.CalendarEvents(ctx, snapshotStart, snapshotEnd)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Ownership != calendarcore.OwnershipImported || events[1].Ownership != calendarcore.OwnershipAppOwned {
		t.Fatalf("calendar after approval = %#v", events)
	}
	if _, err := store.DecideProposal(ctx, input, &owned); !errors.Is(err, ErrProposalAlreadyDecided) {
		t.Fatalf("duplicate decision error = %v", err)
	}
	if err := store.RemoveImportedCalendar(ctx, ZeitBoardCalendarSourceID); !errors.Is(err, ErrCalendarSourceReadOnly) {
		t.Fatalf("ZeitBoard source removal error = %v", err)
	}

	undo, err := store.UndoProposalDecision(ctx, "decision_calendar_undo_01", input.ProposalID, decidedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if undo.Decision != ProposalUndone || undo.SupersedesID != input.DecisionID {
		t.Fatalf("undo = %#v", undo)
	}
	events, err = store.CalendarEvents(ctx, snapshotStart, snapshotEnd)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != imported.EventID || events[0].Title != imported.Title {
		t.Fatalf("undo changed imported data or retained owned block: %#v", events)
	}
	active, err := store.ActiveProposalDecisions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active decisions after undo = %#v", active)
	}
	var decisionCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_proposal_decisions`).Scan(&decisionCount); err != nil {
		t.Fatal(err)
	}
	if decisionCount != 2 {
		t.Fatalf("decision audit count = %d, want 2", decisionCount)
	}
}

func TestProposalDecisionRejectsTaskAndCalendarRaces(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	created := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	task := TaskRecord{
		TaskID:          "task_calendar_01",
		Title:           "Write planning review",
		DurationMinutes: 60,
		Status:          TaskStatusOpen,
		CreatedAt:       created,
	}
	if err := store.AddTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	_, emptyFingerprint, err := store.BusyDomainEvents(ctx, start, end, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	input := ProposalDecisionInput{
		DecisionID:        "decision_calendar_stale_01",
		ProposalID:        "proposal_calendar_stale_01",
		TaskID:            task.TaskID,
		TaskRevision:      1,
		EstimateID:        "estimate_calendar_01",
		Decision:          ProposalRejected,
		DecidedAt:         created.Add(time.Hour),
		SnapshotStartAt:   start,
		SnapshotEndAt:     end,
		EventSnapshotHash: emptyFingerprint,
	}
	blocking := testImportedEvent("calendar_event_import_01", "uid-1/20260105T140000Z", "Private", start.Add(14*time.Hour), true)
	if err := store.ReplaceImportedCalendar(ctx, testCalendarSource(calendarcore.SourceICS), []calendarcore.Event{blocking}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DecideProposal(ctx, input, nil); !errors.Is(err, ErrStaleProposal) {
		t.Fatalf("calendar race error = %v", err)
	}

	_, currentFingerprint, err := store.BusyDomainEvents(ctx, start, end, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	input.EventSnapshotHash = currentFingerprint
	input.DecisionID = "decision_calendar_stale_02"
	task.Revision = 1
	task.UpdatedAt = created.Add(2 * time.Hour)
	task.Title = "Changed planning review"
	if err := store.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DecideProposal(ctx, input, nil); !errors.Is(err, ErrStaleProposal) {
		t.Fatalf("task race error = %v", err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_proposal_decisions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale decisions were persisted: %d", count)
	}
}

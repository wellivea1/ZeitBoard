package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	calendarcore "non24.app/core/calendar"
	"non24.app/core/scheduling"
	storage "non24.app/core/storage/sqlite"
)

func TestImportedEventConstrainsProposalAndApprovalWritesOnlyOwnedBlock(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Now().UTC().Truncate(localProposalTTL).Add(5 * time.Minute)
	app.nowFn = func() time.Time { return fixedNow }
	seedSleepEntries(t, app, 12)
	if _, err := app.AddTask(TaskInput{Title: "Prepare appointment notes", DurationMinutes: 60}); err != nil {
		t.Fatal(err)
	}

	baseline, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	baselineID, baselineCandidate := onlyPendingCandidate(t, baseline)
	privateTitle := "PRIVATE IMPORTED EVENT - NEVER EXPORT"
	ics := fmt.Sprintf("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//ZeitBoard Test//EN\r\n"+
		"BEGIN:VEVENT\r\nUID:fixed-event@example.test\r\nDTSTART:%s\r\nDTEND:%s\r\nSUMMARY:%s\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
		baselineCandidate.proposal.Window.Start.UTC.Format("20060102T150405Z"),
		baselineCandidate.proposal.Window.End.UTC.Format("20060102T150405Z"),
		privateTitle,
	)
	if _, err := app.ImportCalendarFile(CalendarFileInput{
		FileName: "real-commitments.ics",
		Contents: ics,
		ZoneID:   baselineCandidate.proposal.Window.Start.ZoneID,
	}); err != nil {
		t.Fatal(err)
	}

	constrained, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	constrainedID, candidate := onlyPendingCandidate(t, constrained)
	if constrainedID == baselineID {
		t.Fatal("fixed event did not change the exact proposal id")
	}
	if candidate.proposal.Window.Overlaps(baselineCandidate.proposal.Window) {
		t.Fatalf("proposal overlaps imported fixed event: %#v", candidate.proposal.Window)
	}
	if !containsString(candidate.proposal.ExplanationCodes, scheduling.CodeAvoidsFixedEvent) {
		t.Fatalf("proposal did not disclose fixed-event avoidance: %#v", candidate.proposal.ExplanationCodes)
	}

	decision, err := app.DecideLocalProposal(LocalProposalDecisionInput{
		ProposalID: constrainedID,
		Decision:   storage.ProposalApproved,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != storage.ProposalApproved || decision.EventID == "" {
		t.Fatalf("decision = %#v", decision)
	}
	store, err := app.requireStore()
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.CalendarEvents(
		requestContext(t), candidate.snapshotStartAt, candidate.snapshotEndAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("calendar event count after approval = %d: %#v", len(events), events)
	}
	var imported, owned *calendarcore.Event
	for index := range events {
		switch events[index].Ownership {
		case calendarcore.OwnershipImported:
			imported = &events[index]
		case calendarcore.OwnershipAppOwned:
			owned = &events[index]
		}
	}
	if imported == nil || imported.Title != privateTitle || owned == nil || owned.ProposalID != constrainedID {
		t.Fatalf("ownership after approval = %#v", events)
	}
	exported, err := app.ExportOwnedCalendar()
	if err != nil {
		t.Fatal(err)
	}
	if exported.EventCount != 1 || strings.Contains(exported.ICS, privateTitle) || !strings.Contains(exported.ICS, "Prepare appointment notes") {
		t.Fatalf("owned export leaked or omitted data: %#v", exported)
	}
	proposals, err := app.GetProposals()
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals.Proposals) != 1 || proposals.Proposals[0].Decision != storage.ProposalApproved || !proposals.Proposals[0].CanUndo {
		t.Fatalf("persisted approved proposal = %#v", proposals.Proposals)
	}

	undo, err := app.UndoLocalProposalDecision(LocalProposalUndoInput{ProposalID: constrainedID})
	if err != nil {
		t.Fatal(err)
	}
	if undo.Decision != storage.ProposalUndone {
		t.Fatalf("undo = %#v", undo)
	}
	events, err = store.CalendarEvents(requestContext(t), candidate.snapshotStartAt, candidate.snapshotEndAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Ownership != calendarcore.OwnershipImported || events[0].Title != privateTitle {
		t.Fatalf("undo modified import or retained owned block: %#v", events)
	}
	exported, err = app.ExportOwnedCalendar()
	if err != nil {
		t.Fatal(err)
	}
	if exported.EventCount != 0 || strings.Contains(exported.ICS, privateTitle) {
		t.Fatalf("empty owned export = %#v", exported)
	}
}

func TestRejectedLocalProposalWritesNoCalendarBlockAndSurvivesRefresh(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Now().UTC().Truncate(localProposalTTL).Add(5 * time.Minute)
	app.nowFn = func() time.Time { return fixedNow }
	seedSleepEntries(t, app, 12)
	if _, err := app.AddTask(TaskInput{Title: "Review forms", DurationMinutes: 45}); err != nil {
		t.Fatal(err)
	}
	built, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	proposalID, candidate := onlyPendingCandidate(t, built)
	if _, err := app.DecideLocalProposal(LocalProposalDecisionInput{
		ProposalID: proposalID,
		Decision:   storage.ProposalRejected,
	}); err != nil {
		t.Fatal(err)
	}
	store, _ := app.requireStore()
	events, err := store.CalendarEvents(requestContext(t), candidate.snapshotStartAt, candidate.snapshotEndAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("rejection wrote calendar events: %#v", events)
	}
	refreshed, err := app.GetProposals()
	if err != nil {
		t.Fatal(err)
	}
	if len(refreshed.Proposals) != 1 || refreshed.Proposals[0].ID != proposalID || refreshed.Proposals[0].Decision != storage.ProposalRejected {
		t.Fatalf("rejected decision did not survive refresh: %#v", refreshed.Proposals)
	}
	if _, err := app.UndoLocalProposalDecision(LocalProposalUndoInput{ProposalID: proposalID}); err != nil {
		t.Fatal(err)
	}
	afterUndo, err := app.GetProposals()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterUndo.Proposals) != 1 || afterUndo.Proposals[0].Decision != "pending" {
		t.Fatalf("proposal did not return to pending after undo: %#v", afterUndo.Proposals)
	}
}

func TestLocalProposalDecisionRejectsChangedCalendarSnapshot(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Now().UTC().Truncate(localProposalTTL).Add(5 * time.Minute)
	app.nowFn = func() time.Time { return fixedNow }
	seedSleepEntries(t, app, 12)
	if _, err := app.AddTask(TaskInput{Title: "Review forms", DurationMinutes: 45}); err != nil {
		t.Fatal(err)
	}
	built, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	proposalID, candidate := onlyPendingCandidate(t, built)
	ics := fmt.Sprintf("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//ZeitBoard Test//EN\r\n"+
		"BEGIN:VEVENT\r\nUID:race@example.test\r\nDTSTART:%s\r\nDTEND:%s\r\nSUMMARY:Changed calendar\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
		candidate.proposal.Window.Start.UTC.Format("20060102T150405Z"),
		candidate.proposal.Window.End.UTC.Format("20060102T150405Z"),
	)
	if _, err := app.ImportCalendarFile(CalendarFileInput{FileName: "changed.ics", Contents: ics, ZoneID: defaultZoneID}); err != nil {
		t.Fatal(err)
	}
	_, err = app.DecideLocalProposal(LocalProposalDecisionInput{ProposalID: proposalID, Decision: storage.ProposalApproved})
	if !errors.Is(err, storage.ErrStaleProposal) {
		t.Fatalf("stale decision error = %v", err)
	}
}

func TestLocalProposalDecisionRejectsChangedSleepSnapshot(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Now().UTC().Truncate(localProposalTTL).Add(5 * time.Minute)
	app.nowFn = func() time.Time { return fixedNow }
	seedSleepEntries(t, app, 12)
	if _, err := app.AddTask(TaskInput{Title: "Review forms", DurationMinutes: 45}); err != nil {
		t.Fatal(err)
	}
	built, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	proposalID, _ := onlyPendingCandidate(t, built)
	location := locationOrUTC(defaultZoneID)
	addedStart := fixedNow.In(location).Add(-20 * 24 * time.Hour).Truncate(time.Minute)
	if _, err := app.AddSleepEntry(SleepEntryInput{
		StartLocal:     addedStart.Format("2006-01-02T15:04"),
		EndLocal:       addedStart.Add(8 * time.Hour).Format("2006-01-02T15:04"),
		ZoneID:         defaultZoneID,
		Classification: storage.SleepClassificationPrincipal,
	}); err != nil {
		t.Fatal(err)
	}

	_, err = app.DecideLocalProposal(LocalProposalDecisionInput{
		ProposalID: proposalID,
		Decision:   storage.ProposalApproved,
	})
	if !errors.Is(err, storage.ErrStaleProposal) {
		t.Fatalf("stale sleep decision error = %v", err)
	}
}

func onlyPendingCandidate(t *testing.T, built localProposalBuild) (string, localProposalCandidate) {
	t.Helper()
	if len(built.pending) != 1 {
		t.Fatalf("pending candidates = %d, want 1: %#v", len(built.pending), built.dto)
	}
	for id, candidate := range built.pending {
		return id, candidate
	}
	panic("unreachable")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// Found by using the app: with three open tasks, three suggestions landed on
// the same minute, because each task was placed as if it were the only one.
// Accepting them all would have triple-booked that time. The pending set is a
// plan, and a plan does not overlap itself.
func TestPendingSuggestionsDoNotOverlapEachOther(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Now().UTC().Truncate(localProposalTTL).Add(11 * time.Minute)
	app.nowFn = func() time.Time { return fixedNow }
	seedSleepEntries(t, app, 12)
	for _, task := range []TaskInput{
		{Title: "Renew prescription", DurationMinutes: 20},
		{Title: "Email landlord", DurationMinutes: 15},
		{Title: "Deep work", DurationMinutes: 90},
	} {
		if _, err := app.AddTask(task); err != nil {
			t.Fatal(err)
		}
	}

	built, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.pending) != 3 {
		t.Fatalf("%d pending suggestions, want 3", len(built.pending))
	}
	windows := make([]string, 0, len(built.pending))
	for id, left := range built.pending {
		windows = append(windows, left.task.Title+" "+left.proposal.Window.Start.UTC.Format(time.Kitchen))
		for otherID, right := range built.pending {
			if id < otherID && left.proposal.Window.Overlaps(right.proposal.Window) {
				t.Errorf("%q and %q were suggested for overlapping times", left.task.Title, right.task.Title)
			}
		}
		// Another suggestion is not a fixed event, so avoiding one must not be
		// described as avoiding a fixed event.
		if containsString(left.proposal.ExplanationCodes, scheduling.CodeAvoidsFixedEvent) {
			t.Errorf("%q claims to avoid a fixed event, and there are none", left.task.Title)
		}
	}
	if t.Failed() {
		t.Logf("suggested: %v", windows)
	}
}

// Also found by using the app: at 10:41 the planner suggested, and accepted, a
// block from 10:30. The snapshot is pinned to the start of a 30-minute bucket
// so proposals keep their identity while someone reads them; suggestions have
// to start when that bucket ends, not when it began.
func TestSuggestionsNeverStartInThePast(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Now().UTC().Truncate(localProposalTTL).Add(11 * time.Minute)
	app.nowFn = func() time.Time { return fixedNow }
	seedSleepEntries(t, app, 12)
	if _, err := app.AddTask(TaskInput{Title: "Email landlord", DurationMinutes: 15}); err != nil {
		t.Fatal(err)
	}

	built, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	id, candidate := onlyPendingCandidate(t, built)
	if candidate.proposal.Window.Start.UTC.Before(fixedNow) {
		t.Fatalf("suggested %v, which began before now (%v)", candidate.proposal.Window.Start.UTC, fixedNow)
	}

	// The same proposal is still decidable for the rest of its bucket, which
	// is what the pinned snapshot exists for.
	later := fixedNow.Add(15 * time.Minute)
	app.nowFn = func() time.Time { return later }
	if _, err := app.DecideLocalProposal(LocalProposalDecisionInput{ProposalID: id, Decision: storage.ProposalApproved}); err != nil {
		t.Fatalf("a suggestion could not be accepted within its own refresh window: %v", err)
	}
}

// Greedy placement gives the earliest free time to whoever is placed first. A
// task with a deadline has to be placed before one without, or a long open-ended
// task can take the only time before that deadline and leave it unplaced.
func TestADeadlineTaskIsPlannedBeforeAnOpenEndedOne(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Now().UTC().Truncate(localProposalTTL).Add(11 * time.Minute)
	app.nowFn = func() time.Time { return fixedNow }
	seedSleepEntries(t, app, 12)

	probe, err := app.AddTask(TaskInput{Title: "Probe", DurationMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	built, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	_, first := onlyPendingCandidate(t, built)
	firstStart := first.proposal.Window.Start.UTC
	if _, err := app.DeleteTask(TaskActionInput{TaskID: probe.Tasks[0].TaskID, Revision: probe.Tasks[0].Revision}); err != nil {
		t.Fatal(err)
	}

	// Stored first, open-ended and long: in stored order it would take the
	// earliest time.
	if _, err := app.AddTask(TaskInput{Title: "Long open-ended task", DurationMinutes: 120}); err != nil {
		t.Fatal(err)
	}
	location, _ := time.LoadLocation(defaultZoneID)
	deadline := firstStart.Add(45 * time.Minute).In(location).Format("2006-01-02T15:04")
	if _, err := app.AddTask(TaskInput{Title: "Due soon", DurationMinutes: 30, LatestFinishLocal: deadline, ZoneID: defaultZoneID}); err != nil {
		t.Fatal(err)
	}

	built, err = app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	var dueSoon *localProposalCandidate
	for _, candidate := range built.pending {
		if candidate.task.Title == "Due soon" {
			value := candidate
			dueSoon = &value
		}
	}
	if dueSoon == nil {
		for _, unplaced := range built.dto.Unplaced {
			t.Logf("unplaced: %s (%s)", unplaced.Title, unplaced.Reason)
		}
		t.Fatal("the task with a deadline was left unplaced behind an open-ended one")
	}
	if !dueSoon.proposal.Window.Start.UTC.Equal(firstStart) {
		t.Errorf("the deadline task starts at %v, want the earliest free time %v", dueSoon.proposal.Window.Start.UTC, firstStart)
	}
}

// Durations are read by people, so they are counted the way people count.
func TestDurationsReadNaturally(t *testing.T) {
	for _, testCase := range []struct {
		value time.Duration
		want  string
	}{
		{time.Minute, "1 minute"},
		{42 * time.Minute, "42 minutes"},
		{time.Hour, "1 hour"},
		{time.Hour + 42*time.Minute, "1 hour 42 minutes"},
		{2*time.Hour + time.Minute, "2 hours 1 minute"},
		{8 * time.Hour, "8 hours"},
	} {
		if got := formatDuration(testCase.value); got != testCase.want {
			t.Errorf("formatDuration(%s) = %q, want %q", testCase.value, got, testCase.want)
		}
	}
}

// Found by using the app: a block at 4 PM, for someone awake since 8 AM, was
// described as "about 30 minutes into a predicted waking window", because the
// window for now begins at the planning snapshot. In the stretch someone is in,
// the useful reference is when they woke.
func TestASuggestionTodayIsPlacedRelativeToWaking(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Now().UTC().Truncate(localProposalTTL).Add(11 * time.Minute)
	app.nowFn = func() time.Time { return fixedNow }
	seedSleepEntries(t, app, 12)
	if _, err := app.AddTask(TaskInput{Title: "Email landlord", DurationMinutes: 15}); err != nil {
		t.Fatal(err)
	}
	built, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	var context string
	for _, proposal := range built.dto.Proposals {
		if proposal.Title == "Email landlord" {
			context = proposal.RhythmContext
		}
	}
	if !strings.Contains(context, "after you woke") {
		t.Fatalf("rhythm context %q is not measured from waking", context)
	}
	if strings.Contains(context, "1 hours") {
		t.Errorf("rhythm context %q miscounts hours", context)
	}
}

// Found by using the app: after accepting a suggestion, the task list gave no
// sign the task had a time. The accepted block belongs on the task, and only
// while it still describes the task as it is.
func TestAnAcceptedTimeShowsOnItsTask(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Now().UTC().Truncate(localProposalTTL).Add(11 * time.Minute)
	app.nowFn = func() time.Time { return fixedNow }
	seedSleepEntries(t, app, 12)
	if _, err := app.AddTask(TaskInput{Title: "Email landlord", DurationMinutes: 15}); err != nil {
		t.Fatal(err)
	}
	built, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	id, candidate := onlyPendingCandidate(t, built)

	list, err := app.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	if list.Tasks[0].ScheduledStartAt != "" {
		t.Fatalf("a task with only a pending suggestion shows a time: %#v", list.Tasks[0])
	}

	if _, err := app.DecideLocalProposal(LocalProposalDecisionInput{ProposalID: id, Decision: storage.ProposalApproved}); err != nil {
		t.Fatal(err)
	}
	list, err = app.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	task := list.Tasks[0]
	if task.ScheduledStartAt != candidate.proposal.Window.Start.UTC.Format(time.RFC3339) ||
		task.ScheduledEndAt != candidate.proposal.Window.End.UTC.Format(time.RFC3339) ||
		task.ScheduledLabel == "" {
		t.Fatalf("accepted time is missing from the task: %#v", task)
	}

	// Editing the task makes that block a placement of an older version. The
	// planner offers a new one, and the task stops claiming the old time.
	list, err = app.UpdateTask(TaskInput{TaskID: task.TaskID, Revision: task.Revision, Title: "Email landlord", DurationMinutes: 45})
	if err != nil {
		t.Fatal(err)
	}
	if list.Tasks[0].ScheduledStartAt != "" {
		t.Fatalf("an edited task still shows the time accepted for its old version: %#v", list.Tasks[0])
	}
}

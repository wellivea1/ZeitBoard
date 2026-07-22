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

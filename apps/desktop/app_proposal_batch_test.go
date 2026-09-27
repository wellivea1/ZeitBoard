package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

// threeSuggestions plans three tasks at a fixed time and returns the pending
// suggestions' ids, each planned leaving room for the others.
func threeSuggestions(t *testing.T) (*App, time.Time, []string) {
	t.Helper()
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
	ids := make([]string, 0, len(built.pending))
	for id := range built.pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) != 3 {
		t.Fatalf("%d pending suggestions, want 3", len(ids))
	}
	return app, fixedNow, ids
}

// One at a time, the second acceptance of a reviewed list fails: accepting the
// first puts a block in the calendar every other suggestion was planned
// against. That is why a reviewed list is decided together.
func TestSuggestionsReviewedTogetherAreAcceptedTogether(t *testing.T) {
	app, _, ids := threeSuggestions(t)
	if _, err := app.DecideLocalProposal(LocalProposalDecisionInput{ProposalID: ids[0], Decision: storage.ProposalApproved}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.DecideLocalProposal(LocalProposalDecisionInput{ProposalID: ids[1], Decision: storage.ProposalApproved}); !errors.Is(err, storage.ErrStaleProposal) {
		t.Fatalf("a second suggestion from the same review: %v", err)
	}

	app, fixedNow, ids := threeSuggestions(t)
	built, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	decided, err := app.DecideLocalProposals(LocalProposalsDecisionInput{ProposalIDs: ids, Decision: storage.ProposalApproved})
	if err != nil {
		t.Fatal(err)
	}
	if len(decided.Decisions) != 3 || decided.Message != "3 proposals approved and written to ZeitBoard placements together." {
		t.Fatalf("decided = %+v", decided)
	}
	store, _ := app.requireStore()
	owned, err := store.OwnedCalendarEvents(context.Background())
	if err != nil || len(owned) != 3 {
		t.Fatalf("owned blocks = %d, %v", len(owned), err)
	}
	// Each decision keeps the evidence its own suggestion was planned on.
	records, err := store.ActiveProposalDecisions(context.Background())
	if err != nil || len(records) != 3 {
		t.Fatalf("decisions = %d, %v", len(records), err)
	}
	for _, record := range records {
		candidate := built.pending[record.ProposalID]
		if record.Decision != storage.ProposalApproved || record.ProposalTitle != candidate.task.Title ||
			!record.ProposalStartAt.Equal(candidate.proposal.Window.Start.UTC) ||
			record.EventSnapshotHash != candidate.eventSnapshotHash || len(record.ExplanationCodes) == 0 {
			t.Errorf("decision on %q = %+v", candidate.task.Title, record)
		}
	}
	after, err := app.buildLocalProposals(fixedNow)
	if err != nil || len(after.pending) != 0 {
		t.Fatalf("still pending after the batch: %d, %v", len(after.pending), err)
	}
}

// If anything changed since the review, nothing in it is decided.
func TestAChangedPlanDecidesNothingInTheBatch(t *testing.T) {
	app, fixedNow, ids := threeSuggestions(t)
	built, err := app.buildLocalProposals(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	window := built.pending[ids[2]].proposal.Window
	ics := fmt.Sprintf("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//ZeitBoard Test//EN\r\n"+
		"BEGIN:VEVENT\r\nUID:batch-race@example.test\r\nDTSTART:%s\r\nDTEND:%s\r\nSUMMARY:Changed calendar\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
		window.Start.UTC.Format("20060102T150405Z"), window.End.UTC.Format("20060102T150405Z"))
	if _, err := app.ImportCalendarFile(CalendarFileInput{FileName: "changed.ics", Contents: ics, ZoneID: defaultZoneID}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.DecideLocalProposals(LocalProposalsDecisionInput{ProposalIDs: ids, Decision: storage.ProposalApproved}); !errors.Is(err, storage.ErrStaleProposal) {
		t.Fatalf("a batch planned before the calendar changed: %v", err)
	}
	store, _ := app.requireStore()
	if owned, err := store.OwnedCalendarEvents(context.Background()); err != nil || len(owned) != 0 {
		t.Fatalf("a stale batch wrote %d blocks, %v", len(owned), err)
	}
	if records, err := store.ActiveProposalDecisions(context.Background()); err != nil || len(records) != 0 {
		t.Fatalf("a stale batch recorded %d decisions, %v", len(records), err)
	}
}

func TestABatchCanDeclineAndIsBounded(t *testing.T) {
	app, _, ids := threeSuggestions(t)
	decided, err := app.DecideLocalProposals(LocalProposalsDecisionInput{ProposalIDs: ids[:2], Decision: storage.ProposalRejected})
	if err != nil || len(decided.Decisions) != 2 || decided.Message != "2 proposals rejected together." {
		t.Fatalf("declined = %+v, %v", decided, err)
	}
	store, _ := app.requireStore()
	if owned, err := store.OwnedCalendarEvents(context.Background()); err != nil || len(owned) != 0 {
		t.Fatalf("declining wrote %d blocks, %v", len(owned), err)
	}
	for name, input := range map[string]LocalProposalsDecisionInput{
		"nothing":             {Decision: storage.ProposalApproved},
		"the same one twice":  {ProposalIDs: []string{ids[2], ids[2]}, Decision: storage.ProposalApproved},
		"an unknown decision": {ProposalIDs: ids[2:], Decision: "maybe"},
	} {
		if _, err := app.DecideLocalProposals(input); err == nil {
			t.Errorf("%s: decided", name)
		}
	}
}

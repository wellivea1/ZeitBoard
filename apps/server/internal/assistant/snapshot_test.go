package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"non24.app/server/internal/provider"
)

func intPointer(value int) *int { return &value }

// planningSnapshot is the chat view of a synthetic day around fixedNow. Its
// sentences are canaries: none of them may reach a model.
func planningSnapshot() *SnapshotContext {
	at := func(hour, minute int) time.Time { return time.Date(2026, 3, 5, hour, minute, 0, 0, time.UTC) }
	return &SnapshotContext{
		SchemaVersion: "v1", ZoneID: "America/New_York", Now: fixedNow(),
		Rhythm: SnapshotRhythm{
			Status: "estimated", Freshness: "current", Presence: "awake",
			FreshnessExplanation: "CANARY freshness explanation",
			SleepOnset:           &SnapshotRange{Start: at(23, 0), End: at(23, 45)},
			Wake:                 &SnapshotRange{Start: at(23, 45).Add(8 * time.Hour), End: at(23, 45).Add(9 * time.Hour)},
			CycleMinutes:         intPointer(1490), DriftMinutesPerCycle: intPointer(50), TypicalSleepMinutes: intPointer(480),
			FitRating: "medium", FitReasons: []string{"CANARY fit reason"},
		},
		Plans: SnapshotPlans{
			Status:   "available",
			Segments: []SnapshotSegment{{Presence: "awake", Start: at(14, 0), End: at(23, 0)}},
			ReachableHours: []SnapshotReachable{{Start: at(14, 0), End: at(22, 0), Status: "reachable",
				Reachable: []SnapshotRange{{Start: at(14, 0), End: at(22, 0)}}}},
			Commitments: []SnapshotCommitment{
				{Ref: "commitment_01", Start: at(16, 0), End: at(17, 0), Conflict: "none"},
				{Ref: "commitment_02", Start: at(18, 0), End: at(18, 30), Conflict: "none", TaskID: "task_accepted_01"},
				{Ref: "CANARY-not-a-ref", Start: at(19, 0), End: at(19, 30), Conflict: "none"},
			},
			Suggestions: []SnapshotSuggestion{{ProposalID: "proposal_pending_01", TaskID: "task_flexible_01",
				Start: at(20, 0), End: at(20, 30), Confidence: "medium", Reasons: []string{"within_task_bounds"}}},
			Unplaced: []SnapshotUnplaced{{TaskID: "task_unplaced_01", Reason: "no_available_interval"}},
		},
		NeedsYou:      SnapshotNeedsYou{Suggestions: 1, TaskConflicts: 1},
		PrivateFields: "CANARY private fields sentence",
		Disclaimer:    "CANARY disclaimer sentence",
	}
}

func TestAssistantSnapshotReachesTheModelAsEnumsIdsAndCivilTime(t *testing.T) {
	st, device := testStoreAndDevice(t)
	fake := &fakeProvider{responses: []provider.Response{{Text: `{"schema_version":"v1","recommended_action":"answer_only","answer":"You are likely awake until about 11 PM."}`}}}
	service := New(fake, fake.Status(), st)
	service.now = fixedNow
	planning := planningContext()
	planning.Snapshot = planningSnapshot()
	planning.Tasks[0].NeedsReview = true
	if _, err := service.HandleMessage(context.Background(), device, MessageRequest{
		SchemaVersion: SchemaVersion, Message: "When should I do the paperwork?", Context: planning,
	}); err != nil {
		t.Fatal(err)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("provider calls = %d", len(fake.requests))
	}
	captured, err := json.Marshal(fake.requests[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`"snapshot"`, `"presence":"awake"`, `"freshness":"current"`, `"sleep_likely_begins"`, `"waking_likely"`,
		`"drift_minutes_per_cycle":50`, `"fit_rating":"medium"`, `"timeline"`, `"reachable_hours"`,
		`"ref":"commitment_01"`, `"accepted_time_of_task":"task_accepted_01"`, `"awaiting_decision"`,
		`"proposal_pending_01"`, `"task_unplaced_01"`, `"task_conflicts":1`, `"needs_review":true`,
		`Mar 5, 2026 6:00 PM EST`,
	} {
		if !bytes.Contains(captured, []byte(required)) {
			t.Fatalf("model context omitted %s: %s", required, captured)
		}
	}
	if bytes.Contains(captured, []byte("CANARY")) || bytes.Contains(captured, []byte("2026-03-05T")) {
		t.Fatalf("model context carried snapshot text or a raw instant: %s", captured)
	}
}

func TestSnapshotSanitizingDropsWhatAModelMustNotRead(t *testing.T) {
	snapshot := planningSnapshot()
	snapshot.Rhythm.Presence = "CANARY presence"
	snapshot.Rhythm.Status = "CANARY status"
	snapshot.Rhythm.DriftMinutesPerCycle = intPointer(99999)
	snapshot.Plans.Commitments[0].Conflict = "CANARY conflict"
	snapshot.Plans.Suggestions[0].TaskID = "CANARY task"
	snapshot.Plans.Unplaced[0].Reason = "CANARY reason"
	snapshot.Plans.Segments = append(snapshot.Plans.Segments, SnapshotSegment{
		Presence: "awake", Start: fixedNow().Add(30 * 24 * time.Hour), End: fixedNow().Add(31 * 24 * time.Hour),
	})
	out := sanitizeSnapshot(snapshot, fixedNow(), "America/New_York", false)
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "CANARY") {
		t.Fatalf("sanitized snapshot kept unreadable text: %s", encoded)
	}
	if out.Rhythm.Presence != "unknown" || out.Rhythm.Status != "unavailable" || out.Rhythm.DriftMinutesPerCycle != nil ||
		len(out.Timeline) != 1 || len(out.Commitments) != 1 || len(out.AwaitingDecision) != 0 || len(out.Unplaced) != 0 {
		t.Fatalf("sanitized snapshot = %s", encoded)
	}

	// An estimate that is not current states no current state, whatever the
	// snapshot says — as Home's lead does.
	for _, freshness := range []string{"stale", "withheld"} {
		untrusted := planningSnapshot()
		untrusted.Rhythm.Freshness = freshness
		if got := sanitizeSnapshot(untrusted, fixedNow(), "America/New_York", false); got.Rhythm.Presence != "unknown" {
			t.Fatalf("%s estimate kept presence %q", freshness, got.Rhythm.Presence)
		}
	}
	if sanitizeSnapshot(nil, fixedNow(), "UTC", false) != nil {
		t.Fatal("no snapshot became one")
	}
	compact := sanitizeSnapshot(planningSnapshot(), fixedNow(), "America/New_York", true)
	if len(compact.Commitments) > 3 || len(compact.AwaitingDecision) > 2 {
		t.Fatalf("compact snapshot kept too much: %+v", compact)
	}
}

// The chat view has no sleep, task, medication, marker or sync section, so the
// server's strict decoding refuses a request that carries one before anything
// reaches a model.
func TestAChatSnapshotCannotCarryHealthOrSleepSections(t *testing.T) {
	for _, section := range []string{
		`"sleep":{"nights_on_record":3,"going_to_sleep_marked":false}`,
		`"medication":{"status":"ready","count":0,"truncated":false,"items":[]}`,
		`"markers":{"status":"ready","count":0,"truncated":false,"items":[]}`,
		`"tasks":{"count":0,"truncated":false,"items":[]}`,
		`"sync":{"enabled":false,"status":"off","pending_uploads":0,"pending_erasures":0,"waiting_records":0}`,
	} {
		body := `{"schema_version":"v1","message":"hi","context":{"zone_id":"UTC","now":"2026-03-05T12:00:00Z","snapshot":{` +
			`"schema_version":"v1","zone_id":"UTC","now":"2026-03-05T12:00:00Z",` + section + `}}}`
		decoder := json.NewDecoder(strings.NewReader(body))
		decoder.DisallowUnknownFields()
		var request MessageRequest
		if err := decoder.Decode(&request); err == nil {
			t.Fatalf("a chat snapshot carrying %s was accepted", section)
		}
	}
}

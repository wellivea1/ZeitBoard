package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	calendarcore "non24.app/core/calendar"
)

// placeAcceptedTime stands in for an approved suggestion: the app-owned block
// DecideProposal inserts.
func placeAcceptedTime(t *testing.T, store *Store, eventID string, start time.Time) {
	t.Helper()
	ctx := context.Background()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	created := time.Date(2026, 9, 26, 9, 0, 0, 0, time.UTC)
	if err := ensureZeitBoardSource(ctx, tx, created); err != nil {
		t.Fatal(err)
	}
	if err := insertCalendarEvent(ctx, tx, calendarcore.Event{
		EventID: eventID, SourceID: ZeitBoardCalendarSourceID, SourceRecordID: eventID, Title: "Synthetic deep work",
		StartAt: start, EndAt: start.Add(90 * time.Minute), ZoneID: "America/New_York", Busy: true,
		Ownership: calendarcore.OwnershipAppOwned, CreatedAt: created,
		TaskID: "task_synthetic_01", TaskRevision: 1, ProposalID: "proposal_" + eventID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptedTimesTravelAsPlacementsAndLeaveWithTheirBlock(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "placements.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	start := time.Date(2026, 9, 27, 18, 30, 0, 0, time.UTC)
	placeAcceptedTime(t, store, "event_accepted_01", start)
	placeAcceptedTime(t, store, "event_accepted_02", start.Add(24*time.Hour))

	pending, err := store.PendingPlacementSyncRecords(ctx, 10)
	if err != nil || len(pending) != 2 {
		t.Fatalf("pending placements = %#v %v", pending, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(pending[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	// Which task and when; the title stays with the task record.
	if payload["placement_id"] != "event_accepted_01" || payload["task_id"] != "task_synthetic_01" ||
		payload["start_at"] != "2026-09-27T18:30:00Z" || payload["end_at"] != "2026-09-27T20:00:00Z" ||
		payload["zone_id"] != "America/New_York" || payload["title"] != nil {
		t.Fatalf("placement payload = %v", payload)
	}

	pushedAt := time.Date(2026, 9, 26, 9, 5, 0, 0, time.UTC)
	if err := store.MarkPlacementSyncRecordsPushed(ctx, pending[:1], pushedAt); err != nil {
		t.Fatal(err)
	}
	if count, _ := store.PendingPlacementSyncRecordCount(ctx); count != 1 {
		t.Fatalf("pending after acknowledgment = %d", count)
	}

	// Undo removes the block; its record is queued for erasure at once.
	if _, err := store.db.ExecContext(ctx, `DELETE FROM local_calendar_events WHERE event_id = 'event_accepted_01'`); err != nil {
		t.Fatal(err)
	}
	erasures, err := store.PendingSyncErasures(ctx)
	if err != nil || len(erasures) != 1 || erasures[0] != "event_accepted_01" {
		t.Fatalf("erasures = %v %v", erasures, err)
	}
	// An acknowledgment arriving after the block went is not recorded.
	if err := store.MarkPlacementSyncRecordsPushed(ctx, pending[:1], pushedAt); err != nil {
		t.Fatal(err)
	}
	var acknowledged int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_placement_sync_records`).Scan(&acknowledged); err != nil || acknowledged != 0 {
		t.Fatalf("acknowledged = %d %v", acknowledged, err)
	}

	// Pulling the second placement back confirms its upload; its tombstone
	// later settles the bookkeeping and leaves this computer's block alone.
	if _, err := store.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 1, Records: []SyncPullRecord{SyncPullPlacement{PlacementID: "event_accepted_02"}}}); err != nil {
		t.Fatal(err)
	}
	if count, _ := store.PendingPlacementSyncRecordCount(ctx); count != 0 {
		t.Fatalf("pulled placement still pending: %d", count)
	}
	if _, err := store.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 2, Records: []SyncPullRecord{SyncPullTombstone{RecordID: "event_accepted_02", RecordKind: "placement"}}}); err != nil {
		t.Fatal(err)
	}
	var blocks int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_calendar_events WHERE event_id = 'event_accepted_02'`).Scan(&blocks); err != nil || blocks != 1 {
		t.Fatalf("tombstone touched the block: %d %v", blocks, err)
	}
}

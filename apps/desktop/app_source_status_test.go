package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"non24.app/core/sources"
	storage "non24.app/core/storage/sqlite"
)

// The phone's reports arrive as any pulled page does, and every view that
// judges the data reads the same status from them (ADR-0054).
func TestThePhonesReportsReachTheWindowAndTheSnapshot(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)

	// The newest night Health Connect sent ended thirty hours ago.
	nightEnd := now.Add(-30 * time.Hour)
	if err := app.store.AppendSleepObservation(ctx, storage.SleepObservationRecord{
		ObservationID: "hc-0a1b2c3d4e5f60718293a4b5",
		Kind:          storage.SleepKindEpisode,
		StartAt:       nightEnd.Add(-8 * time.Hour),
		EndAt:         nightEnd,
		ZoneID:        defaultZoneID,
		Sleep:         storage.SleepObservationDetails{Classification: storage.SleepClassificationPrincipal},
		Provenance: storage.SleepObservationProvenance{
			AcquisitionMethod: storage.ProvenanceAcquisitionHealthConnect,
			EvidenceStatus:    storage.ProvenanceEvidenceDirectlyObserved,
			RecordedAt:        nightEnd,
			SourceRecordID:    "synthetic-provider-record",
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Read this morning, then the permission went.
	held := 2
	read := sources.Report{
		ReportID: "srcrep-0a1b2c3d4e5f60718293a4b5", Source: sources.HealthConnect, Outcome: sources.Read,
		ReportedAt: now.Add(-3 * time.Hour), BackgroundReads: true,
		Covered: &sources.Window{StartAt: now.Add(-3*time.Hour - 30*24*time.Hour), EndAt: now.Add(-3 * time.Hour)}, Held: &held,
	}
	missing := sources.Report{
		ReportID: "srcrep-1b2c3d4e5f60718293a4b5c6", Source: sources.HealthConnect, Outcome: sources.PermissionMissing,
		ReportedAt: now.Add(-time.Hour),
	}
	page := syncPullResponse{SchemaVersion: "v1", Cursor: 2}
	for index, report := range []sources.Report{read, missing} {
		payload, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		page.Records = append(page.Records, syncEnvelope{
			Seq: int64(index + 1), RecordID: report.ReportID, Kind: storage.SyncKindSourceReport,
			DeviceID: "device_0a1b2c3d", CreatedAt: report.ReportedAt, Payload: payload,
		})
	}
	if err := validatePullEnvelopePage(page, 0); err != nil {
		t.Fatalf("a page of reports was refused: %v", err)
	}
	var records []storage.SyncPullRecord
	for _, envelope := range page.Records {
		record, err := decodePulledRecord(envelope)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if _, err := app.store.ApplySyncPullPage(ctx, storage.SyncPullPage{Cursor: page.Cursor, Records: records}); err != nil {
		t.Fatal(err)
	}

	statuses, err := app.GetSourceStatuses()
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %+v", statuses)
	}
	status := statuses[0]
	if status.Source != sources.HealthConnect || status.Outcome != sources.PermissionMissing || status.BackgroundReads || status.Held != 2 ||
		status.Since != missing.ReportedAt.Format(time.RFC3339) || status.ReadThrough != read.Covered.EndAt.Format(time.RFC3339) ||
		status.NewestRecordEnd != nightEnd.Format(time.RFC3339) {
		t.Fatalf("status = %+v", status)
	}

	// An assistant reads the same status, with no time of sleep in it.
	snapshot, err := app.assistantSnapshot(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.Sync.Sources; len(got) != 1 || got[0].Outcome != sources.PermissionMissing || got[0].ReadThrough == nil || got[0].Held != 2 {
		t.Fatalf("snapshot sources = %+v", got)
	}
	whole, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	requireSnapshotContract(t, snapshotContract(t), string(whole))
	encoded, err := json.Marshal(snapshot.Sync)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	var listed []map[string]any
	if err := json.Unmarshal(fields["sources"], &listed); err != nil || len(listed) != 1 {
		t.Fatalf("sources = %s", fields["sources"])
	}
	if _, leaked := listed[0]["newest_record_end"]; leaked {
		t.Fatal("the snapshot carries the time of a sleep record")
	}
}

func TestNoReportsMeanNoStatuses(t *testing.T) {
	app := newTestApp(t)
	statuses, err := app.GetSourceStatuses()
	if err != nil || statuses == nil || len(statuses) != 0 {
		t.Fatalf("statuses = %+v, %v", statuses, err)
	}
}

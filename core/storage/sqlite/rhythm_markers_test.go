package sqlite

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testRhythmMarker(now time.Time) RhythmMarkerRecord {
	end := now.Add(-30 * time.Minute)
	return RhythmMarkerRecord{
		MarkerID: "marker_test_01",
		Kind:     RhythmMarkerTravel,
		StartAt:  now.Add(-2 * time.Hour),
		EndAt:    &end,
		ZoneID:   "America/New_York",
		Note:     "Private travel context",
		Provenance: SleepObservationProvenance{
			AcquisitionMethod: ProvenanceAcquisitionManual,
			EvidenceStatus:    ProvenanceEvidenceUserReported,
			RecordedAt:        now,
		},
	}
}

func TestRhythmMarkersAreImmutableContractRecords(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	record := testRhythmMarker(now)
	if err := store.CreateRhythmMarker(ctx, record); err != nil {
		t.Fatal(err)
	}

	records, err := store.ListRhythmMarkers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].MarkerID != record.MarkerID || records[0].Note != record.Note {
		t.Fatalf("stored markers = %#v", records)
	}
	if records[0].StartAt.Location() != time.UTC || records[0].EndAt == nil || records[0].EndAt.Location() != time.UTC {
		t.Fatalf("marker timestamps were not normalized to UTC: %#v", records[0])
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE local_rhythm_markers SET kind = 'illness' WHERE marker_id = ?`, record.MarkerID); err == nil {
		t.Fatal("database allowed a marker to be edited in place")
	}

	exported, err := store.ExportRhythmMarkers(ctx, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if exported.SchemaVersion != "v1" || len(exported.Markers) != 1 || exported.Markers[0].Note != record.Note {
		t.Fatalf("marker export = %#v", exported)
	}
	if _, err := store.ExportRhythmMarkers(ctx, time.Time{}); err == nil {
		t.Fatal("zero export time was accepted")
	}
}

func TestRhythmMarkerValidationPreservesMedicalAndPrivacyBoundaries(t *testing.T) {
	now := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	base := testRhythmMarker(now)

	tests := []struct {
		name   string
		mutate func(*RhythmMarkerRecord)
	}{
		{"unsupported kind", func(record *RhythmMarkerRecord) { record.Kind = "diagnosis" }},
		{"machine local zone", func(record *RhythmMarkerRecord) { record.ZoneID = "Local" }},
		{"padded note", func(record *RhythmMarkerRecord) { record.Note = " padded" }},
		{"future start", func(record *RhythmMarkerRecord) { record.StartAt = now.Add(time.Minute) }},
		{"future end", func(record *RhythmMarkerRecord) {
			end := now.Add(time.Minute)
			record.EndAt = &end
		}},
		{"reversed range", func(record *RhythmMarkerRecord) {
			end := record.StartAt
			record.EndAt = &end
		}},
		{"non-manual acquisition", func(record *RhythmMarkerRecord) {
			record.Provenance.AcquisitionMethod = ProvenanceAcquisitionFileImport
		}},
		{"inferred evidence", func(record *RhythmMarkerRecord) { record.Provenance.EvidenceStatus = ProvenanceEvidenceInferred }},
		{"source identifier", func(record *RhythmMarkerRecord) { record.Provenance.SourceRecordID = "remote-row" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := base
			if base.EndAt != nil {
				end := *base.EndAt
				record.EndAt = &end
			}
			test.mutate(&record)
			if err := validateRhythmMarker(normalizeRhythmMarker(record)); err == nil {
				t.Fatalf("invalid marker was accepted: %#v", record)
			}
		})
	}
}

func TestDeleteRhythmMarkerErasesPrivateBytesFromDatabaseAndWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rhythm-marker-erasure.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	now := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	privateMarker := "rhythm-marker-erasure-5c72b18f"
	record := testRhythmMarker(now)
	record.Note = privateMarker
	if err := store.CreateRhythmMarker(ctx, record); err != nil {
		t.Fatal(err)
	}

	readFiles := func() ([]byte, []byte) {
		t.Helper()
		databaseBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		walBytes, err := os.ReadFile(path + "-wal")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		return databaseBytes, walBytes
	}
	databaseBytes, walBytes := readFiles()
	if !bytes.Contains(databaseBytes, []byte(privateMarker)) && !bytes.Contains(walBytes, []byte(privateMarker)) {
		t.Fatal("private marker note was not persisted before the erasure check")
	}

	if err := store.DeleteRhythmMarker(ctx, record.MarkerID); err != nil {
		t.Fatal(err)
	}
	databaseBytes, walBytes = readFiles()
	if bytes.Contains(databaseBytes, []byte(privateMarker)) || bytes.Contains(walBytes, []byte(privateMarker)) {
		t.Fatal("erased marker note remains in the SQLite database or WAL")
	}
	markers, err := store.ListRhythmMarkers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 0 {
		t.Fatalf("erased marker remained queryable: %#v", markers)
	}
	if err := store.DeleteRhythmMarker(ctx, record.MarkerID); !errors.Is(err, ErrRhythmMarkerNotFound) {
		t.Fatalf("second erasure error = %v", err)
	}
}

func TestDeleteAllIncludesRhythmMarkers(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	if err := store.CreateRhythmMarker(ctx, testRhythmMarker(now)); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAll(ctx); err != nil {
		t.Fatal(err)
	}
	markers, err := store.ListRhythmMarkers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 0 {
		t.Fatalf("DeleteAll retained markers: %#v", markers)
	}
}

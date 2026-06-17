package readmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"non24.app/server/internal/auth"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

func TestEffectiveSleepSessionsAppliesSupersedingCorrections(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "zeitboardd.db"), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.RegisterDevice(ctx, "dev_test", "desktop", auth.HashToken("token"), time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	originalStart := time.Date(2026, 3, 1, 4, 0, 0, 0, time.UTC)
	firstCorrection := originalStart.Add(30 * time.Minute)
	secondCorrection := originalStart.Add(90 * time.Minute)
	req := syncmodel.PushRequest{
		SchemaVersion: syncmodel.SchemaVersion,
		Records: []syncmodel.PushRecord{
			testPushRecord("obs_sleep_01", syncmodel.KindObservation, testSleepObservationPayload("obs_sleep_01", originalStart, originalStart.Add(8*time.Hour))),
			testPushRecord("cor_sleep_01", syncmodel.KindCorrection, testSleepStartCorrectionPayload("cor_sleep_01", "obs_sleep_01", "", firstCorrection)),
			testPushRecord("cor_sleep_02", syncmodel.KindCorrection, testSleepStartCorrectionPayload("cor_sleep_02", "obs_sleep_01", "cor_sleep_01", secondCorrection)),
		},
	}
	if err := syncmodel.ValidatePushRequest(&req); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Append(ctx, "dev_test", req.Records); err != nil {
		t.Fatal(err)
	}

	sessions, err := (SleepReader{Store: st}).EffectiveSleepSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("effective session count = %d, want 1", len(sessions))
	}
	start := sessions[0].Intervals[0].Interval.Start.UTC
	if !start.Equal(secondCorrection) {
		t.Fatalf("effective start = %s, want superseding correction %s", start, secondCorrection)
	}
	if start.Equal(originalStart) {
		t.Fatalf("effective start still matches raw observation start")
	}
	corrections := sessions[0].Intervals[0].StartEvidence.CorrectionIDs
	if len(corrections) != 1 || string(corrections[0]) != "cor_sleep_02" {
		t.Fatalf("start evidence corrections = %v, want only cor_sleep_02", corrections)
	}
}

func testPushRecord(id string, kind syncmodel.Kind, payload json.RawMessage) syncmodel.PushRecord {
	return syncmodel.PushRecord{
		RecordID:  id,
		Kind:      kind,
		CreatedAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		Payload:   payload,
	}
}

func testSleepObservationPayload(id string, start, end time.Time) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"observation_id":%q,"kind":"sleep_episode","start_at":%q,"end_at":%q,"zone_id":"America/New_York","sleep":{"classification":"principal"},"provenance":{"acquisition_method":"synthetic","evidence_status":"directly_observed","recorded_at":"2026-03-01T12:00:00Z","source_record_id":"raw-source-id"}}`,
		id,
		start.UTC().Format(time.RFC3339),
		end.UTC().Format(time.RFC3339),
	))
}

func testSleepStartCorrectionPayload(id, targetID, supersedesID string, start time.Time) json.RawMessage {
	supersedes := ""
	if supersedesID != "" {
		supersedes = fmt.Sprintf(`,"supersedes_correction_id":%q`, supersedesID)
	}
	return json.RawMessage(fmt.Sprintf(
		`{"correction_id":%q,"target_observation_id":%q%s,"created_at":%q,"reason":"user_edit","changes":{"start_at":%q}}`,
		id,
		targetID,
		supersedes,
		start.Add(10*time.Minute).UTC().Format(time.RFC3339),
		start.UTC().Format(time.RFC3339),
	))
}

package syncmodel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The Android companion builds these payloads by hand in Kotlin, in
// apps/android/.../data/SyncContract.kt. Nothing in the Go build imports that
// file, so the two can drift silently — and did: the first end-to-end push was
// rejected because the Kotlin used an acquisition method that is not in the
// server's closed enum, and no unit test on either side could see it.
//
// These fixtures are copied from what the Kotlin emits. If a change here makes
// them fail, the Kotlin has to change with it.

const androidObservationPayload = `{"observation_id":"hc-6735800bfc6c2f403e72d700",` +
	`"kind":"sleep_episode","start_at":"2026-08-04T04:00:00Z","end_at":"2026-08-04T12:00:00Z",` +
	`"zone_id":"America/New_York","sleep":{"classification":"principal"},` +
	`"provenance":{"acquisition_method":"health_connect","evidence_status":"directly_observed",` +
	`"recorded_at":"2026-08-04T12:05:00Z","source_record_id":"episode-1"}}`

const androidCorrectionPayload = `{"correction_id":"cor-c5f66660febe533547745cd3",` +
	`"target_observation_id":"hc-6735800bfc6c2f403e72d700",` +
	`"supersedes_correction_id":"cor-1111111111111111aaaaaaaa",` +
	`"created_at":"2026-08-04T13:00:00Z","reason":"source_conflict",` +
	`"changes":{"start_at":"2026-08-04T04:30:00Z","end_at":"2026-08-04T12:00:00Z"}}`

func androidBatch(t *testing.T) *PushRequest {
	t.Helper()
	return &PushRequest{
		SchemaVersion: SchemaVersion,
		Records: []PushRecord{
			{
				RecordID:  "hc-6735800bfc6c2f403e72d700",
				Kind:      KindObservation,
				CreatedAt: time.Date(2026, 8, 4, 12, 10, 0, 0, time.UTC),
				Payload:   json.RawMessage(androidObservationPayload),
			},
			{
				RecordID:  "cor-c5f66660febe533547745cd3",
				Kind:      KindCorrection,
				CreatedAt: time.Date(2026, 8, 4, 13, 10, 0, 0, time.UTC),
				Payload:   json.RawMessage(androidCorrectionPayload),
			},
		},
	}
}

// TestAndroidSyncPayloadsValidate is the cross-language guard.
func TestAndroidSyncPayloadsValidate(t *testing.T) {
	request := androidBatch(t)
	if err := ValidatePushRequest(request); err != nil {
		t.Fatalf("the Android companion's payload shape is no longer accepted: %v", err)
	}
}

// TestAndroidRecordIdsMatchTheIdentifierRule pins the id shape the Kotlin
// derives by hashing. A longer hash prefix or a different separator would push
// these past the pattern.
func TestAndroidRecordIdsMatchTheIdentifierRule(t *testing.T) {
	for _, id := range []string{
		"hc-6735800bfc6c2f403e72d700",
		"cor-c5f66660febe533547745cd3",
	} {
		if !identifierPattern.MatchString(id) {
			t.Errorf("Android id %q does not satisfy the server identifier rule", id)
		}
	}
}

// TestAndroidAcquisitionMethodIsInTheClosedEnum names the exact defect the
// end-to-end run caught, so a future rename fails here rather than at a device
// pushing into a real instance.
func TestAndroidAcquisitionMethodIsInTheClosedEnum(t *testing.T) {
	if !strings.Contains(androidObservationPayload, `"acquisition_method":"health_connect"`) {
		t.Fatal("the Android fixture no longer claims health_connect provenance")
	}

	broken := strings.Replace(
		androidObservationPayload,
		`"acquisition_method":"health_connect"`,
		`"acquisition_method":"device_sensor"`,
		1,
	)
	request := androidBatch(t)
	request.Records = request.Records[:1]
	request.Records[0].Payload = json.RawMessage(broken)

	if err := ValidatePushRequest(request); err == nil {
		t.Fatal("an acquisition method outside the closed enum was accepted")
	}
}

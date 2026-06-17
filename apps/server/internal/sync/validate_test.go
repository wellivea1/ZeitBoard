package syncmodel

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidatePushRequestCoversTimeZonesAndHalfOpenIntervals(t *testing.T) {
	valid := requestWithPayload(t, validObservation("obs_sleep_01", "America/New_York", "2026-03-05T04:30:00Z", "2026-03-05T12:30:00Z"))
	if err := ValidatePushRequest(&valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	invalidZone := requestWithPayload(t, validObservation("obs_sleep_01", "Mars/Base", "2026-03-05T04:30:00Z", "2026-03-05T12:30:00Z"))
	if err := ValidatePushRequest(&invalidZone); err == nil {
		t.Fatal("invalid IANA zone was accepted")
	}

	closedInterval := requestWithPayload(t, validObservation("obs_sleep_01", "America/New_York", "2026-03-05T04:30:00Z", "2026-03-05T04:30:00Z"))
	if err := ValidatePushRequest(&closedInterval); err == nil {
		t.Fatal("zero-duration interval was accepted")
	}
}

func TestValidatePushRequestRejectsUnknownPayloadFields(t *testing.T) {
	payload := strings.Replace(validObservation("obs_sleep_01", "America/New_York", "2026-03-05T04:30:00Z", "2026-03-05T12:30:00Z"), `"provenance":`, `"extra":"nope","provenance":`, 1)
	req := requestWithPayload(t, payload)
	if err := ValidatePushRequest(&req); err == nil {
		t.Fatal("payload with an unknown field was accepted")
	}
}

func requestWithPayload(t *testing.T, payload string) PushRequest {
	t.Helper()
	raw := `{"schema_version":"v1","records":[{"recordId":"obs_sleep_01","kind":"observation","createdAt":"2026-03-05T12:40:00Z","payload":` + payload + `}]}`
	var req PushRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatal(err)
	}
	return req
}

func validObservation(id, zoneID, startAt, endAt string) string {
	return `{"observation_id":"` + id + `","kind":"sleep_episode","start_at":"` + startAt + `","end_at":"` + endAt + `","zone_id":"` + zoneID + `","sleep":{"classification":"principal"},"provenance":{"acquisition_method":"synthetic","evidence_status":"directly_observed","recorded_at":"2026-03-05T12:35:00Z","source_record_id":"synthetic-source"}}`
}

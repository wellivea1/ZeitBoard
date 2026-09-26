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

func TestValidatePushRequestAcceptsOnlyWellFormedPlacements(t *testing.T) {
	placement := func(id, start, end, zone, extra string) PushRequest {
		t.Helper()
		payload := `{"placement_id":"` + id + `","task_id":"task_flexible_01","start_at":"` + start + `","end_at":"` + end +
			`","zone_id":"` + zone + `","created_at":"2026-03-05T12:40:00Z"` + extra + `}`
		raw := `{"schema_version":"v1","records":[{"recordId":"event_placement_01","kind":"placement","createdAt":"2026-03-05T12:40:00Z","payload":` + payload + `}]}`
		var req PushRequest
		if err := json.Unmarshal([]byte(raw), &req); err != nil {
			t.Fatal(err)
		}
		return req
	}
	valid := placement("event_placement_01", "2026-03-06T14:00:00Z", "2026-03-06T15:30:00Z", "America/New_York", "")
	if err := ValidatePushRequest(&valid); err != nil {
		t.Fatalf("valid placement rejected: %v", err)
	}
	for name, req := range map[string]PushRequest{
		"another record's id":       placement("event_placement_02", "2026-03-06T14:00:00Z", "2026-03-06T15:30:00Z", "America/New_York", ""),
		"ends before it starts":     placement("event_placement_01", "2026-03-06T15:30:00Z", "2026-03-06T14:00:00Z", "America/New_York", ""),
		"longer than a day":         placement("event_placement_01", "2026-03-06T14:00:00Z", "2026-03-07T15:30:00Z", "America/New_York", ""),
		"unknown zone":              placement("event_placement_01", "2026-03-06T14:00:00Z", "2026-03-06T15:30:00Z", "Mars/Base", ""),
		"a title it must not carry": placement("event_placement_01", "2026-03-06T14:00:00Z", "2026-03-06T15:30:00Z", "America/New_York", `,"title":"private"`),
	} {
		if err := ValidatePushRequest(&req); err == nil {
			t.Errorf("placement with %s was accepted", name)
		}
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

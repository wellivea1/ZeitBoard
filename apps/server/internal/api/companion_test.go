package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"non24.app/server/internal/projection"
)

func TestCompanionRequiresDeviceAuthAndReturnsTypedRefusalWithoutSampleFallback(t *testing.T) {
	h := newTestHarness(t)
	status, _ := h.request(t, http.MethodGet, "/v2/companion", "", "")
	if status != http.StatusUnauthorized {
		t.Fatal("companion projection was public")
	}
	token := h.registerDevice(t, "synthetic-phone")
	status, body := h.request(t, http.MethodGet, "/v2/companion", token, "")
	var response projection.CompanionResponse
	if status != http.StatusOK || json.Unmarshal(body, &response) != nil {
		t.Fatal("companion response unavailable")
	}
	if response.SchemaVersion != "v2" || response.Status != "refused" || response.Refusal == nil || len(response.Forecasts) != 0 || response.Confidence != nil {
		t.Fatalf("empty history manufactured a forecast: %s", body)
	}
	if response.SourceCursor != 0 || response.AlgorithmVersion == "" || response.Provenance != "self_hosted_synced_sleep" {
		t.Fatal("missing snapshot provenance")
	}
}

func TestCompanionProjectsRealInstantsAndExpiryFromSyncedInputs(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "synthetic-phone")
	start := time.Date(2026, 2, 23, 1, 0, 0, 0, time.UTC)
	h.pushRecords(t, token, driftingSleepRecords(10, start, 24*time.Hour+50*time.Minute, 8*time.Hour)...)
	status, body := h.request(t, http.MethodGet, "/v2/companion", token, "")
	var response projection.CompanionResponse
	if status != http.StatusOK || json.Unmarshal(body, &response) != nil {
		t.Fatal("companion response unavailable")
	}
	if response.Status != "estimated" || len(response.Forecasts) != 7 || len(response.Sleep) != 10 || response.SourceCursor != 10 {
		t.Fatalf("unexpected companion state: %s", body)
	}
	if !response.ContainsSyntheticData {
		t.Fatal("explicitly synthetic source history was disguised as personal evidence")
	}
	if !response.ValidUntil.After(response.GeneratedAt) || response.ValidUntil.After(response.GeneratedAt.Add(15*time.Minute)) {
		t.Fatal("cache assessment lacks a bounded expiry")
	}
	for _, forecast := range response.Forecasts {
		if !forecast.Sleep.EndAt.After(forecast.Sleep.StartAt) || !forecast.Waking.EndAt.After(forecast.Waking.StartAt) || forecast.Sleep.ZoneID != "America/New_York" {
			t.Fatal("invalid typed windows")
		}
	}
	for _, forbidden := range []string{"source_record_id", "token", "medication", "calendar", "synthetic-source-plaintext"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("companion leaked %q", forbidden)
		}
	}
}

// The forecasts pair each sleep with the waking window after it, so the hours
// someone is in before the next sleep have no window. Without the end of that
// stretch a companion draws the next sleep beginning sharply at its earliest
// plausible onset, and has nothing to say the onset is a range with.
func TestCompanionSaysHowLongTheWakingStretchUnderWayIsLikely(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "synthetic-phone")
	// The newest night ends at 08:00 on the harness's day; its clock reads noon.
	start := time.Date(2026, 2, 23, 16, 30, 0, 0, time.UTC)
	h.pushRecords(t, token, driftingSleepRecords(10, start, 24*time.Hour+50*time.Minute, 8*time.Hour)...)
	status, body := h.request(t, http.MethodGet, "/v2/companion", token, "")
	var response projection.CompanionResponse
	if status != http.StatusOK || json.Unmarshal(body, &response) != nil || response.Status != "estimated" {
		t.Fatalf("companion response unavailable: %s", body)
	}
	next := response.Forecasts[0].Sleep
	if !next.StartAt.After(response.GeneratedAt) {
		t.Fatalf("the test needs the next sleep still ahead: %s", body)
	}
	until := response.WakingUntil
	if until == nil {
		t.Fatalf("no end to the waking stretch under way: %s", body)
	}
	// It ends inside the next sleep's envelope: after the earliest the sleep
	// may begin, which is what makes the onset a range, and before its end.
	if !until.After(next.StartAt) || !until.Before(next.EndAt) {
		t.Fatalf("waking until %s is not inside the next sleep envelope %s to %s", until, next.StartAt, next.EndAt)
	}

	// A refusal has no forecast, so it has no stretch either.
	refused := newTestHarness(t)
	_, body = refused.request(t, http.MethodGet, "/v2/companion", refused.registerDevice(t, "synthetic-phone"), "")
	if strings.Contains(string(body), "waking_until") {
		t.Fatalf("a refusal carried a waking stretch: %s", body)
	}
}

func TestCompanionReflectsErasureInsteadOfKeepingPriorEstimate(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "synthetic-phone")
	h.pushRecords(t, token, driftingSleepRecords(7, time.Date(2026, 2, 25, 1, 0, 0, 0, time.UTC), 24*time.Hour, 8*time.Hour)...)
	status, _ := h.request(t, http.MethodPost, "/v1/sync/erase", token, `{"schema_version":"v1","record_ids":["obs_sleep_01"]}`)
	if status != http.StatusOK {
		t.Fatal("erasure failed")
	}
	_, body := h.request(t, http.MethodGet, "/v2/companion", token, "")
	var response projection.CompanionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "refused" || len(response.Forecasts) != 0 {
		t.Fatal("erasure retained the previous forecast")
	}
	for _, row := range response.Sleep {
		if row.StartAt.Equal(time.Date(2026, 2, 26, 1, 0, 0, 0, time.UTC)) {
			t.Fatal("erased sleep remained in the projection")
		}
	}
}

func TestPullSupportsBoundedClientPagesAndRejectsInvalidLimits(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "synthetic-phone")
	h.pushRecords(t, token, driftingSleepRecords(3, time.Date(2026, 3, 1, 1, 0, 0, 0, time.UTC), 24*time.Hour, 8*time.Hour)...)
	status, body := h.request(t, http.MethodGet, "/v1/sync/pull?since=0&limit=1", token, "")
	var page struct {
		Records []json.RawMessage `json:"records"`
		Cursor  int64             `json:"cursor"`
	}
	if status != http.StatusOK || json.Unmarshal(body, &page) != nil || len(page.Records) != 1 || page.Cursor != 1 {
		t.Fatal("page limit was not honored")
	}
	for _, limit := range []string{"0", "-1", "501", "invalid"} {
		status, _ := h.request(t, http.MethodGet, "/v1/sync/pull?limit="+limit, token, "")
		if status != http.StatusBadRequest {
			t.Fatal("invalid page limit was accepted")
		}
	}
}

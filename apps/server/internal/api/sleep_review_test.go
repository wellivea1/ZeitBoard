package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"non24.app/server/internal/projection"
)

func TestSleepReviewIsPrivateAndCanResolveConcurrentCorrections(t *testing.T) {
	h := newTestHarness(t)
	path := "/v1/sleep/obs_review/review"
	status, _ := h.request(t, http.MethodGet, path, "", "")
	if status != http.StatusUnauthorized {
		t.Fatal("private editing context was public")
	}
	token := h.registerDevice(t, "synthetic-review-phone")
	h.pushRecords(t, token, syncRecord("obs_review", "observation", observationPayload("obs_review", "private-source-label")),
		syncRecord("obs_unrelated", "observation", observationPayload("obs_unrelated", "unrelated-private-source")))
	for i := 1; i <= 2; i++ {
		id := fmt.Sprintf("cor_review_%d", i)
		payload := fmt.Sprintf(`{"correction_id":%q,"target_observation_id":"obs_review","created_at":"2026-03-05T13:00:00Z","reason":"user_edit","acquisition_method":"manual","based_on_source_revision":"2026-03-05T12:35:00Z","changes":{"start_at":"2026-03-05T04:%d0:00Z"}}`, id, i)
		h.pushRecords(t, token, syncRecord(id, "correction", payload))
	}
	status, body := h.request(t, http.MethodGet, path, token, "")
	var response projection.SleepReviewResponse
	if status != http.StatusOK || json.Unmarshal(body, &response) != nil {
		t.Fatal("review unavailable")
	}
	if response.SourceCursor != 4 || !response.NeedsReview || len(response.ManualCorrections) != 2 || response.EffectiveWindow != response.SourceWindow {
		t.Fatal("dispute lost its review context")
	}
	for _, secret := range []string{"private-source", "obs_unrelated", "source_record_id", "token"} {
		if strings.Contains(string(body), secret) {
			t.Fatal("review included unrelated private fields")
		}
	}
	payload := `{"correction_id":"cor_resolution","target_observation_id":"obs_review","created_at":"2026-03-05T14:00:00Z","reason":"user_edit","acquisition_method":"manual","based_on_source_revision":"2026-03-05T12:35:00Z","supersedes_correction_ids":["cor_review_1","cor_review_2"],"changes":{"start_at":"2026-03-05T04:20:00Z","end_at":"2026-03-05T12:30:00Z","sleep_classification":"principal","excluded":false}}`
	h.pushRecords(t, token, syncRecord("cor_resolution", "correction", payload))
	_, body = h.request(t, http.MethodGet, path, token, "")
	if json.Unmarshal(body, &response) != nil || response.NeedsReview || len(response.ManualCorrections) != 1 || response.EffectiveWindow.StartAt.Minute() != 20 {
		t.Fatal("explicit review did not resolve the dispute")
	}
	status, _ = h.request(t, http.MethodPost, "/v1/sync/erase", token, `{"schema_version":"v1","record_ids":["obs_review"]}`)
	if status != http.StatusOK {
		t.Fatal("erasure failed")
	}
	status, body = h.request(t, http.MethodGet, path, token, "")
	if status != http.StatusNotFound || strings.Contains(string(body), "2026-") {
		t.Fatal("erased review retained timestamps")
	}
}

func TestSleepReviewDisablesHTTPCaching(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "synthetic-review-cache")
	req, err := http.NewRequest(http.MethodGet, h.server.URL+"/v1/sleep/obs_absent/review", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("review response could be cached")
	}
}

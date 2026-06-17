package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	stdsync "sync"
	"testing"
	"time"

	"non24.app/server/internal/provider"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

const enrollmentSecret = "enrollment-secret-123"

type testHarness struct {
	dir       string
	st        *store.Store
	server    *httptest.Server
	client    *http.Client
	closeOnce stdsync.Once
}

func newTestHarness(t *testing.T, opts ...Option) *testHarness {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "zeitboardd.db"), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	srv := New(st, enrollmentSecret, opts...)
	srv.now = func() time.Time { return time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC) }
	httpServer := httptest.NewTLSServer(srv.Handler())
	h := &testHarness{dir: dir, st: st, server: httpServer, client: httpServer.Client()}
	t.Cleanup(h.Close)
	return h
}

func (h *testHarness) Close() {
	h.closeOnce.Do(func() {
		h.server.Close()
		_ = h.st.Close()
	})
}

type apiFakeProvider struct {
	response string
	calls    int
}

func (f *apiFakeProvider) Complete(_ context.Context, _ provider.Request) (provider.Response, error) {
	f.calls++
	return provider.Response{Text: f.response}, nil
}

func (f *apiFakeProvider) Status() provider.Status {
	return provider.Status{Configured: true, Provider: "fake", Model: "fake-model"}
}

func TestSyncRequiresAuthAndRegisteredDeviceCanPushAndPull(t *testing.T) {
	h := newTestHarness(t)

	status, _ := h.request(t, http.MethodGet, "/v1/sync/pull?since=0", "", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated pull status = %d, want %d", status, http.StatusUnauthorized)
	}

	desktopToken := h.registerDevice(t, "desktop")
	payload := observationPayload("obs_sleep_01", "sensitive-source-plaintext")
	push := syncBatch(syncRecord("obs_sleep_01", "observation", payload))
	status, body := h.request(t, http.MethodPost, "/v1/sync/push", desktopToken, push)
	if status != http.StatusOK {
		t.Fatalf("push status = %d body = %s", status, body)
	}
	var pushResp syncmodel.PushResponse
	if err := json.Unmarshal(body, &pushResp); err != nil {
		t.Fatal(err)
	}
	if pushResp.Cursor != 1 || pushResp.Accepted != 1 {
		t.Fatalf("push response = %+v, want cursor 1 accepted 1", pushResp)
	}

	phoneToken := h.registerDevice(t, "phone")
	status, body = h.request(t, http.MethodGet, "/v1/sync/pull?since=0", phoneToken, "")
	if status != http.StatusOK {
		t.Fatalf("pull status = %d body = %s", status, body)
	}
	var pullResp syncmodel.PullResponse
	if err := json.Unmarshal(body, &pullResp); err != nil {
		t.Fatal(err)
	}
	if pullResp.Cursor != 1 || len(pullResp.Records) != 1 {
		t.Fatalf("pull response cursor=%d records=%d, want cursor 1 records 1", pullResp.Cursor, len(pullResp.Records))
	}
	if pullResp.Records[0].RecordID != "obs_sleep_01" || pullResp.Records[0].Kind != syncmodel.KindObservation {
		t.Fatalf("pulled record = %+v", pullResp.Records[0])
	}
	if !bytes.Contains(pullResp.Records[0].Payload, []byte("sensitive-source-plaintext")) {
		t.Fatalf("pulled payload did not round-trip")
	}
}

func TestPushIsIdempotentByRecordID(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "desktop")

	push := syncBatch(syncRecord("obs_sleep_01", "observation", observationPayload("obs_sleep_01", "stable-source")))
	status, body := h.request(t, http.MethodPost, "/v1/sync/push", token, push)
	if status != http.StatusOK {
		t.Fatalf("first push status = %d body = %s", status, body)
	}
	status, body = h.request(t, http.MethodPost, "/v1/sync/push", token, push)
	if status != http.StatusOK {
		t.Fatalf("second push status = %d body = %s", status, body)
	}
	var resp syncmodel.PushResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Cursor != 1 || resp.Accepted != 0 {
		t.Fatalf("second push response = %+v, want cursor 1 accepted 0", resp)
	}
	count, err := h.st.CountRecords(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("record count = %d, want 1", count)
	}
}

func TestPayloadsAreEncryptedAtRestAndRoundTrip(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "desktop")
	secret := "disk-plaintext-sentinel"
	push := syncBatch(syncRecord("obs_sleep_01", "observation", observationPayload("obs_sleep_01", secret)))

	status, body := h.request(t, http.MethodPost, "/v1/sync/push", token, push)
	if status != http.StatusOK {
		t.Fatalf("push status = %d body = %s", status, body)
	}
	status, body = h.request(t, http.MethodGet, "/v1/sync/pull?since=0", token, "")
	if status != http.StatusOK {
		t.Fatalf("pull status = %d body = %s", status, body)
	}
	if !bytes.Contains(body, []byte(secret)) {
		t.Fatalf("pull response did not contain decrypted payload")
	}

	h.Close()
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(h.dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("%s contains plaintext payload bytes", entry.Name())
		}
	}
}

func TestMalformedAndOversizedBatchesDoNotMutateLog(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "desktop")

	valid := syncBatch(syncRecord("obs_sleep_01", "observation", observationPayload("obs_sleep_01", "first-source")))
	status, body := h.request(t, http.MethodPost, "/v1/sync/push", token, valid)
	if status != http.StatusOK {
		t.Fatalf("valid push status = %d body = %s", status, body)
	}

	badZone := strings.Replace(observationPayload("obs_sleep_03", "bad-zone-source"), `"zone_id":"America/New_York"`, `"zone_id":"Mars/Base"`, 1)
	batch := syncBatch(
		syncRecord("obs_sleep_02", "observation", observationPayload("obs_sleep_02", "should-not-commit")),
		syncRecord("obs_sleep_03", "observation", badZone),
	)
	status, _ = h.request(t, http.MethodPost, "/v1/sync/push", token, batch)
	if status != http.StatusBadRequest {
		t.Fatalf("malformed batch status = %d, want %d", status, http.StatusBadRequest)
	}

	oversizedPayload := observationPayload("obs_sleep_04", strings.Repeat("x", syncmodel.MaxPayloadBytes+1))
	status, _ = h.request(t, http.MethodPost, "/v1/sync/push", token, syncBatch(syncRecord("obs_sleep_04", "observation", oversizedPayload)))
	if status != http.StatusBadRequest && status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized batch status = %d", status)
	}

	count, err := h.st.CountRecords(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("record count after rejected batches = %d, want 1", count)
	}
	status, body = h.request(t, http.MethodGet, "/v1/sync/pull?since=1", token, "")
	if status != http.StatusOK {
		t.Fatalf("pull status = %d body = %s", status, body)
	}
	var pullResp syncmodel.PullResponse
	if err := json.Unmarshal(body, &pullResp); err != nil {
		t.Fatal(err)
	}
	if len(pullResp.Records) != 0 || pullResp.Cursor != 1 {
		t.Fatalf("pull after rejected batches = %+v, want no new records and cursor 1", pullResp)
	}
}

func TestDeviceListAndRevokeRejectsRevokedToken(t *testing.T) {
	h := newTestHarness(t)
	first := h.registerDeviceFull(t, "desktop")
	second := h.registerDeviceFull(t, "phone")

	status, body := h.request(t, http.MethodGet, "/v1/devices", first.Token, "")
	if status != http.StatusOK {
		t.Fatalf("list status = %d body = %s", status, body)
	}
	if !bytes.Contains(body, []byte(first.DeviceID)) || !bytes.Contains(body, []byte(second.DeviceID)) {
		t.Fatalf("device list missing registered devices: %s", body)
	}
	status, body = h.request(t, http.MethodPost, "/v1/devices/"+second.DeviceID+"/revoke", first.Token, "")
	if status != http.StatusOK {
		t.Fatalf("revoke status = %d body = %s", status, body)
	}
	status, _ = h.request(t, http.MethodGet, "/v1/status", second.Token, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, want %d", status, http.StatusUnauthorized)
	}
}

func TestStatusDisclosesProviderWithoutSecrets(t *testing.T) {
	fake := &apiFakeProvider{response: `{"schema_version":"v1","recommended_action":"answer_only","answer":"ok"}`}
	h := newTestHarness(t, WithProvider(fake, fake.Status()))
	token := h.registerDevice(t, "desktop")

	status, body := h.request(t, http.MethodGet, "/v1/status", token, "")
	if status != http.StatusOK {
		t.Fatalf("status endpoint = %d body = %s", status, body)
	}
	if !bytes.Contains(body, []byte(`"provider":"fake"`)) || !bytes.Contains(body, []byte(`"model":"fake-model"`)) {
		t.Fatalf("provider disclosure missing: %s", body)
	}
	if bytes.Contains(bytes.ToLower(body), []byte("key")) || bytes.Contains(bytes.ToLower(body), []byte("token")) {
		t.Fatalf("provider disclosure leaked secret-shaped fields: %s", body)
	}
}

func TestAssistantProposalDecisionEndpointRequiresOneUseToken(t *testing.T) {
	fake := &apiFakeProvider{response: `{"schema_version":"v1","recommended_action":"propose_place_task","target":{"task_id":"task_flexible_01"},"answer":"Queued for approval."}`}
	h := newTestHarness(t, WithProvider(fake, fake.Status()))
	token := h.registerDevice(t, "desktop")

	status, body := h.request(t, http.MethodPost, "/v1/assistant/message", token, assistantRequestBody())
	if status != http.StatusOK {
		t.Fatalf("assistant status = %d body = %s", status, body)
	}
	var assistantResp struct {
		Proposals []struct {
			ProposalID    string `json:"proposalId"`
			DecisionToken string `json:"decisionToken"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(body, &assistantResp); err != nil {
		t.Fatal(err)
	}
	if len(assistantResp.Proposals) != 1 || assistantResp.Proposals[0].DecisionToken == "" {
		t.Fatalf("assistant response missing proposal token: %s", body)
	}
	decision := fmt.Sprintf(`{"decision":"approved","token":%q}`, assistantResp.Proposals[0].DecisionToken)
	status, body = h.request(t, http.MethodPost, "/v1/proposals/"+assistantResp.Proposals[0].ProposalID+"/decision", token, decision)
	if status != http.StatusOK {
		t.Fatalf("decision status = %d body = %s", status, body)
	}
	status, _ = h.request(t, http.MethodPost, "/v1/proposals/"+assistantResp.Proposals[0].ProposalID+"/decision", token, decision)
	if status != http.StatusConflict {
		t.Fatalf("replay status = %d, want %d", status, http.StatusConflict)
	}
}

func (h *testHarness) registerDevice(t *testing.T, label string) string {
	t.Helper()
	return h.registerDeviceFull(t, label).Token
}

type registeredDevice struct {
	DeviceID string
	Token    string
}

func (h *testHarness) registerDeviceFull(t *testing.T, label string) registeredDevice {
	t.Helper()
	body := fmt.Sprintf(`{"enrollmentSecret":%q,"label":%q}`, enrollmentSecret, label)
	status, data := h.request(t, http.MethodPost, "/v1/devices", "", body)
	if status != http.StatusCreated {
		t.Fatalf("register status = %d body = %s", status, data)
	}
	var resp struct {
		DeviceID string `json:"deviceId"`
		Token    string `json:"token"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.DeviceID == "" || resp.Token == "" {
		t.Fatalf("register response missing credentials: %+v", resp)
	}
	return registeredDevice{DeviceID: resp.DeviceID, Token: resp.Token}
}

func (h *testHarness) request(t *testing.T, method, path, token, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, h.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}

func syncBatch(records ...string) string {
	return `{"schema_version":"v1","records":[` + strings.Join(records, ",") + `]}`
}

func syncRecord(id, kind, payload string) string {
	return fmt.Sprintf(`{"recordId":%q,"kind":%q,"createdAt":"2026-03-05T12:40:00Z","payload":%s}`, id, kind, payload)
}

func observationPayload(id, sourceRecordID string) string {
	return fmt.Sprintf(`{"observation_id":%q,"kind":"sleep_episode","start_at":"2026-03-05T04:30:00Z","end_at":"2026-03-05T12:30:00Z","zone_id":"America/New_York","sleep":{"classification":"principal"},"provenance":{"acquisition_method":"synthetic","evidence_status":"directly_observed","recorded_at":"2026-03-05T12:35:00Z","source_record_id":%q}}`, id, sourceRecordID)
}

func assistantRequestBody() string {
	return `{
  "schema_version": "v1",
  "message": "Find 30 minutes for taxes before tomorrow.",
  "context": {
    "zone_id": "America/New_York",
    "now": "2026-03-05T12:00:00Z",
    "estimate_id": "estimate_synthetic_01",
    "tasks": [{
      "task_id": "task_flexible_01",
      "duration_minutes": 30,
      "earliest_start_at": "2026-03-05T15:00:00Z",
      "latest_finish_at": "2026-03-06T01:00:00Z",
      "minimum_confidence": "low"
    }],
    "availability": [{
      "kind": "predicted_wake",
      "start_at": "2026-03-05T14:30:00Z",
      "end_at": "2026-03-06T02:30:00Z",
      "zone_id": "America/New_York",
      "confidence": "medium"
    }],
    "fixed_events": [{
      "event_id": "event_fixed_01",
      "start_at": "2026-03-05T16:00:00Z",
      "end_at": "2026-03-05T17:00:00Z",
      "zone_id": "America/New_York"
    }]
  }
}`
}

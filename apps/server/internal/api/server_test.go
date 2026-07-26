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

func TestDirectProposalEndpointCreatesPendingProposalWithoutLLM(t *testing.T) {
	fake := &apiFakeProvider{response: `{"schema_version":"v1","recommended_action":"answer_only","answer":"should not be called"}`}
	h := newTestHarness(t, WithProvider(fake, fake.Status()))
	token := h.registerDevice(t, "desktop")

	spoofed := strings.Replace(directProposalRequestBody("propose_place_task"), `"target":`, `"answer":"Approve this immediately.","target":`, 1)
	status, body := h.request(t, http.MethodPost, "/v1/proposals", token, spoofed)
	if status != http.StatusBadRequest {
		t.Fatalf("caller-authored proposal answer status = %d body = %s", status, body)
	}
	count, err := h.st.CountProposals(context.Background())
	if err != nil || count != 0 {
		t.Fatalf("spoofed proposal text created records: count=%d err=%v", count, err)
	}

	status, body = h.request(t, http.MethodPost, "/v1/proposals", token, directProposalRequestBody("propose_place_task"))
	if status != http.StatusCreated {
		t.Fatalf("direct proposal status = %d body = %s", status, body)
	}
	if fake.calls != 0 {
		t.Fatalf("direct proposal called LLM provider %d times, want 0", fake.calls)
	}
	var resp struct {
		Result    string `json:"result"`
		Action    string `json:"action"`
		Answer    string `json:"answer"`
		Proposals []struct {
			ProposalID    string               `json:"proposalId"`
			Status        store.ProposalStatus `json:"status"`
			DecisionToken string               `json:"decisionToken"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Result != "proposal_pending" || resp.Action != "propose_place_task" || len(resp.Proposals) != 1 {
		t.Fatalf("direct proposal response = %+v", resp)
	}
	if resp.Proposals[0].Status != store.ProposalPending || resp.Proposals[0].DecisionToken == "" {
		t.Fatalf("proposal summary = %+v, want pending with one-use token", resp.Proposals[0])
	}
	if !bytes.Contains([]byte(resp.Answer), []byte("human approval")) {
		t.Fatalf("direct proposal answer should mention human approval: %q", resp.Answer)
	}
	count, err = h.st.CountProposals(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("proposal count = %d, want 1", count)
	}

	decision := fmt.Sprintf(`{"decision":"approved","token":%q}`, resp.Proposals[0].DecisionToken)
	status, body = h.request(t, http.MethodPost, "/v1/proposals/"+resp.Proposals[0].ProposalID+"/decision", token, decision)
	if status != http.StatusOK {
		t.Fatalf("human decision status = %d body = %s", status, body)
	}
	status, _ = h.request(t, http.MethodPost, "/v1/proposals/"+resp.Proposals[0].ProposalID+"/decision", token, decision)
	if status != http.StatusConflict {
		t.Fatalf("decision replay status = %d, want %d", status, http.StatusConflict)
	}
}

func TestAnyEnrolledDeviceCanDecideViaListedToken(t *testing.T) {
	h := newTestHarness(t)
	agent := h.registerDevice(t, "mcp-agent")
	desktop := h.registerDevice(t, "desktop")

	// Device A (the agent) creates a pending proposal.
	status, body := h.request(t, http.MethodPost, "/v1/proposals", agent, directProposalRequestBody("propose_place_task"))
	if status != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", status, body)
	}

	// Device B (the desktop) lists proposals and receives a decision token for
	// the pending item, without having created it.
	status, body = h.request(t, http.MethodGet, "/v1/proposals", desktop, "")
	if status != http.StatusOK {
		t.Fatalf("list status = %d body = %s", status, body)
	}
	var list struct {
		Proposals []struct {
			ProposalID    string               `json:"proposalId"`
			Status        store.ProposalStatus `json:"status"`
			DecisionToken string               `json:"decisionToken"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Proposals) != 1 || list.Proposals[0].Status != store.ProposalPending {
		t.Fatalf("list = %+v, want one pending proposal", list)
	}
	if list.Proposals[0].DecisionToken == "" {
		t.Fatal("pending proposal must carry a decision token for enrolled devices")
	}

	// Device B decides it; the decision consumes the one-use nonce.
	decision := fmt.Sprintf(`{"decision":"approved","token":%q}`, list.Proposals[0].DecisionToken)
	status, body = h.request(t, http.MethodPost, "/v1/proposals/"+list.Proposals[0].ProposalID+"/decision", desktop, decision)
	if status != http.StatusOK {
		t.Fatalf("cross-device decision status = %d body = %s", status, body)
	}
	// Replay (by anyone, including the creator) is rejected.
	status, _ = h.request(t, http.MethodPost, "/v1/proposals/"+list.Proposals[0].ProposalID+"/decision", agent, decision)
	if status != http.StatusConflict {
		t.Fatalf("replay status = %d, want %d", status, http.StatusConflict)
	}
	// After the decision, the list no longer carries a token for it.
	status, body = h.request(t, http.MethodGet, "/v1/proposals", desktop, "")
	if status != http.StatusOK {
		t.Fatalf("relist status = %d", status)
	}
	list.Proposals = nil // reset: omitempty fields would otherwise survive re-unmarshal
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Proposals) != 1 || list.Proposals[0].Status != store.ProposalApproved || list.Proposals[0].DecisionToken != "" {
		t.Fatalf("decided proposal listing = %+v, want approved without token", list.Proposals[0])
	}
}

func TestDirectProposalEndpointRequiresAuthAndRejectsRevokedToken(t *testing.T) {
	h := newTestHarness(t)
	first := h.registerDeviceFull(t, "desktop")
	second := h.registerDeviceFull(t, "phone")

	status, _ := h.request(t, http.MethodPost, "/v1/proposals", "", directProposalRequestBody("propose_place_task"))
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated direct proposal status = %d, want %d", status, http.StatusUnauthorized)
	}
	status, body := h.request(t, http.MethodPost, "/v1/devices/"+second.DeviceID+"/revoke", first.Token, "")
	if status != http.StatusOK {
		t.Fatalf("revoke status = %d body = %s", status, body)
	}
	status, _ = h.request(t, http.MethodPost, "/v1/proposals", second.Token, directProposalRequestBody("propose_place_task"))
	if status != http.StatusUnauthorized {
		t.Fatalf("revoked direct proposal status = %d, want %d", status, http.StatusUnauthorized)
	}
}

func TestEraseHardDeletesMintsTombstonesAndBlocksResurrection(t *testing.T) {
	h := newTestHarness(t)
	desktop := h.registerDevice(t, "desktop")
	phone := h.registerDevice(t, "phone")
	start := time.Date(2026, 4, 1, 2, 0, 0, 0, time.UTC)
	records := driftingSleepRecords(3, start, 25*time.Hour, 8*time.Hour)
	h.pushRecords(t, desktop, records...)
	const erasedID = "obs_sleep_00" // first drifting record's id

	// Erase the first record from a DIFFERENT device (single-user model).
	eraseBody := fmt.Sprintf(`{"schema_version":"v1","record_ids":[%q]}`, erasedID)
	status, body := h.request(t, http.MethodPost, "/v1/sync/erase", phone, eraseBody)
	if status != http.StatusOK {
		t.Fatalf("erase status = %d body = %s", status, body)
	}
	var eraseResp syncmodel.EraseResponse
	if err := json.Unmarshal(body, &eraseResp); err != nil {
		t.Fatal(err)
	}
	if eraseResp.Erased != 1 || eraseResp.Tombstones != 1 {
		t.Fatalf("erase response = %+v, want 1 erased + 1 tombstone", eraseResp)
	}

	// The pull stream no longer contains the record's payload; it contains a
	// tombstone whose payload carries only the record id.
	status, body = h.request(t, http.MethodGet, "/v1/sync/pull?since=0", phone, "")
	if status != http.StatusOK {
		t.Fatalf("pull status = %d", status)
	}
	var pull syncmodel.PullResponse
	if err := json.Unmarshal(body, &pull); err != nil {
		t.Fatal(err)
	}
	sawTombstone := false
	for _, envelope := range pull.Records {
		if envelope.RecordID == erasedID {
			if envelope.Kind != syncmodel.KindTombstone {
				t.Fatalf("erased record still present with kind %q", envelope.Kind)
			}
			sawTombstone = true
			var payload syncmodel.TombstonePayload
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.RecordID != erasedID {
				t.Fatalf("tombstone payload = %+v", payload)
			}
			if bytes.Contains(envelope.Payload, []byte("start_at")) {
				t.Fatal("tombstone payload must not contain observation data")
			}
		}
	}
	if !sawTombstone {
		t.Fatal("pull must deliver the tombstone to other devices")
	}

	// A stale device re-pushing the erased record is a silent no-op.
	h.pushRecords(t, desktop, records[0])
	status, body = h.request(t, http.MethodGet, "/v1/sync/pull?since=0", phone, "")
	if status != http.StatusOK {
		t.Fatalf("post-resurrection pull status = %d", status)
	}
	pull = syncmodel.PullResponse{}
	if err := json.Unmarshal(body, &pull); err != nil {
		t.Fatal(err)
	}
	for _, envelope := range pull.Records {
		if envelope.RecordID == erasedID && envelope.Kind != syncmodel.KindTombstone {
			t.Fatalf("erased record was resurrected: %+v", envelope)
		}
	}

	// The server estimate no longer uses the erased episode: only 2 remain,
	// which is below the estimator minimum, so overview honestly refuses.
	status, body = h.request(t, http.MethodGet, "/v1/overview", phone, "")
	if status != http.StatusOK {
		t.Fatalf("overview status = %d", status)
	}
	var overview struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &overview); err != nil {
		t.Fatal(err)
	}
	if overview.Status != "refused" {
		t.Fatalf("overview after erasing to 2 episodes = %q, want refused", overview.Status)
	}

	// Erasure requires auth; malformed requests are rejected.
	status, _ = h.request(t, http.MethodPost, "/v1/sync/erase", "", eraseBody)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated erase status = %d", status)
	}
	status, _ = h.request(t, http.MethodPost, "/v1/sync/erase", phone, `{"schema_version":"v1","record_ids":[]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("empty erase status = %d", status)
	}
	// Erasing again is idempotent (no new tombstone).
	status, body = h.request(t, http.MethodPost, "/v1/sync/erase", phone, eraseBody)
	if status != http.StatusOK {
		t.Fatalf("repeat erase status = %d", status)
	}
	eraseResp = syncmodel.EraseResponse{}
	if err := json.Unmarshal(body, &eraseResp); err != nil {
		t.Fatal(err)
	}
	if eraseResp.Tombstones != 0 {
		t.Fatalf("repeat erase minted %d tombstones, want 0", eraseResp.Tombstones)
	}
}

func TestProjectionEndpointsUseServerEstimateFromSyncedSleep(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "desktop")
	start := time.Date(2026, 2, 23, 1, 0, 0, 0, time.UTC)
	h.pushRecords(t, token, driftingSleepRecords(10, start, 24*time.Hour+50*time.Minute, 8*time.Hour)...)

	status, body := h.request(t, http.MethodGet, "/v1/overview", token, "")
	if status != http.StatusOK {
		t.Fatalf("overview status = %d body = %s", status, body)
	}
	var overview struct {
		Status                   string `json:"status"`
		CurrentEstimatedState    string `json:"currentEstimatedState"`
		TimeSinceWake            string `json:"timeSinceWake"`
		PredictedNextSleepWindow string `json:"predictedNextSleepWindow"`
		DriftEstimate            string `json:"driftEstimate"`
		Confidence               string `json:"confidence"`
		FixtureMode              bool   `json:"fixtureMode"`
	}
	if err := json.Unmarshal(body, &overview); err != nil {
		t.Fatal(err)
	}
	if overview.Status != "estimated" || overview.FixtureMode {
		t.Fatalf("overview response = %+v", overview)
	}
	if !strings.Contains(overview.DriftEstimate, "+50") {
		t.Fatalf("overview drift = %q, want +50 minutes", overview.DriftEstimate)
	}
	if overview.CurrentEstimatedState == "" || overview.TimeSinceWake == "" || overview.PredictedNextSleepWindow == "" {
		t.Fatalf("overview missing speakable fields: %+v", overview)
	}
	assertNoForbiddenProjectionFields(t, body)

	status, body = h.request(t, http.MethodGet, "/v1/rhythm", token, "")
	if status != http.StatusOK {
		t.Fatalf("rhythm status = %d body = %s", status, body)
	}
	var rhythm struct {
		Status     string `json:"status"`
		Projection struct {
			FixtureMode  bool   `json:"fixtureMode"`
			SlopeLabel   string `json:"slopeLabel"`
			ForecastRows []struct {
				DurationHours float64 `json:"durationHours"`
				CivilDate     string  `json:"civilDate"`
			} `json:"forecastRows"`
			ObservedRows []struct {
				ID        string `json:"id"`
				CivilDate string `json:"civilDate"`
			} `json:"observedRows"`
			DriftPoints []struct {
				ID        string `json:"id"`
				CivilDate string `json:"civilDate"`
			} `json:"driftPoints"`
		} `json:"projection"`
	}
	if err := json.Unmarshal(body, &rhythm); err != nil {
		t.Fatal(err)
	}
	if rhythm.Status != "estimated" || rhythm.Projection.FixtureMode {
		t.Fatalf("rhythm response = %+v", rhythm)
	}
	if !strings.Contains(rhythm.Projection.SlopeLabel, "+50") {
		t.Fatalf("rhythm slope label = %q, want +50 minutes", rhythm.Projection.SlopeLabel)
	}
	if len(rhythm.Projection.ForecastRows) < 2 {
		t.Fatalf("forecast rows = %d, want at least 2", len(rhythm.Projection.ForecastRows))
	}
	if rhythm.Projection.ForecastRows[1].DurationHours <= rhythm.Projection.ForecastRows[0].DurationHours {
		t.Fatalf("forecast did not widen: first=%v second=%v", rhythm.Projection.ForecastRows[0].DurationHours, rhythm.Projection.ForecastRows[1].DurationHours)
	}
	if len(rhythm.Projection.ObservedRows) < 7 || len(rhythm.Projection.DriftPoints) < 7 {
		t.Fatalf("rhythm missing screen-reader equivalent rows: %+v", rhythm.Projection)
	}
	if rhythm.Projection.ObservedRows[0].CivilDate == "" || rhythm.Projection.ForecastRows[0].CivilDate == "" || rhythm.Projection.DriftPoints[0].CivilDate == "" {
		t.Fatalf("rhythm missing structured civil dates: %+v", rhythm.Projection)
	}
	if !strings.HasPrefix(rhythm.Projection.ObservedRows[0].ID, "observed-") || !strings.HasPrefix(rhythm.Projection.DriftPoints[0].ID, "drift-") {
		t.Fatalf("rhythm IDs were not sanitized: %+v %+v", rhythm.Projection.ObservedRows[0], rhythm.Projection.DriftPoints[0])
	}
	assertNoForbiddenProjectionFields(t, body)

	status, body = h.request(t, http.MethodGet, "/v1/accuracy", token, "")
	if status != http.StatusOK {
		t.Fatalf("accuracy status = %d body = %s", status, body)
	}
	var accuracy struct {
		Status string `json:"status"`
		Report struct {
			Evaluations int     `json:"evaluations"`
			HitRate     float64 `json:"hitRate"`
		} `json:"report"`
	}
	if err := json.Unmarshal(body, &accuracy); err != nil {
		t.Fatal(err)
	}
	if accuracy.Status != "estimated" || accuracy.Report.Evaluations == 0 {
		t.Fatalf("accuracy response = %+v", accuracy)
	}
	assertNoForbiddenProjectionFields(t, body)
}

func TestProjectionEndpointsRefuseInsufficientDataWithoutFabricatingEstimate(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "desktop")
	start := time.Date(2026, 3, 1, 4, 0, 0, 0, time.UTC)
	h.pushRecords(t, token, driftingSleepRecords(3, start, 24*time.Hour+50*time.Minute, 8*time.Hour)...)

	for _, path := range []string{"/v1/overview", "/v1/rhythm", "/v1/accuracy"} {
		status, body := h.request(t, http.MethodGet, path, token, "")
		if status != http.StatusOK {
			t.Fatalf("%s status = %d body = %s", path, status, body)
		}
		var response struct {
			Status  string `json:"status"`
			Refusal struct {
				Code string `json:"code"`
			} `json:"refusal"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		if response.Status != "refused" || response.Refusal.Code != "insufficient_data" {
			t.Fatalf("%s refusal = %+v body = %s", path, response, body)
		}
		if path == "/v1/overview" && bytes.Contains(body, []byte("currentEstimatedState")) {
			t.Fatalf("overview refusal fabricated estimate fields: %s", body)
		}
		assertNoForbiddenProjectionFields(t, body)
	}
}

func TestProjectionEndpointsRequireAuthAndRejectRevokedTokens(t *testing.T) {
	h := newTestHarness(t)
	first := h.registerDeviceFull(t, "desktop")
	second := h.registerDeviceFull(t, "phone")

	status, _ := h.request(t, http.MethodGet, "/v1/overview", "", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated overview status = %d, want %d", status, http.StatusUnauthorized)
	}
	status, body := h.request(t, http.MethodPost, "/v1/devices/"+second.DeviceID+"/revoke", first.Token, "")
	if status != http.StatusOK {
		t.Fatalf("revoke status = %d body = %s", status, body)
	}
	status, _ = h.request(t, http.MethodGet, "/v1/rhythm", second.Token, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("revoked rhythm status = %d, want %d", status, http.StatusUnauthorized)
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

func (h *testHarness) pushRecords(t *testing.T, token string, records ...string) {
	t.Helper()
	status, body := h.request(t, http.MethodPost, "/v1/sync/push", token, syncBatch(records...))
	if status != http.StatusOK {
		t.Fatalf("push status = %d body = %s", status, body)
	}
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

func directProposalRequestBody(action string) string {
	return fmt.Sprintf(`{
  "schema_version": "v1",
  "recommended_action": %q,
  "target": {"task_id":"task_flexible_01"},
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
}`, action)
}

func driftingSleepRecords(count int, start time.Time, period, duration time.Duration) []string {
	records := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("obs_sleep_%02d", i)
		sleepStart := start.Add(time.Duration(i) * period)
		records = append(records, syncRecord(id, "observation", sleepObservationPayloadAt(id, sleepStart, sleepStart.Add(duration), fmt.Sprintf("sync-source-%02d", i))))
	}
	return records
}

func sleepObservationPayloadAt(id string, start, end time.Time, sourceRecordID string) string {
	return fmt.Sprintf(
		`{"observation_id":%q,"kind":"sleep_episode","start_at":%q,"end_at":%q,"zone_id":"America/New_York","sleep":{"classification":"principal"},"provenance":{"acquisition_method":"synthetic","evidence_status":"directly_observed","recorded_at":%q,"source_record_id":%q}}`,
		id,
		start.UTC().Format(time.RFC3339),
		end.UTC().Format(time.RFC3339),
		end.UTC().Add(5*time.Minute).Format(time.RFC3339),
		sourceRecordID,
	)
}

func assertNoForbiddenProjectionFields(t *testing.T, body []byte) {
	t.Helper()
	lower := strings.ToLower(string(body))
	for _, forbidden := range []string{
		`"payload"`,
		`"source_record_id"`,
		`"sourceids"`,
		`"observation_id"`,
		`"observationids"`,
		"obs_sleep_",
		"sync-source",
		`"inputsessionids"`,
		`"algorithmversion"`,
		`"zoneid"`,
		`"utc"`,
		`"notes"`,
		`"diagnosis"`,
		`"token"`,
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("projection leaked forbidden field/value %q in %s", forbidden, body)
		}
	}
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

func taskRevisionRecord(taskID string, revision int, title, status string) string {
	id := fmt.Sprintf("%s_r%d", taskID, revision)
	payload := fmt.Sprintf(
		`{"task_id":%q,"title":%q,"duration_minutes":45,"status":%q,"created_at":"2026-03-05T12:00:00Z","revision":%d,"updated_at":"2026-03-05T12:%02d:00Z"}`,
		taskID, title, status, revision, revision)
	return syncRecord(id, "task", payload)
}

func TestTaskRevisionSyncLifecycle(t *testing.T) {
	h := newTestHarness(t)
	desktop := h.registerDevice(t, "desktop")
	laptop := h.registerDevice(t, "laptop")

	// Two revisions of one task flow through the log; the server stores both
	// (LWW is the consumer's job) and any device can pull them.
	h.pushRecords(t, desktop, taskRevisionRecord("task_paperwork_01", 1, "File paperwork", "open"))
	h.pushRecords(t, desktop, taskRevisionRecord("task_paperwork_01", 2, "File paperwork (rescoped)", "done"))

	status, body := h.request(t, http.MethodGet, "/v1/sync/pull?since=0", laptop, "")
	if status != http.StatusOK {
		t.Fatalf("pull status = %d", status)
	}
	var pull syncmodel.PullResponse
	if err := json.Unmarshal(body, &pull); err != nil {
		t.Fatal(err)
	}
	taskRevisions := []string{}
	for _, envelope := range pull.Records {
		if envelope.Kind == syncmodel.KindTask {
			taskRevisions = append(taskRevisions, envelope.RecordID)
		}
	}
	if len(taskRevisions) != 2 || taskRevisions[0] != "task_paperwork_01_r1" || taskRevisions[1] != "task_paperwork_01_r2" {
		t.Fatalf("task revisions in pull = %v", taskRevisions)
	}

	// Validation is strict: mismatched revision-record id, bad revision, and
	// out-of-bounds fields are rejected.
	badID := syncRecord("task_paperwork_01_r9", "task",
		`{"task_id":"task_paperwork_01","title":"x","duration_minutes":45,"status":"open","created_at":"2026-03-05T12:00:00Z","revision":3,"updated_at":"2026-03-05T12:03:00Z"}`)
	if status, _ := h.request(t, http.MethodPost, "/v1/sync/push", desktop, syncBatch(badID)); status != http.StatusBadRequest {
		t.Fatalf("mismatched revision id accepted: %d", status)
	}
	badRevision := syncRecord("task_paperwork_01_r0", "task",
		`{"task_id":"task_paperwork_01","title":"x","duration_minutes":45,"status":"open","created_at":"2026-03-05T12:00:00Z","revision":0,"updated_at":"2026-03-05T12:03:00Z"}`)
	if status, _ := h.request(t, http.MethodPost, "/v1/sync/push", desktop, syncBatch(badRevision)); status != http.StatusBadRequest {
		t.Fatalf("revision 0 accepted: %d", status)
	}
	badStatus := taskRevisionRecord("task_paperwork_02", 1, "ok", "archived")
	if status, _ := h.request(t, http.MethodPost, "/v1/sync/push", desktop, syncBatch(badStatus)); status != http.StatusBadRequest {
		t.Fatalf("off-enum status accepted: %d", status)
	}

	// Deleting the task erases every pushed revision: payloads hard-deleted,
	// tombstones minted, resurrection blocked (ADR-0017 machinery, unchanged).
	eraseBody := `{"schema_version":"v1","record_ids":["task_paperwork_01_r1","task_paperwork_01_r2"]}`
	status, body = h.request(t, http.MethodPost, "/v1/sync/erase", laptop, eraseBody)
	if status != http.StatusOK {
		t.Fatalf("erase status = %d body = %s", status, body)
	}
	var erase syncmodel.EraseResponse
	if err := json.Unmarshal(body, &erase); err != nil {
		t.Fatal(err)
	}
	if erase.Erased != 2 || erase.Tombstones != 2 {
		t.Fatalf("erase = %+v, want 2 erased + 2 tombstones", erase)
	}
	h.pushRecords(t, desktop, taskRevisionRecord("task_paperwork_01", 1, "File paperwork", "open")) // silent no-op
	status, body = h.request(t, http.MethodGet, "/v1/sync/pull?since=0", desktop, "")
	if status != http.StatusOK {
		t.Fatalf("pull status = %d", status)
	}
	pull = syncmodel.PullResponse{}
	if err := json.Unmarshal(body, &pull); err != nil {
		t.Fatal(err)
	}
	for _, envelope := range pull.Records {
		if envelope.Kind == syncmodel.KindTask {
			t.Fatalf("erased task revision resurfaced: %s", envelope.RecordID)
		}
	}

	// Task records must not disturb server-side sleep estimation.
	start := time.Date(2026, 4, 1, 2, 0, 0, 0, time.UTC)
	h.pushRecords(t, desktop, driftingSleepRecords(10, start, 24*time.Hour+50*time.Minute, 8*time.Hour)...)
	h.pushRecords(t, desktop, taskRevisionRecord("task_paperwork_03", 1, "Unrelated task", "open"))
	status, body = h.request(t, http.MethodGet, "/v1/overview", desktop, "")
	if status != http.StatusOK {
		t.Fatalf("overview status = %d body = %s", status, body)
	}
	if !strings.Contains(string(body), "estimated") {
		t.Fatalf("estimation should proceed alongside task records: %s", body)
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSyncPushBatchesTaskAndSleepRecordsByCount(t *testing.T) {
	app := newTestApp(t)
	var batches [][]syncPushRecord
	var bodySizes []int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{
				SchemaVersion: "v1",
				DeviceID:      "device_desktop",
				Token:         "batch-token",
			})
		case "/v1/sync/push":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			var req syncPushRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatal(err)
			}
			batches = append(batches, req.Records)
			bodySizes = append(bodySizes, len(body))
			_ = json.NewEncoder(w).Encode(syncPushResponse{
				SchemaVersion: "v1",
				Cursor:        int64(len(batches)),
				Accepted:      len(req.Records),
			})
		case "/v1/sync/pull":
			_ = json.NewEncoder(w).Encode(syncPullResponse{
				SchemaVersion: "v1",
				Cursor:        int64(len(batches)),
				Records:       []syncEnvelope{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureBackendForTest(t, app, server.URL)

	for i := 0; i < 101; i++ {
		if _, err := app.AddTask(TaskInput{
			Title:           fmt.Sprintf("Batch task %03d", i),
			DurationMinutes: 30,
		}); err != nil {
			t.Fatal(err)
		}
	}
	seedSleepEntries(t, app, 101)

	status, err := app.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if status.LastError != "" || status.PushedCount != 202 {
		t.Fatalf("sync status = %#v, want 202 pushed without error", status)
	}
	if len(batches) != 4 {
		t.Fatalf("push batch count = %d, want 4", len(batches))
	}
	wantLengths := []int{100, 1, 100, 1}
	for i, want := range wantLengths {
		if len(batches[i]) != want {
			t.Fatalf("batch %d length = %d, want %d", i, len(batches[i]), want)
		}
		if bodySizes[i] > maxSyncPushRequestBytes {
			t.Fatalf("batch %d encoded body = %d bytes, maximum is %d", i, bodySizes[i], maxSyncPushRequestBytes)
		}
	}
	for i := 0; i < 2; i++ {
		for _, record := range batches[i] {
			if record.Kind == "task" {
				t.Fatalf("batch %d contains a task before all sleep batches completed", i)
			}
		}
	}
	for i := 2; i < 4; i++ {
		for _, record := range batches[i] {
			if record.Kind != "task" {
				t.Fatalf("batch %d contains %q after task batching started", i, record.Kind)
			}
		}
	}
}

func TestNextSyncPushBatchLengthUsesExactEncodedBodySize(t *testing.T) {
	createdAt := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	records := make([]syncPushRecord, 0, 18)
	for i := 0; i < 16; i++ {
		records = append(records, syncPushRecord{
			RecordID:  fmt.Sprintf("record_%02d", i),
			Kind:      "observation",
			CreatedAt: createdAt,
			Payload:   json.RawMessage(`"` + strings.Repeat("x", 60*1024) + `"`),
		})
	}

	tail := syncPushRecord{
		RecordID:  "record_16",
		Kind:      "observation",
		CreatedAt: createdAt,
		Payload:   json.RawMessage(`""`),
	}
	withEmptyTail := append(append([]syncPushRecord{}, records...), tail)
	base, err := json.Marshal(syncPushRequest{SchemaVersion: "v1", Records: withEmptyTail})
	if err != nil {
		t.Fatal(err)
	}
	padding := maxSyncPushRequestBytes - len(base)
	if padding <= 0 || padding+len(tail.Payload) > 64*1024 {
		t.Fatalf("test setup cannot create a server-sized boundary payload: base=%d padding=%d", len(base), padding)
	}
	tail.Payload = json.RawMessage(`"` + strings.Repeat("x", padding) + `"`)
	records = append(records, tail)

	nearBody, err := json.Marshal(syncPushRequest{SchemaVersion: "v1", Records: records})
	if err != nil {
		t.Fatal(err)
	}
	if len(nearBody) != maxSyncPushRequestBytes {
		t.Fatalf("near-limit body = %d bytes, want exactly %d", len(nearBody), maxSyncPushRequestBytes)
	}
	records = append(records, syncPushRecord{
		RecordID:  "record_17",
		Kind:      "observation",
		CreatedAt: createdAt,
		Payload:   json.RawMessage(`{}`),
	})
	overBody, err := json.Marshal(syncPushRequest{SchemaVersion: "v1", Records: records})
	if err != nil {
		t.Fatal(err)
	}
	if len(overBody) <= maxSyncPushRequestBytes {
		t.Fatalf("over-limit body = %d bytes, want more than %d", len(overBody), maxSyncPushRequestBytes)
	}

	length, err := nextSyncPushBatchLength(records)
	if err != nil {
		t.Fatal(err)
	}
	if length != len(records)-1 {
		t.Fatalf("batch length = %d, want %d records ending at exact byte limit", length, len(records)-1)
	}
}

func TestNextSyncPushBatchLengthRejectsSingleUnsendableRecord(t *testing.T) {
	record := syncPushRecord{
		RecordID:  "oversized_record",
		Kind:      "observation",
		CreatedAt: time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
		Payload:   json.RawMessage(`"` + strings.Repeat("x", maxSyncPushRequestBytes) + `"`),
	}

	length, err := nextSyncPushBatchLength([]syncPushRecord{record})
	if length != 0 {
		t.Fatalf("batch length = %d, want 0", length)
	}
	if err == nil ||
		!strings.Contains(err.Error(), record.RecordID) ||
		!strings.Contains(err.Error(), "encoded request body") ||
		!strings.Contains(err.Error(), fmt.Sprint(maxSyncPushRequestBytes)) {
		t.Fatalf("unexpected oversized-record error: %v", err)
	}
}

func TestTaskPushRetainsProgressAfterLaterBatchFails(t *testing.T) {
	app := newTestApp(t)
	pushCalls := 0
	var acceptedIDs []string
	failSecondBatch := true
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{
				SchemaVersion: "v1",
				DeviceID:      "device_desktop",
				Token:         "progress-token",
			})
		case "/v1/sync/push":
			pushCalls++
			var req syncPushRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if failSecondBatch && pushCalls == 2 {
				http.Error(w, "temporary failure", http.StatusServiceUnavailable)
				return
			}
			for _, record := range req.Records {
				acceptedIDs = append(acceptedIDs, record.RecordID)
			}
			_ = json.NewEncoder(w).Encode(syncPushResponse{
				SchemaVersion: "v1",
				Cursor:        int64(len(acceptedIDs)),
				Accepted:      len(req.Records),
			})
		case "/v1/sync/pull":
			_ = json.NewEncoder(w).Encode(syncPullResponse{
				SchemaVersion: "v1",
				Cursor:        int64(len(acceptedIDs)),
				Records:       []syncEnvelope{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureBackendForTest(t, app, server.URL)
	for i := 0; i < 101; i++ {
		if _, err := app.AddTask(TaskInput{
			Title:           fmt.Sprintf("Progress task %03d", i),
			DurationMinutes: 30,
		}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := app.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "error" || first.PushedCount != 100 {
		t.Fatalf("first sync = %#v, want 100 persisted pushes and an error", first)
	}
	store, err := app.requireStore()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.UnpushedTaskSyncRecords(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending tasks after partial success = %d, want 1", len(pending))
	}

	failSecondBatch = false
	second, err := app.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if second.LastError != "" || second.PushedCount != 1 {
		t.Fatalf("retry sync = %#v, want only the remaining task pushed", second)
	}
	if len(acceptedIDs) != 101 {
		t.Fatalf("accepted task records = %d, want 101 without replaying the first batch", len(acceptedIDs))
	}
	seen := make(map[string]struct{}, len(acceptedIDs))
	for _, id := range acceptedIDs {
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("task record %q was replayed after partial success", id)
		}
		seen[id] = struct{}{}
	}
}

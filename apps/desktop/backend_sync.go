package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"non24.app/core/agentactions"
	"non24.app/core/estimation"
	"non24.app/core/platform/privatefile"
	"non24.app/core/recompute"
	storage "non24.app/core/storage/sqlite"
)

const (
	backendRequestTimeout   = 10 * time.Second
	maxSyncPushRecords      = 100
	maxSyncPushRequestBytes = 1024 * 1024
)

var newBackendHTTPClient = func(insecureSkipVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // Explicit localhost/self-hosted dev escape hatch.
	}
	return &http.Client{Timeout: backendRequestTimeout, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("backend redirects are not supported; use the final HTTPS address")
		},
	}
}

type BackendSyncInput struct {
	Enabled            bool   `json:"enabled"`
	BackendURL         string `json:"backendUrl"`
	EnrollmentSecret   string `json:"enrollmentSecret"`
	DeviceLabel        string `json:"deviceLabel"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
}

type BackendSyncStatusDTO struct {
	TaskConflictCount   int    `json:"taskConflictCount"`
	Enabled             bool   `json:"enabled"`
	Status              string `json:"status"`
	BackendURL          string `json:"backendUrl"`
	DeviceID            string `json:"deviceId"`
	InsecureSkipVerify  bool   `json:"insecureSkipVerify"`
	LastSyncLabel       string `json:"lastSyncLabel"`
	LastError           string `json:"lastError"`
	PendingPushCount    int    `json:"pendingPushCount"`
	PendingErasureCount int    `json:"pendingErasureCount"`
	WaitingRecordCount  int    `json:"waitingRecordCount"`
	PushedCount         int    `json:"pushedCount"`
	PulledCount         int    `json:"pulledCount"`
	SkippedCount        int    `json:"skippedCount"`
	ErasuresPushed      int    `json:"erasuresPushed"`
	TombstonesApplied   int    `json:"tombstonesApplied"`
	Cursor              int64  `json:"cursor"`
}

type backendSyncConfig = storage.SyncConnection

type registerDeviceRequest struct {
	EnrollmentSecret string `json:"enrollmentSecret"`
	Label            string `json:"label"`
}

type registerDeviceResponse struct {
	SchemaVersion string `json:"schema_version"`
	DeviceID      string `json:"deviceId"`
	Token         string `json:"token"`
}

type syncPushRequest struct {
	SchemaVersion string           `json:"schema_version"`
	Records       []syncPushRecord `json:"records"`
}

type syncPushRecord struct {
	RecordID  string          `json:"recordId"`
	Kind      string          `json:"kind"`
	CreatedAt time.Time       `json:"createdAt"`
	Payload   json.RawMessage `json:"payload"`
}

type syncPushResponse struct {
	SchemaVersion string `json:"schema_version"`
	Cursor        int64  `json:"cursor"`
	Accepted      int    `json:"accepted"`
}

func (r *syncPushResponse) UnmarshalJSON(data []byte) error {
	var value struct {
		SchemaVersion string `json:"schema_version"`
		Cursor        *int64 `json:"cursor"`
		Accepted      *int   `json:"accepted"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if value.Cursor == nil || value.Accepted == nil {
		return errors.New("sync acknowledgment is incomplete")
	}
	*r = syncPushResponse{SchemaVersion: value.SchemaVersion, Cursor: *value.Cursor, Accepted: *value.Accepted}
	return nil
}

type syncEnvelope struct {
	Seq       int64           `json:"seq"`
	RecordID  string          `json:"recordId"`
	Kind      string          `json:"kind"`
	DeviceID  string          `json:"deviceId"`
	CreatedAt time.Time       `json:"createdAt"`
	Payload   json.RawMessage `json:"payload"`
}

type syncPullResponse struct {
	SchemaVersion string         `json:"schema_version"`
	Cursor        int64          `json:"cursor"`
	Records       []syncEnvelope `json:"records"`
}

type syncEraseRequest struct {
	SchemaVersion string   `json:"schema_version"`
	RecordIDs     []string `json:"record_ids"`
}

type syncEraseResponse struct {
	SchemaVersion string `json:"schema_version"`
	Erased        int    `json:"erased"`
	Tombstones    int    `json:"tombstones"`
	Cursor        int64  `json:"cursor"`
}

type syncTombstonePayload struct {
	RecordID   string `json:"record_id"`
	RecordKind string `json:"record_kind,omitempty"`
}

type serverOverviewResponse struct {
	SchemaVersion            string               `json:"schema_version"`
	Status                   string               `json:"status"`
	Refusal                  *RefusalDTO          `json:"refusal,omitempty"`
	CurrentEstimatedState    string               `json:"currentEstimatedState,omitempty"`
	TimeSinceWake            string               `json:"timeSinceWake,omitempty"`
	PredictedNextSleepWindow string               `json:"predictedNextSleepWindow,omitempty"`
	DriftEstimate            string               `json:"driftEstimate,omitempty"`
	Confidence               string               `json:"confidence,omitempty"`
	ConfidenceReasons        []string             `json:"confidenceReasons,omitempty"`
	NextUsefulTaskWindow     string               `json:"nextUsefulTaskWindow,omitempty"`
	SharingStatus            string               `json:"sharingStatus,omitempty"`
	MedicationEvents         []MedicationEventDTO `json:"medicationEvents"`
	FixtureMode              bool                 `json:"fixtureMode"`
	Disclaimer               string               `json:"disclaimer"`
}

type serverRhythmResponse struct {
	SchemaVersion string                        `json:"schema_version"`
	Status        string                        `json:"status"`
	Refusal       *estimation.EstimationRefusal `json:"refusal,omitempty"`
	Projection    *estimation.RhythmProjection  `json:"projection,omitempty"`
}

type backendHTTPError struct {
	StatusCode int
	Message    string
}

func (e backendHTTPError) Error() string {
	return fmt.Sprintf("backend returned HTTP %d: %s", e.StatusCode, e.Message)
}

type desktopBackendClient struct {
	baseURL string
	token   string
	client  *http.Client
	// Every request tells the outage whether the server answered. A passive
	// read, one nobody asked for by name, skips a server known to be down.
	outage  *backendOutage
	now     func() time.Time
	passive bool
}

func (a *App) GetBackendSyncStatus() (BackendSyncStatusDTO, error) {
	a.backendConfigMu.RLock()
	defer a.backendConfigMu.RUnlock()
	cfg, err := a.loadBackendSyncConfig()
	if err != nil {
		return BackendSyncStatusDTO{}, errors.New("Saved backend settings could not be read. Sync is unavailable.")
	}
	return a.backendSyncStatusCounts(cfg, syncCounts{}), nil
}

func (a *App) ConfigureBackendSync(input BackendSyncInput) (BackendSyncStatusDTO, error) {
	if !input.Enabled {
		return a.DisableBackendSync()
	}
	a.cancelBackendRun()
	a.backendSyncMu.Lock()
	configured := false
	defer func() {
		a.backendSyncMu.Unlock()
		if configured {
			a.wakeDesktopBackground()
		}
	}()
	if a.closing.Load() {
		return BackendSyncStatusDTO{}, errors.New("ZeitBoard is quitting; reopen it to configure sync.")
	}
	a.backendConfigMu.Lock()
	defer a.backendConfigMu.Unlock()
	baseURL, err := normalizeBackendURL(input.BackendURL)
	if err != nil {
		return BackendSyncStatusDTO{}, err
	}
	if err := validateBackendTLS(baseURL, input.InsecureSkipVerify); err != nil {
		return BackendSyncStatusDTO{}, err
	}
	store, err := a.requireStore()
	if err != nil {
		return BackendSyncStatusDTO{}, err
	}
	previous, _, err := store.LoadSyncConnection(a.applicationContext())
	if err != nil {
		return BackendSyncStatusDTO{}, errors.New("Saved backend settings could not be read.")
	}
	cfg := backendSyncConfig{Enabled: true, BackendURL: baseURL, InsecureSkipVerify: input.InsecureSkipVerify}
	label := strings.TrimSpace(input.DeviceLabel)
	if label == "" {
		label = "ZeitBoard desktop"
	}
	client := a.newDesktopBackendClient(cfg, "")
	var response registerDeviceResponse
	err = client.postJSON(a.applicationContext(), "/v1/devices", registerDeviceRequest{
		EnrollmentSecret: input.EnrollmentSecret,
		Label:            label,
	}, &response)
	if err != nil {
		return a.backendSyncStatusCounts(previous, syncCounts{}), errors.New(sanitizeBackendError(err))
	}
	if response.SchemaVersion != "v1" || response.DeviceID == "" || response.Token == "" {
		return a.backendSyncStatusCounts(previous, syncCounts{}), errors.New("Backend enrollment returned an invalid device credential; the previous connection was retained.")
	}
	cfg.DeviceID = response.DeviceID
	if err := store.SaveSyncEnrollment(a.applicationContext(), cfg, response.Token); err != nil {
		return a.backendSyncStatusCounts(previous, syncCounts{}), errors.New("Could not save the new enrollment; the previous connection was retained.")
	}
	configured = true
	return a.backendSyncStatusCounts(cfg, syncCounts{}), nil
}

func (a *App) DisableBackendSync() (BackendSyncStatusDTO, error) {
	a.cancelBackendRun()
	a.backendSyncMu.Lock()
	defer a.backendSyncMu.Unlock()
	a.backendConfigMu.Lock()
	defer a.backendConfigMu.Unlock()
	cfg, _ := a.loadBackendSyncConfig()
	cfg.Enabled = false
	cfg.LastError = ""
	if err := a.saveBackendSyncConfig(cfg); err != nil {
		return BackendSyncStatusDTO{}, err
	}
	return a.backendSyncStatusCounts(cfg, syncCounts{}), nil
}

func (a *App) SyncNow() (BackendSyncStatusDTO, error) {
	a.backendSyncMu.Lock()
	defer a.backendSyncMu.Unlock()
	if a.closing.Load() {
		return BackendSyncStatusDTO{}, errors.New("ZeitBoard is quitting; sync has stopped.")
	}
	return a.syncNowLocked(a.applicationContext())
}

func (a *App) syncNowLocked(parent context.Context) (BackendSyncStatusDTO, error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	a.backendRunMu.Lock()
	a.backendRunCancel = cancel
	a.backendRunMu.Unlock()
	defer func() { a.backendRunMu.Lock(); a.backendRunCancel = nil; a.backendRunMu.Unlock() }()
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		if cfg.Enabled {
			cfg.LastError = sanitizeBackendError(err)
			a.backendConfigMu.Lock()
			_ = a.saveBackendSyncConfig(cfg)
			a.backendConfigMu.Unlock()
		}
		return a.backendSyncStatusCounts(cfg, syncCounts{}), nil
	}
	counts, syncErr := a.syncRecords(ctx, cfg, token)
	if counts.pulled > 0 || counts.tombstonesApplied > 0 {
		a.announce("zeitboard:sync-applied")
	}
	a.backendConfigMu.Lock()
	defer a.backendConfigMu.Unlock()
	if syncErr != nil {
		cfg.LastError = sanitizeBackendError(syncErr)
		_ = a.saveBackendSyncConfig(cfg)
		return a.backendSyncStatusCounts(cfg, counts), nil
	}
	cfg.LastSyncAt = a.currentTime().UTC()
	cfg.LastError = ""
	if err := a.saveBackendSyncConfig(cfg); err != nil {
		return BackendSyncStatusDTO{}, err
	}
	return a.backendSyncStatusCounts(cfg, counts), nil
}

type syncCounts struct {
	pushed            int
	pulled            int
	skipped           int
	erasuresPushed    int
	tombstonesApplied int
}

// syncRecords runs one sync pass: download, reconcile, upload erasures and
// every pending kind in dependency order, then download what the upload made
// visible.
func (a *App) syncRecords(ctx context.Context, cfg backendSyncConfig, token string) (syncCounts, error) {
	defer a.requestLocalAnalysis(recompute.ReasonEvidence)
	counts := syncCounts{}
	store, err := a.requireStore()
	if err != nil {
		return counts, err
	}
	client := a.newDesktopBackendClient(cfg, token)
	// Learn remote erasure before replaying local data. A new enrollment starts
	// at zero and reconciles old acknowledgments only after a complete pull.
	counts.pulled, counts.skipped, counts.tombstonesApplied, err = a.pullRecords(ctx, store, client)
	if err != nil {
		return counts, err
	}
	if err := store.FinishSyncReconciliation(ctx); err != nil {
		return counts, err
	}
	counts.erasuresPushed, err = a.pushErasures(ctx, store, client)
	if err != nil {
		return counts, err
	}
	for _, push := range []func(context.Context, *storage.Store, desktopBackendClient) (int, error){
		a.pushSleepRecords, a.pushTaskRecords, a.pushPlacementRecords, a.pushMedicationRecords, a.pushMarkerRecords,
	} {
		pushed, err := push(ctx, store, client)
		counts.pushed += pushed
		if err != nil {
			return counts, err
		}
	}
	moreErasures, err := a.pushErasures(ctx, store, client)
	counts.erasuresPushed += moreErasures
	if err != nil {
		return counts, err
	}
	pulled, skipped, tombstones, err := a.pullRecords(ctx, store, client)
	counts.pulled += pulled
	counts.skipped += skipped
	counts.tombstonesApplied += tombstones
	if err == nil {
		var deferred int
		deferred, err = store.DeferredSyncRecordCount(ctx)
		if err == nil && deferred > 0 {
			err = errors.New("Some downloaded records are waiting for the records they belong to. They are saved on this computer; sync will retry.")
		}
	}
	return counts, err
}

// pushTaskRecords pushes the current revision of every locally-edited task as
// an immutable revision record (ADR-0020).
func (a *App) pushTaskRecords(ctx context.Context, store *storage.Store, client desktopBackendClient) (int, error) {
	return pushPendingSyncRecords(ctx, client, storage.MaxTaskSyncPageSize,
		store.PendingTaskSyncRecords, store.MarkTaskSyncRecordsPushed,
		func(record storage.TaskSyncRecord) syncPushRecord {
			return syncPushRecord{RecordID: record.RecordID, Kind: "task", CreatedAt: record.CreatedAt.UTC(), Payload: record.Payload}
		})
}

// pushPlacementRecords sends accepted times, so the companion can draw the
// day's plans (ADR-0047). Removing one queues its erasure instead.
func (a *App) pushPlacementRecords(ctx context.Context, store *storage.Store, client desktopBackendClient) (int, error) {
	return pushPendingSyncRecords(ctx, client, storage.MaxPlacementSyncPageSize,
		store.PendingPlacementSyncRecords, store.MarkPlacementSyncRecordsPushed,
		func(record storage.PlacementSyncRecord) syncPushRecord {
			return syncPushRecord{RecordID: record.RecordID, Kind: "placement", CreatedAt: record.CreatedAt.UTC(), Payload: record.Payload}
		})
}

// pushErasures propagates local hard-deletes of any kind, including uncertain
// uploads, to the backend, which hard-deletes the synced copies and mints
// tombstones so every other device erases too (ADR-0017).
func (a *App) pushErasures(ctx context.Context, store *storage.Store, client desktopBackendClient) (int, error) {
	ids, err := store.PendingSyncErasures(ctx)
	if err != nil {
		return 0, err
	}
	confirmed := 0
	const batchSize = 100
	for start := 0; start < len(ids); start += batchSize {
		end := min(start+batchSize, len(ids))
		batch := ids[start:end]
		var response syncEraseResponse
		if err := client.postJSON(ctx, "/v1/sync/erase", syncEraseRequest{SchemaVersion: "v1", RecordIDs: batch}, &response); err != nil {
			return confirmed, err
		}
		if response.SchemaVersion != "v1" || response.Cursor < 0 || response.Erased < 0 || response.Tombstones < 0 {
			return confirmed, errors.New("server returned an invalid erasure acknowledgment; deletion markers were retained")
		}
		if err := store.ClearSyncErasures(ctx, batch); err != nil {
			return confirmed, err
		}
		confirmed += len(batch)
	}
	return confirmed, nil
}

// pushMedicationRecords sends medication definitions, doses and dose
// corrections, definitions first (ADR-0048).
func (a *App) pushMedicationRecords(ctx context.Context, store *storage.Store, client desktopBackendClient) (int, error) {
	return pushPendingSyncRecords(ctx, client, storage.MaxMedicationSyncPageSize,
		store.PendingMedicationSyncRecords, store.MarkMedicationSyncRecordsPushed,
		func(record storage.MedicationSyncRecord) syncPushRecord {
			return syncPushRecord{RecordID: record.RecordID, Kind: record.Kind, CreatedAt: record.CreatedAt.UTC(), Payload: record.Payload}
		})
}

// pushMarkerRecords sends rhythm context markers (ADR-0048).
func (a *App) pushMarkerRecords(ctx context.Context, store *storage.Store, client desktopBackendClient) (int, error) {
	return pushPendingSyncRecords(ctx, client, storage.MaxMarkerSyncPageSize,
		store.PendingMarkerSyncRecords, store.MarkMarkerSyncRecordsPushed,
		func(record storage.MarkerSyncRecord) syncPushRecord {
			return syncPushRecord{RecordID: record.RecordID, Kind: storage.SyncKindContextMarker, CreatedAt: record.CreatedAt.UTC(), Payload: record.Payload}
		})
}

func (a *App) pushSleepRecords(ctx context.Context, store *storage.Store, client desktopBackendClient) (int, error) {
	return pushPendingSyncRecords(ctx, client, storage.MaxSleepSyncPageSize,
		store.PendingSleepSyncRecords, store.MarkSleepSyncRecordsPushed,
		func(record storage.SleepSyncRecord) syncPushRecord {
			return syncPushRecord{RecordID: record.RecordID, Kind: record.Kind, CreatedAt: record.CreatedAt.UTC(), Payload: record.Payload}
		})
}

// Every upload shares the same request bounds, acknowledgment checks and
// durable commit order. Their stores retain ownership of payload encoding
// and bookkeeping; no domain records are generalized into untyped maps.
func pushPendingSyncRecords[T any](ctx context.Context, client desktopBackendClient, pageLimit int,
	pending func(context.Context, int) ([]T, error),
	markPushed func(context.Context, []T, time.Time) error,
	encode func(T) syncPushRecord,
) (int, error) {
	totalPushed := 0
	for {
		records, err := pending(ctx, pageLimit)
		if err != nil {
			return totalPushed, err
		}
		if len(records) == 0 {
			return totalPushed, nil
		}
		pushRecords := make([]syncPushRecord, len(records))
		for i, record := range records {
			pushRecords[i] = encode(record)
		}
		for offset := 0; offset < len(records); {
			batchLength, err := nextSyncPushBatchLength(pushRecords[offset:])
			if err != nil {
				return totalPushed, err
			}
			end := offset + batchLength
			var response syncPushResponse
			if err := client.postJSON(ctx, "/v1/sync/push", syncPushRequest{SchemaVersion: "v1", Records: pushRecords[offset:end]}, &response); err != nil {
				return totalPushed, err
			}
			if err := validateSyncAcknowledgment(response, batchLength); err != nil {
				return totalPushed, err
			}
			if err := markPushed(ctx, records[offset:end], time.Now().UTC()); err != nil {
				return totalPushed, err
			}
			offset = end
			totalPushed += batchLength
		}
	}
}

func validateSyncAcknowledgment(response syncPushResponse, batchLength int) error {
	if response.SchemaVersion != "v1" || response.Cursor < 0 || response.Accepted < 0 || response.Accepted > batchLength {
		return errors.New("server returned an invalid sync acknowledgment; pending records were retained")
	}
	return nil
}

func nextSyncPushBatchLength(records []syncPushRecord) (int, error) {
	limit := min(len(records), maxSyncPushRecords)
	for end := 1; end <= limit; end++ {
		data, err := json.Marshal(syncPushRequest{SchemaVersion: "v1", Records: records[:end]})
		if err != nil {
			return 0, err
		}
		if len(data) <= maxSyncPushRequestBytes {
			continue
		}
		if end == 1 {
			return 0, fmt.Errorf(
				"sync record %q cannot be pushed: encoded request body is %d bytes; maximum is %d bytes",
				records[0].RecordID,
				len(data),
				maxSyncPushRequestBytes,
			)
		}
		return end - 1, nil
	}
	return limit, nil
}

var errSyncMorePages = errors.New("More records are waiting. Sync will continue on the next attempt.")

func (a *App) pullRecords(ctx context.Context, store *storage.Store, client desktopBackendClient) (int, int, int, error) {
	total := storage.SyncPullPageResult{}
	for page := 0; page < 4; page++ {
		result, complete, err := a.pullPage(ctx, store, client)
		total.Applied += result.Applied
		total.Skipped += result.Skipped
		total.TombstonesApplied += result.TombstonesApplied
		if err != nil {
			return total.Applied, total.Skipped, total.TombstonesApplied, err
		}
		if complete {
			return total.Applied, total.Skipped, total.TombstonesApplied, nil
		}
	}
	return total.Applied, total.Skipped, total.TombstonesApplied, errSyncMorePages
}

func (a *App) pullPage(ctx context.Context, store *storage.Store, client desktopBackendClient) (storage.SyncPullPageResult, bool, error) {
	cursor, err := store.SleepSyncCursor(ctx)
	if err != nil {
		return storage.SyncPullPageResult{}, false, err
	}
	var response syncPullResponse
	if err := client.getJSON(ctx, fmt.Sprintf("/v1/sync/pull?since=%d", cursor), &response); err != nil {
		var httpErr backendHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict {
			err = errors.New("The server history changed after a restore. Re-enroll in Settings to download it again; saved local records will be reconciled.")
		}
		return storage.SyncPullPageResult{}, false, err
	}
	if response.SchemaVersion != "v1" {
		return storage.SyncPullPageResult{}, false, errors.New("server returned an unsupported sync version; no records were applied")
	}
	if err := validatePullEnvelopePage(response, cursor); err != nil {
		return storage.SyncPullPageResult{}, false, err
	}
	records := make([]storage.SyncPullRecord, 0, len(response.Records))
	for _, record := range response.Records {
		decoded, err := decodePulledRecord(record)
		if err != nil {
			return storage.SyncPullPageResult{}, false, err
		}
		records = append(records, decoded)
	}
	result, err := store.ApplySyncPullPage(ctx, storage.SyncPullPage{
		Cursor:  response.Cursor,
		Records: records,
	})
	if err != nil {
		return result, false, err
	}
	return result, len(response.Records) < storage.MaxSyncPullPageSize, nil
}

// decodePulledRecord turns one downloaded envelope into the store's typed
// record. The store validates and applies it; tombstones apply last, so erasure
// wins over the record it erases even within one page.
func decodePulledRecord(record syncEnvelope) (storage.SyncPullRecord, error) {
	decode := func(target any) error { return json.Unmarshal(record.Payload, target) }
	switch record.Kind {
	case storage.SleepSyncKindObservation:
		var value storage.SyncPullObservation
		return value, decode(&value.Observation)
	case storage.SleepSyncKindCorrection:
		var value storage.SyncPullCorrection
		return value, decode(&value.Correction)
	case "task":
		var value storage.SyncPullTask
		return value, decode(&value.Task)
	case "placement":
		// This computer authored it; pulling it back confirms the upload.
		return storage.SyncPullPlacement{PlacementID: record.RecordID}, nil
	case storage.SyncKindMedication:
		var value storage.SyncPullMedication
		return value, decode(&value.Medication)
	case storage.SyncKindMedicationEvent:
		var value storage.SyncPullMedicationEvent
		return value, decode(&value.Event)
	case storage.SyncKindMedicationCorrection:
		var value storage.SyncPullMedicationCorrection
		return value, decode(&value.Correction)
	case storage.SyncKindContextMarker:
		var value storage.SyncPullMarker
		return value, decode(&value.Marker)
	case "tombstone":
		var payload syncTombstonePayload
		if err := decode(&payload); err != nil {
			return nil, err
		}
		id := payload.RecordID
		if id == "" {
			id = record.RecordID
		}
		return storage.SyncPullTombstone{RecordID: id, RecordKind: payload.RecordKind}, nil
	default:
		return nil, fmt.Errorf("unsupported synced record kind %q", record.Kind)
	}
}

func (a *App) serverOverview(ctx context.Context, now time.Time) (OverviewDTO, bool) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return OverviewDTO{}, false
	}
	var response serverOverviewResponse
	if err := a.newPassiveBackendClient(cfg, token).getJSON(ctx, "/v1/overview", &response); err != nil {
		a.recordBackendSyncError(cfg, err)
		return OverviewDTO{}, false
	}
	return overviewDTOFromServer(response, now), true
}

func (a *App) serverRhythm(ctx context.Context, now time.Time) (estimation.RhythmProjection, bool) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return estimation.RhythmProjection{}, false
	}
	var response serverRhythmResponse
	if err := a.newPassiveBackendClient(cfg, token).getJSON(ctx, "/v1/rhythm", &response); err != nil {
		a.recordBackendSyncError(cfg, err)
		return estimation.RhythmProjection{}, false
	}
	if response.Projection != nil {
		projection := *response.Projection
		projection.FixtureMode = false
		projection.EstimateSource = "synced"
		return projection, true
	}
	message := "The synced server estimate is waiting for enough sleep data."
	if response.Refusal != nil {
		message = response.Refusal.Message
	}
	localNow := now.In(locationOrUTC(localZoneID()))
	return estimation.RhythmProjection{
		FixtureMode:     false,
		EstimateSource:  "synced",
		Status:          fallbackStatus(response.Status),
		Refusal:         response.Refusal,
		ActogramSummary: message,
		ObservedRows:    []estimation.RhythmBand{},
		ForecastRows:    []estimation.RhythmBand{},
		Now: estimation.RhythmNow{
			Label:     "now",
			Day:       localNow.Format("Jan 2"),
			CivilDate: localNow.Format("2006-01-02"),
			ZoneID:    localZoneID(),
			Hour:      localClockHour(localNow),
		},
		DriftTitle:      "Sleep-onset drift",
		SlopeLabel:      "Not enough data",
		DriftConfidence: "Low",
		DriftSummary:    message,
		YMinHour:        0,
		YMaxHour:        24,
		DriftPoints:     []estimation.RhythmDriftPoint{},
	}, true
}

func overviewDTOFromServer(response serverOverviewResponse, now time.Time) OverviewDTO {
	status := fallbackStatus(response.Status)
	if status == "estimated" {
		return OverviewDTO{
			EstimateSource:           "synced",
			Status:                   "estimated",
			CurrentEstimatedState:    response.CurrentEstimatedState,
			TimeSinceWake:            response.TimeSinceWake,
			PredictedNextSleepWindow: response.PredictedNextSleepWindow,
			DriftEstimate:            response.DriftEstimate,
			Confidence:               response.Confidence,
			ConfidenceReasons:        nonEmptyStrings(response.ConfidenceReasons, "Synced server estimate"),
			NextUsefulTaskWindow:     response.NextUsefulTaskWindow,
			SharingStatus:            response.SharingStatus,
			MedicationEvents:         response.MedicationEvents,
			FixtureMode:              false,
			Disclaimer:               nonEmpty(response.Disclaimer, disclaimer),
			UpdatedLabel:             "Synced - server estimate just now",
		}
	}
	message := "The synced server estimate is waiting for enough sleep data."
	if response.Refusal != nil && response.Refusal.Message != "" {
		message = response.Refusal.Message
	}
	return OverviewDTO{
		EstimateSource:           "synced",
		Status:                   status,
		Empty:                    status == "empty",
		Refusal:                  response.Refusal,
		CurrentEstimatedState:    "Synced server estimate unavailable",
		TimeSinceWake:            "Not available",
		PredictedNextSleepWindow: "Not enough synced data",
		DriftEstimate:            "Not enough synced data",
		Confidence:               "low",
		ConfidenceReasons:        []string{message},
		NextUsefulTaskWindow:     "No reliable proposal",
		SharingStatus:            "Server projection only; trusted sharing remains default-deny.",
		MedicationEvents:         []MedicationEventDTO{},
		FixtureMode:              false,
		Disclaimer:               nonEmpty(response.Disclaimer, disclaimer),
		UpdatedLabel:             now.Local().Format("Synced Jan 2, 3:04 PM"),
	}
}

// --- Synced proposals (approvals unification, ADR-0016) ---

type backendProposalRecord struct {
	ProposalID    string          `json:"proposalId"`
	ActionID      string          `json:"actionId"`
	DeviceID      string          `json:"deviceId"`
	Status        string          `json:"status"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
	ExpiresAt     time.Time       `json:"expiresAt"`
	Payload       json.RawMessage `json:"payload"`
	DecisionToken string          `json:"decisionToken"`
}

type backendProposalPagination struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor"`
}

type backendProposalListResponse struct {
	PendingCount  int                       `json:"pendingCount"`
	NextExpiryAt  string                    `json:"nextExpiryAt"`
	SchemaVersion string                    `json:"schema_version"`
	Proposals     []backendProposalRecord   `json:"proposals"`
	Pagination    backendProposalPagination `json:"pagination"`
}

type backendProposalPayload struct {
	ScheduleProposals struct {
		Proposals []struct {
			TaskID     string    `json:"task_id"`
			StartAt    time.Time `json:"start_at"`
			EndAt      time.Time `json:"end_at"`
			ZoneID     string    `json:"zone_id"`
			Confidence struct {
				Level   string   `json:"level"`
				Reasons []string `json:"reasons"`
			} `json:"confidence"`
			ExplanationCodes []string `json:"explanation_codes"`
		} `json:"proposals"`
	} `json:"schedule_proposals"`
	Answer string `json:"answer"`
}

type BackendProposalDTO struct {
	ExpiresAt     string   `json:"expiresAt"`
	ProposalID    string   `json:"proposalId"`
	Action        string   `json:"action"`
	Status        string   `json:"status"`
	Title         string   `json:"title"`
	Window        string   `json:"window"`
	Confidence    string   `json:"confidence"`
	ReasonLabels  []string `json:"reasonLabels"`
	Answer        string   `json:"answer,omitempty"`
	CreatedLabel  string   `json:"createdLabel"`
	ExpiresLabel  string   `json:"expiresLabel"`
	DecisionToken string   `json:"decisionToken,omitempty"`
}

type BackendProposalPaginationDTO struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor"`
}

type BackendProposalsDTO struct {
	DecisionRecorded bool                         `json:"decisionRecorded,omitempty"`
	PendingCount     int                          `json:"pendingCount"`
	NextExpiryAt     string                       `json:"nextExpiryAt"`
	Status           string                       `json:"status"`
	Message          string                       `json:"message,omitempty"`
	Proposals        []BackendProposalDTO         `json:"proposals"`
	Pagination       BackendProposalPaginationDTO `json:"pagination"`
}
type BackendProposalPageInput struct {
	Cursor string `json:"cursor"`
}

type BackendProposalDecisionInput struct {
	ProposalID string `json:"proposalId"`
	Decision   string `json:"decision"`
	Token      string `json:"token"`
}

// GetBackendProposals lists the synced backend's proposals (assistant/agent
// origins) with their one-use decision tokens so this device can decide them.
// When sync is off it reports status "off" without touching the network.
func (a *App) GetBackendProposals() (BackendProposalsDTO, error) {
	return a.getBackendProposalPage("")
}

// GetBackendProposalPage returns the next opaque-cursor page without making
// the frontend retain or reinterpret server row identifiers.
func (a *App) GetBackendProposalPage(input BackendProposalPageInput) (BackendProposalsDTO, error) {
	cursor := strings.TrimSpace(input.Cursor)
	if cursor == "" {
		return BackendProposalsDTO{}, errors.New("proposal cursor is required")
	}
	if len(cursor) > 512 {
		return BackendProposalsDTO{}, errors.New("proposal cursor is too long")
	}
	return a.getBackendProposalPage(cursor)
}

func (a *App) getBackendProposalPage(cursor string) (BackendProposalsDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		if !cfg.Enabled {
			return BackendProposalsDTO{Status: "off", Proposals: []BackendProposalDTO{}}, nil
		}
		return BackendProposalsDTO{Status: "error", Message: sanitizeBackendError(err), Proposals: []BackendProposalDTO{}}, nil
	}
	return a.fetchBackendProposals(a.applicationContext(), cfg, token, cursor), nil
}

// DecideBackendProposal approves or rejects a pending synced proposal via the
// backend decision endpoint, consuming the one-use token, then returns the
// refreshed list. Nothing is applied locally; the backend records the decision.
func (a *App) DecideBackendProposal(input BackendProposalDecisionInput) (BackendProposalsDTO, error) {
	if input.Decision != "approved" && input.Decision != "rejected" {
		return BackendProposalsDTO{}, errors.New("decision must be approved or rejected")
	}
	if input.ProposalID == "" || input.Token == "" {
		return BackendProposalsDTO{}, errors.New("proposal id and decision token are required")
	}
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return BackendProposalsDTO{Status: "error", Message: sanitizeBackendError(err), Proposals: []BackendProposalDTO{}}, nil
	}
	client := a.newDesktopBackendClient(cfg, token)
	payload := map[string]string{"decision": input.Decision, "token": input.Token}
	var decided map[string]json.RawMessage
	ctx := a.applicationContext()
	if err := client.postJSON(ctx, "/v1/proposals/"+url.PathEscape(input.ProposalID)+"/decision", payload, &decided); err != nil {
		result := a.fetchBackendProposals(ctx, cfg, token, "")
		result.Status = "error"
		result.Message = sanitizeBackendError(err)
		return result, nil
	}
	result := a.fetchBackendProposals(ctx, cfg, token, "")
	result.DecisionRecorded = true
	if result.Status != "ok" {
		result.Status = "error"
		result.Message = "Decision recorded. The queue could not be refreshed; refresh it before another decision."
	}
	return result, nil
}

func (a *App) fetchBackendProposals(ctx context.Context, cfg backendSyncConfig, token, cursor string) BackendProposalsDTO {
	// The first page loads on its own; an older page is asked for by name.
	client := a.newDesktopBackendClient(cfg, token)
	if cursor == "" {
		client = a.newPassiveBackendClient(cfg, token)
	}
	path := "/v1/proposals"
	if cursor != "" {
		path += "?" + url.Values{"cursor": []string{cursor}}.Encode()
	}
	var response backendProposalListResponse
	if err := client.getJSON(ctx, path, &response); err != nil {
		a.recordBackendSyncError(cfg, err)
		return BackendProposalsDTO{Status: "error", Message: sanitizeBackendError(err), Proposals: []BackendProposalDTO{}}
	}
	proposals := make([]BackendProposalDTO, 0, len(response.Proposals))
	for _, record := range response.Proposals {
		if record.ActionID == visitorRequestActionID {
			// Visitor requests have their own surface. The generic decision
			// route refuses them on purpose — approving one means choosing an
			// exact block — so listing them here would offer a control that
			// cannot work.
			continue
		}
		proposals = append(proposals, backendProposalDTO(record))
	}
	return BackendProposalsDTO{
		Status:       "ok",
		PendingCount: response.PendingCount,
		NextExpiryAt: response.NextExpiryAt,
		Proposals:    proposals,
		Pagination: BackendProposalPaginationDTO{
			Limit:      response.Pagination.Limit,
			HasMore:    response.Pagination.HasMore,
			NextCursor: response.Pagination.NextCursor,
		},
	}
}

func backendProposalDTO(record backendProposalRecord) BackendProposalDTO {
	dto := BackendProposalDTO{
		ExpiresAt:     record.ExpiresAt.UTC().Format(time.RFC3339Nano),
		ProposalID:    record.ProposalID,
		Action:        record.ActionID,
		Status:        record.Status,
		Title:         backendProposalTitle(record.ActionID, ""),
		Window:        "Window unavailable",
		Confidence:    "Low",
		ReasonLabels:  []string{},
		CreatedLabel:  "Proposed " + record.CreatedAt.Local().Format("Jan 2, 3:04 PM"),
		ExpiresLabel:  "expires " + record.ExpiresAt.Local().Format("Jan 2, 3:04 PM"),
		DecisionToken: record.DecisionToken,
	}
	var payload backendProposalPayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return dto
	}
	dto.Answer = payload.Answer
	if len(payload.ScheduleProposals.Proposals) == 0 {
		return dto
	}
	item := payload.ScheduleProposals.Proposals[0]
	dto.Title = backendProposalTitle(record.ActionID, item.TaskID)
	dto.Window = civilWindow(item.StartAt, item.EndAt, item.ZoneID)
	dto.Confidence = titleConfidence(item.Confidence.Level)
	dto.ReasonLabels = reasonLabels(item.ExplanationCodes)
	return dto
}

func backendProposalTitle(action, taskID string) string {
	verb := agentactions.CardTitle(action)
	if taskID == "" {
		return verb
	}
	return verb + " “" + taskID + "”"
}

func civilWindow(start, end time.Time, zoneID string) string {
	location := time.Local
	if zoneID != "" {
		if loaded, err := time.LoadLocation(zoneID); err == nil {
			location = loaded
		}
	}
	return start.In(location).Format("Mon Jan 2, 3:04 PM") + " to " + end.In(location).Format("3:04 PM MST")
}

func titleConfidence(level string) string {
	switch strings.ToLower(level) {
	case "high":
		return "High"
	case "medium", "moderate":
		return "Medium"
	default:
		return "Low"
	}
}

func (a *App) requireBackendSync() (backendSyncConfig, string, error) {
	a.backendConfigMu.RLock()
	defer a.backendConfigMu.RUnlock()
	store, err := a.requireStore()
	if err != nil {
		return backendSyncConfig{}, "", err
	}
	cfg, token, err := store.LoadSyncConnection(a.applicationContext())
	if err != nil {
		return cfg, "", err
	}
	if !cfg.Enabled {
		return cfg, "", errors.New("backend sync is off")
	}
	if cfg.DeviceID == "" {
		return cfg, "", errors.New("backend device is not enrolled")
	}
	baseURL, err := normalizeBackendURL(cfg.BackendURL)
	if err != nil {
		return cfg, "", err
	}
	if err := validateBackendTLS(baseURL, cfg.InsecureSkipVerify); err != nil {
		return cfg, "", err
	}
	if token == "" {
		return cfg, "", errors.New("backend device token is not stored")
	}
	return cfg, token, nil
}

func (a *App) backendSyncStatusCounts(cfg backendSyncConfig, counts syncCounts) BackendSyncStatusDTO {
	status := "off"
	if cfg.Enabled {
		status = "connected"
		if cfg.LastError != "" {
			status = "error"
		}
	}
	pending := 0
	pendingErasures, waitingRecords, taskConflicts := 0, 0, 0
	cursor := int64(0)
	if store, err := a.requireStore(); err == nil {
		ctx := a.applicationContext()
		for _, count := range []func(context.Context) (int, error){
			store.PendingSleepSyncRecordCount, store.PendingTaskSyncRecordCount, store.PendingPlacementSyncRecordCount,
			store.PendingMedicationSyncRecordCount, store.PendingMarkerSyncRecordCount,
		} {
			if value, err := count(ctx); err == nil {
				pending += value
			}
		}
		if value, err := store.SleepSyncCursor(ctx); err == nil {
			cursor = value
		}
		pendingErasures, _ = store.PendingSyncErasureCount(ctx)
		waitingRecords, _ = store.DeferredSyncRecordCount(ctx)
		taskConflicts, _ = store.TaskSyncConflictCount(ctx)
	}
	return BackendSyncStatusDTO{
		Enabled:             cfg.Enabled,
		Status:              status,
		BackendURL:          cfg.BackendURL,
		DeviceID:            cfg.DeviceID,
		InsecureSkipVerify:  cfg.InsecureSkipVerify,
		LastSyncLabel:       lastSyncLabel(cfg.LastSyncAt),
		LastError:           cfg.LastError,
		PendingPushCount:    pending,
		PendingErasureCount: pendingErasures,
		WaitingRecordCount:  waitingRecords,
		TaskConflictCount:   taskConflicts,
		PushedCount:         counts.pushed,
		PulledCount:         counts.pulled,
		SkippedCount:        counts.skipped,
		ErasuresPushed:      counts.erasuresPushed,
		TombstonesApplied:   counts.tombstonesApplied,
		Cursor:              cursor,
	}
}

func (a *App) loadBackendSyncConfig() (backendSyncConfig, error) {
	store, err := a.requireStore()
	if err != nil {
		return backendSyncConfig{}, err
	}
	cfg, _, err := store.LoadSyncConnection(a.applicationContext())
	return cfg, err
}

func (a *App) saveBackendSyncConfig(cfg backendSyncConfig) error {
	store, err := a.requireStore()
	if err != nil {
		return err
	}
	return store.UpdateSyncConnection(a.applicationContext(), cfg)
}

func desktopDataDir() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("ZEITBOARD_DATA_DIR")); configured != "" {
		if !filepath.IsAbs(configured) {
			return "", errors.New("ZEITBOARD_DATA_DIR must be an absolute path")
		}
		return filepath.Clean(configured), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ZeitBoard"), nil
}

// ensurePrivateDir creates the directory and restricts it to this user. The
// restriction is inherited by files created inside it, so a writer that has not
// been taught about privatefile still does not leave a readable file behind.
func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return privatefile.RestrictDir(dir)
}

func (a *App) backendHTTPClient(insecureSkipVerify bool) *http.Client {
	a.backendHTTPMu.Lock()
	defer a.backendHTTPMu.Unlock()
	if a.backendHTTPClients == nil {
		a.backendHTTPClients = make(map[bool]*http.Client, 2)
	}
	client := a.backendHTTPClients[insecureSkipVerify]
	if client == nil {
		client = newBackendHTTPClient(insecureSkipVerify)
		a.backendHTTPClients[insecureSkipVerify] = client
	}
	return client
}

func (a *App) closeBackendHTTPClients() {
	a.backendHTTPMu.Lock()
	clients := a.backendHTTPClients
	a.backendHTTPClients = nil
	a.backendHTTPMu.Unlock()

	for _, client := range clients {
		if client != nil {
			client.CloseIdleConnections()
		}
	}
}

func (a *App) newDesktopBackendClient(cfg backendSyncConfig, token string) desktopBackendClient {
	return desktopBackendClient{
		baseURL: strings.TrimRight(cfg.BackendURL, "/"),
		token:   token,
		client:  a.backendHTTPClient(cfg.InsecureSkipVerify),
		outage:  &a.backendOutage,
		now:     a.currentTime,
	}
}

// newPassiveBackendClient is for reads that happen on their own: Home's
// overview, Rhythm, the decision queue's synced sources, the list of share
// links. While the server is known to be down they give up at once instead of
// waiting out the request timeout. What the owner does, and sync, always try.
func (a *App) newPassiveBackendClient(cfg backendSyncConfig, token string) desktopBackendClient {
	client := a.newDesktopBackendClient(cfg, token)
	client.passive = true
	return client
}

func (c desktopBackendClient) getJSON(ctx context.Context, path string, target any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, target)
}

func (c desktopBackendClient) postJSON(ctx context.Context, path string, payload any, target any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.doJSON(ctx, http.MethodPost, path, data, target)
}

func (c desktopBackendClient) doJSON(ctx context.Context, method, path string, payload []byte, target any) error {
	if c.baseURL == "" {
		return errors.New("backend URL is not configured")
	}
	if c.passive && c.outage.active(c.now()) {
		return errBackendUnreachable
	}
	var body io.Reader
	if len(payload) > 0 {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			// Given up on, not unanswered.
			return ctx.Err()
		}
		c.outage.start(c.now())
		return errBackendUnreachable
	}
	defer resp.Body.Close()
	// Any answer, even an error status, means the server is there.
	c.outage.clear()
	const responseLimit = 2 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	if err != nil {
		return err
	}
	if len(data) > responseLimit {
		return errors.New("backend response exceeded the supported size")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return backendHTTPError{StatusCode: resp.StatusCode, Message: sanitizeHTTPBody(data)}
	}
	if target == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return errors.New("backend returned an invalid JSON response")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("backend returned more than one JSON value")
	}
	return nil
}

func normalizeBackendURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("backend URL must be an https URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("backend URL must not contain credentials, a query or a fragment")
	}
	parsed.Host = strings.ToLower(parsed.Host)
	value = parsed.String()
	return value, nil
}

func validateBackendTLS(baseURL string, insecureSkipVerify bool) error {
	if !insecureSkipVerify {
		return nil
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return nil
	default:
		return errors.New("self-signed TLS skip verify is only allowed for localhost development")
	}
}

func (a *App) recordBackendSyncError(cfg backendSyncConfig, err error) {
	a.backendConfigMu.Lock()
	defer a.backendConfigMu.Unlock()
	current, loadErr := a.loadBackendSyncConfig()
	if loadErr != nil || !current.Enabled || current.DeviceID != cfg.DeviceID || current.BackendURL != cfg.BackendURL {
		return
	}
	current.LastError = sanitizeBackendError(err)
	_ = a.saveBackendSyncConfig(current)
}

func sanitizeBackendError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 180 {
		message = message[:180]
	}
	return message
}

func sanitizeHTTPBody(data []byte) string {
	message := strings.TrimSpace(string(data))
	if len(message) > 160 {
		message = message[:160]
	}
	if message == "" {
		return "request failed"
	}
	return message
}

func fallbackStatus(value string) string {
	switch value {
	case "empty", "refused", "unavailable", "estimated":
		return value
	default:
		return "unavailable"
	}
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func nonEmptyStrings(values []string, fallback string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return []string{fallback}
	}
	return result
}

func lastSyncLabel(value time.Time) string {
	if value.IsZero() {
		return "Not synced yet"
	}
	return value.Local().Format("Last synced Jan 2, 3:04 PM")
}

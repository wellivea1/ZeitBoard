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

	"non24.app/core/estimation"
	storage "non24.app/core/storage/sqlite"
)

const (
	backendSyncConfigFile = "backend-sync.json"
	backendSyncTokenFile  = "backend-sync-token"
	backendRequestTimeout = 10 * time.Second
)

var newBackendHTTPClient = func(insecureSkipVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // Explicit localhost/self-hosted dev escape hatch.
	}
	return &http.Client{Timeout: backendRequestTimeout, Transport: transport}
}

type BackendSyncInput struct {
	Enabled            bool   `json:"enabled"`
	BackendURL         string `json:"backendUrl"`
	EnrollmentSecret   string `json:"enrollmentSecret"`
	DeviceLabel        string `json:"deviceLabel"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
}

type BackendSyncStatusDTO struct {
	Enabled            bool   `json:"enabled"`
	Status             string `json:"status"`
	BackendURL         string `json:"backendUrl"`
	DeviceID           string `json:"deviceId"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	LastSyncLabel      string `json:"lastSyncLabel"`
	LastError          string `json:"lastError"`
	PendingPushCount   int    `json:"pendingPushCount"`
	PushedCount        int    `json:"pushedCount"`
	PulledCount        int    `json:"pulledCount"`
	Cursor             int64  `json:"cursor"`
}

type backendSyncConfig struct {
	Enabled            bool      `json:"enabled"`
	BackendURL         string    `json:"backendUrl"`
	DeviceID           string    `json:"deviceId"`
	InsecureSkipVerify bool      `json:"insecureSkipVerify,omitempty"`
	LastSyncAt         time.Time `json:"lastSyncAt,omitempty"`
	LastError          string    `json:"lastError,omitempty"`
}

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
}

func (a *App) GetBackendSyncStatus() (BackendSyncStatusDTO, error) {
	cfg, _ := a.loadBackendSyncConfig()
	return a.backendSyncStatus(cfg, 0, 0), nil
}

func (a *App) ConfigureBackendSync(input BackendSyncInput) (BackendSyncStatusDTO, error) {
	if !input.Enabled {
		return a.DisableBackendSync()
	}
	baseURL, err := normalizeBackendURL(input.BackendURL)
	if err != nil {
		return BackendSyncStatusDTO{}, err
	}
	if err := validateBackendTLS(baseURL, input.InsecureSkipVerify); err != nil {
		return BackendSyncStatusDTO{}, err
	}
	cfg := backendSyncConfig{
		Enabled:            false,
		BackendURL:         baseURL,
		InsecureSkipVerify: input.InsecureSkipVerify,
	}
	_ = a.deleteBackendSyncToken()
	label := strings.TrimSpace(input.DeviceLabel)
	if label == "" {
		label = "ZeitBoard desktop"
	}
	client := newDesktopBackendClient(cfg, "")
	var response registerDeviceResponse
	err = client.postJSON(context.Background(), "/v1/devices", registerDeviceRequest{
		EnrollmentSecret: input.EnrollmentSecret,
		Label:            label,
	}, &response)
	if err != nil {
		cfg.LastError = sanitizeBackendError(err)
		_ = a.saveBackendSyncConfig(cfg)
		return a.backendSyncStatus(cfg, 0, 0), errors.New(cfg.LastError)
	}
	if response.DeviceID == "" || response.Token == "" {
		cfg.LastError = "backend enrollment returned an invalid device credential"
		_ = a.saveBackendSyncConfig(cfg)
		return a.backendSyncStatus(cfg, 0, 0), errors.New(cfg.LastError)
	}
	if err := a.saveBackendSyncToken(response.Token); err != nil {
		cfg.LastError = "could not store backend device token"
		_ = a.saveBackendSyncConfig(cfg)
		return a.backendSyncStatus(cfg, 0, 0), errors.New(cfg.LastError)
	}
	cfg.Enabled = true
	cfg.DeviceID = response.DeviceID
	cfg.LastError = ""
	if err := a.saveBackendSyncConfig(cfg); err != nil {
		return BackendSyncStatusDTO{}, err
	}
	return a.backendSyncStatus(cfg, 0, 0), nil
}

func (a *App) DisableBackendSync() (BackendSyncStatusDTO, error) {
	cfg, _ := a.loadBackendSyncConfig()
	cfg.Enabled = false
	cfg.LastError = ""
	if err := a.saveBackendSyncConfig(cfg); err != nil {
		return BackendSyncStatusDTO{}, err
	}
	if err := a.deleteBackendSyncToken(); err != nil {
		return BackendSyncStatusDTO{}, err
	}
	return a.backendSyncStatus(cfg, 0, 0), nil
}

func (a *App) SyncNow() (BackendSyncStatusDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		if cfg.Enabled {
			cfg.LastError = sanitizeBackendError(err)
			_ = a.saveBackendSyncConfig(cfg)
		}
		return a.backendSyncStatus(cfg, 0, 0), nil
	}
	pushed, pulled, syncErr := a.syncSleepRecords(context.Background(), cfg, token)
	if syncErr != nil {
		cfg.LastError = sanitizeBackendError(syncErr)
		_ = a.saveBackendSyncConfig(cfg)
		return a.backendSyncStatus(cfg, pushed, pulled), nil
	}
	cfg.LastSyncAt = time.Now().UTC()
	cfg.LastError = ""
	if err := a.saveBackendSyncConfig(cfg); err != nil {
		return BackendSyncStatusDTO{}, err
	}
	return a.backendSyncStatus(cfg, pushed, pulled), nil
}

func (a *App) syncSleepRecords(ctx context.Context, cfg backendSyncConfig, token string) (int, int, error) {
	store, err := a.requireStore()
	if err != nil {
		return 0, 0, err
	}
	client := newDesktopBackendClient(cfg, token)
	pushed, err := a.pushSleepRecords(ctx, store, client)
	if err != nil {
		return pushed, 0, err
	}
	pulled, err := a.pullSleepRecords(ctx, store, client, cfg.DeviceID)
	return pushed, pulled, err
}

func (a *App) pushSleepRecords(ctx context.Context, store *storage.Store, client desktopBackendClient) (int, error) {
	records, err := store.UnpushedSleepSyncRecords(ctx)
	if err != nil {
		return 0, err
	}
	if len(records) == 0 {
		return 0, nil
	}
	pushRecords := make([]syncPushRecord, 0, len(records))
	for _, record := range records {
		pushRecords = append(pushRecords, syncPushRecord{
			RecordID:  record.RecordID,
			Kind:      record.Kind,
			CreatedAt: record.CreatedAt.UTC(),
			Payload:   record.Payload,
		})
	}
	var response syncPushResponse
	if err := client.postJSON(ctx, "/v1/sync/push", syncPushRequest{SchemaVersion: "v1", Records: pushRecords}, &response); err != nil {
		return 0, err
	}
	if err := store.MarkSleepSyncRecordsPushed(ctx, records, time.Now().UTC()); err != nil {
		return 0, err
	}
	return len(records), nil
}

func (a *App) pullSleepRecords(ctx context.Context, store *storage.Store, client desktopBackendClient, deviceID string) (int, error) {
	cursor, err := store.SleepSyncCursor(ctx)
	if err != nil {
		return 0, err
	}
	var response syncPullResponse
	if err := client.getJSON(ctx, fmt.Sprintf("/v1/sync/pull?since=%d", cursor), &response); err != nil {
		return 0, err
	}
	inserted := 0
	corrections := make([]storage.SleepCorrectionRecord, 0)
	for _, record := range response.Records {
		if record.DeviceID == deviceID {
			continue
		}
		switch record.Kind {
		case storage.SleepSyncKindObservation:
			var observation storage.SleepObservationRecord
			if err := json.Unmarshal(record.Payload, &observation); err != nil {
				return inserted, err
			}
			ok, err := store.InsertSyncedSleepObservation(ctx, observation)
			if err != nil {
				return inserted, err
			}
			if ok {
				inserted++
			}
		case storage.SleepSyncKindCorrection:
			var correction storage.SleepCorrectionRecord
			if err := json.Unmarshal(record.Payload, &correction); err != nil {
				return inserted, err
			}
			corrections = append(corrections, correction)
		default:
			return inserted, fmt.Errorf("unsupported synced record kind %q", record.Kind)
		}
	}
	for _, correction := range corrections {
		ok, err := store.InsertSyncedSleepCorrection(ctx, correction)
		if err != nil {
			return inserted, err
		}
		if ok {
			inserted++
		}
	}
	if err := store.SaveSleepSyncCursor(ctx, response.Cursor); err != nil {
		return inserted, err
	}
	return inserted, nil
}

func (a *App) serverOverview(ctx context.Context, now time.Time) (OverviewDTO, bool) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return OverviewDTO{}, false
	}
	var response serverOverviewResponse
	if err := newDesktopBackendClient(cfg, token).getJSON(ctx, "/v1/overview", &response); err != nil {
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
	if err := newDesktopBackendClient(cfg, token).getJSON(ctx, "/v1/rhythm", &response); err != nil {
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
	return estimation.RhythmProjection{
		FixtureMode:     false,
		EstimateSource:  "synced",
		Status:          fallbackStatus(response.Status),
		Refusal:         response.Refusal,
		ActogramSummary: message,
		ObservedRows:    []estimation.RhythmBand{},
		ForecastRows:    []estimation.RhythmBand{},
		Now:             estimation.RhythmNow{Label: "now", Day: now.Local().Format("Jan 2"), Hour: localClockHour(now.Local())},
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

func (a *App) requireBackendSync() (backendSyncConfig, string, error) {
	cfg, err := a.loadBackendSyncConfig()
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
	token, err := a.loadBackendSyncToken()
	if err != nil {
		return cfg, "", err
	}
	if token == "" {
		return cfg, "", errors.New("backend device token is not stored")
	}
	return cfg, token, nil
}

func (a *App) backendSyncStatus(cfg backendSyncConfig, pushed, pulled int) BackendSyncStatusDTO {
	status := "off"
	if cfg.Enabled {
		status = "connected"
		if cfg.LastError != "" {
			status = "error"
		}
	}
	pending := 0
	cursor := int64(0)
	if store, err := a.requireStore(); err == nil {
		if records, err := store.UnpushedSleepSyncRecords(context.Background()); err == nil {
			pending = len(records)
		}
		if value, err := store.SleepSyncCursor(context.Background()); err == nil {
			cursor = value
		}
	}
	return BackendSyncStatusDTO{
		Enabled:            cfg.Enabled,
		Status:             status,
		BackendURL:         cfg.BackendURL,
		DeviceID:           cfg.DeviceID,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		LastSyncLabel:      lastSyncLabel(cfg.LastSyncAt),
		LastError:          cfg.LastError,
		PendingPushCount:   pending,
		PushedCount:        pushed,
		PulledCount:        pulled,
		Cursor:             cursor,
	}
}

func (a *App) loadBackendSyncConfig() (backendSyncConfig, error) {
	dir, err := a.syncConfigDir()
	if err != nil {
		return backendSyncConfig{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, backendSyncConfigFile))
	if errors.Is(err, os.ErrNotExist) {
		return backendSyncConfig{}, nil
	}
	if err != nil {
		return backendSyncConfig{}, err
	}
	var cfg backendSyncConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return backendSyncConfig{}, err
	}
	return cfg, nil
}

func (a *App) saveBackendSyncConfig(cfg backendSyncConfig) error {
	dir, err := a.syncConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, backendSyncConfigFile), data, 0o600)
}

func (a *App) saveBackendSyncToken(token string) error {
	dir, err := a.syncConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, backendSyncTokenFile), []byte(token), 0o600)
}

func (a *App) loadBackendSyncToken() (string, error) {
	dir, err := a.syncConfigDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, backendSyncTokenFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (a *App) deleteBackendSyncToken() error {
	dir, err := a.syncConfigDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, backendSyncTokenFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (a *App) syncConfigDir() (string, error) {
	if a.configDir != "" {
		return a.configDir, nil
	}
	return desktopDataDir()
}

func desktopDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ZeitBoard"), nil
}

func newDesktopBackendClient(cfg backendSyncConfig, token string) desktopBackendClient {
	return desktopBackendClient{
		baseURL: strings.TrimRight(cfg.BackendURL, "/"),
		token:   token,
		client:  newBackendHTTPClient(cfg.InsecureSkipVerify),
	}
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
		return errors.New("backend request failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return err
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
	return nil
}

func normalizeBackendURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("backend URL must be an https URL")
	}
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
	cfg.LastError = sanitizeBackendError(err)
	_ = a.saveBackendSyncConfig(cfg)
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

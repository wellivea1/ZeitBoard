// Package fixtures deterministically generates the synthetic phase-one
// contract fixtures. It is the Go replacement for the former
// scripts/generate-testdata.py and produces byte-identical output, so the
// checked-in testdata/v1 files remain the source of truth.
package fixtures

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

var (
	zoneID      = "America/New_York"
	baseStart   = time.Date(2026, 3, 5, 4, 30, 0, 0, time.UTC)
	generatedAt = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	cycle       = 24*time.Hour + 50*time.Minute
	durations   = []int{480, 470, 490, 475, 485, 480, 465, 495, 480, 475}
	notice      = "Estimated windows are uncertain and are not medical advice."
)

// ManifestEntry is the authoritative mapping from a generated fixture to its
// contract version and validating schema.
type ManifestEntry struct {
	Version string
	Name    string
	Schema  string
}

// GeneratedPath returns the repository-relative path owned by this entry.
func (entry ManifestEntry) GeneratedPath() string {
	return path.Join("testdata", entry.Version, entry.Name)
}

// File is a generated fixture and its manifest metadata.
type File struct {
	ManifestEntry
	Data []byte
}

type fixtureID string

type fixtureSpec struct {
	id fixtureID
	ManifestEntry
}

var fixtureManifest = []fixtureSpec{
	{"v1/observations", ManifestEntry{Version: "v1", Name: "observations.json", Schema: "observation-set.schema.json"}},
	{"v1/corrections", ManifestEntry{Version: "v1", Name: "corrections.json", Schema: "correction-set.schema.json"}},
	{"v1/sleep-data-export", ManifestEntry{Version: "v1", Name: "sleep-data-export.json", Schema: "sleep-data-export.schema.json"}},
	{"v1/sync-batch", ManifestEntry{Version: "v1", Name: "sync-batch.json", Schema: "sync-batch.schema.json"}},
	{"v1/sync-erase", ManifestEntry{Version: "v1", Name: "sync-erase.json", Schema: "sync-erase.schema.json"}},
	{"v1/task-set", ManifestEntry{Version: "v1", Name: "task-set.json", Schema: "task-set.schema.json"}},
	{"v1/calendar-event-set", ManifestEntry{Version: "v1", Name: "calendar-event-set.json", Schema: "calendar-event-set.schema.json"}},
	{"v1/medication-set", ManifestEntry{Version: "v1", Name: "medication-set.json", Schema: "medication-set.schema.json"}},
	{"v1/medication-event-set", ManifestEntry{Version: "v1", Name: "medication-event-set.json", Schema: "medication-event-set.schema.json"}},
	{"v1/medication-data-export", ManifestEntry{Version: "v1", Name: "medication-data-export.json", Schema: "medication-data-export.schema.json"}},
	{"v1/rhythm-marker-set", ManifestEntry{Version: "v1", Name: "rhythm-marker-set.json", Schema: "rhythm-marker-set.schema.json"}},
	{"v1/clinical-chart-request", ManifestEntry{Version: "v1", Name: "clinical-chart-request.json", Schema: "clinical-chart-request.schema.json"}},
	{"v1/assistant-action", ManifestEntry{Version: "v1", Name: "assistant-action.json", Schema: "assistant-action.schema.json"}},
	{"v1/direct-proposal-request", ManifestEntry{Version: "v1", Name: "direct-proposal-request.json", Schema: "direct-proposal-request.schema.json"}},
	{"v1/phase-estimate", ManifestEntry{Version: "v1", Name: "phase-estimate.json", Schema: "phase-estimate.schema.json"}},
	{"v1/phase-estimate-refused", ManifestEntry{Version: "v1", Name: "phase-estimate-refused.json", Schema: "phase-estimate.schema.json"}},
	{"v1/schedule-request", ManifestEntry{Version: "v1", Name: "schedule-request.json", Schema: "schedule-request.schema.json"}},
	{"v1/schedule-proposals", ManifestEntry{Version: "v1", Name: "schedule-proposals.json", Schema: "schedule-proposals.schema.json"}},
	{"v1/proposal-response", ManifestEntry{Version: "v1", Name: "proposal-response.json", Schema: "proposal-response.schema.json"}},
	{"v1/share-profile-default-deny", ManifestEntry{Version: "v1", Name: "share-profile-default-deny.json", Schema: "share-profile.schema.json"}},
	{"v1/share-profile-allowlisted", ManifestEntry{Version: "v1", Name: "share-profile-allowlisted.json", Schema: "share-profile.schema.json"}},
	{"v1/trusted-view-default-deny", ManifestEntry{Version: "v1", Name: "trusted-view-default-deny.json", Schema: "trusted-view.schema.json"}},
	{"v1/trusted-view", ManifestEntry{Version: "v1", Name: "trusted-view.json", Schema: "trusted-view.schema.json"}},
	{"v1/overview", ManifestEntry{Version: "v1", Name: "overview.json", Schema: "overview.schema.json"}},
	{"v1/rhythm", ManifestEntry{Version: "v1", Name: "rhythm.json", Schema: "rhythm.schema.json"}},
	{"v1/accuracy", ManifestEntry{Version: "v1", Name: "accuracy.json", Schema: "accuracy.schema.json"}},
	{"v2/medication-set", ManifestEntry{Version: "v2", Name: "medication-set.json", Schema: "medication-set.schema.json"}},
	{"v2/medication-event-set", ManifestEntry{Version: "v2", Name: "medication-event-set.json", Schema: "medication-event-set.schema.json"}},
	{"v2/medication-data-export", ManifestEntry{Version: "v2", Name: "medication-data-export.json", Schema: "medication-data-export.schema.json"}},
}

// Manifest returns a copy of the generated fixture registry in stable output
// order. Callers cannot mutate the package's authoritative registry.
func Manifest() []ManifestEntry {
	entries := make([]ManifestEntry, len(fixtureManifest))
	for i, spec := range fixtureManifest {
		entries[i] = spec.ManifestEntry
	}
	return entries
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func minutes(n int) time.Duration { return time.Duration(n) * time.Minute }

func boolPtr(b bool) *bool { return &b }

type provenance struct {
	AcquisitionMethod string `json:"acquisition_method"`
	EvidenceStatus    string `json:"evidence_status"`
	RecordedAt        string `json:"recorded_at"`
	SourceRecordID    string `json:"source_record_id"`
}

type sleepInfo struct {
	Classification string `json:"classification"`
}

type activityInfo struct {
	Level string `json:"level"`
}

type observation struct {
	ObservationID string        `json:"observation_id"`
	Kind          string        `json:"kind"`
	StartAt       string        `json:"start_at"`
	EndAt         string        `json:"end_at"`
	ZoneID        string        `json:"zone_id"`
	Sleep         *sleepInfo    `json:"sleep,omitempty"`
	Activity      *activityInfo `json:"activity,omitempty"`
	Provenance    provenance    `json:"provenance"`
}

type observationSet struct {
	SchemaVersion string        `json:"schema_version"`
	GeneratedAt   string        `json:"generated_at"`
	Observations  []observation `json:"observations"`
}

type correctionChanges struct {
	StartAt  string `json:"start_at,omitempty"`
	Excluded *bool  `json:"excluded,omitempty"`
}

type correction struct {
	CorrectionID        string            `json:"correction_id"`
	TargetObservationID string            `json:"target_observation_id"`
	CreatedAt           string            `json:"created_at"`
	Reason              string            `json:"reason"`
	Changes             correctionChanges `json:"changes"`
}

type correctionSet struct {
	SchemaVersion string       `json:"schema_version"`
	GeneratedAt   string       `json:"generated_at"`
	Corrections   []correction `json:"corrections"`
}

type sleepDataExport struct {
	SchemaVersion  string         `json:"schema_version"`
	GeneratedAt    string         `json:"generated_at"`
	ObservationSet observationSet `json:"observation_set"`
	CorrectionSet  correctionSet  `json:"correction_set"`
}

type syncRecord struct {
	Seq       int    `json:"seq"`
	RecordID  string `json:"recordId"`
	Kind      string `json:"kind"`
	DeviceID  string `json:"deviceId"`
	CreatedAt string `json:"createdAt"`
	Payload   any    `json:"payload"`
}

type syncBatch struct {
	SchemaVersion string       `json:"schema_version"`
	Cursor        int          `json:"cursor"`
	Records       []syncRecord `json:"records"`
}

type syncTombstonePayload struct {
	RecordID string `json:"record_id"`
}

type syncErase struct {
	SchemaVersion string   `json:"schema_version"`
	RecordIDs     []string `json:"record_ids"`
}

type taskSet struct {
	SchemaVersion string     `json:"schema_version"`
	GeneratedAt   string     `json:"generated_at"`
	Tasks         []taskItem `json:"tasks"`
}

type taskItem struct {
	TaskID                    string `json:"task_id"`
	Title                     string `json:"title"`
	DurationMinutes           int    `json:"duration_minutes"`
	Status                    string `json:"status"`
	CreatedAt                 string `json:"created_at"`
	EarliestStartAt           string `json:"earliest_start_at,omitempty"`
	LatestFinishAt            string `json:"latest_finish_at,omitempty"`
	PreferredAfterWakeMinutes int    `json:"preferred_after_wake_minutes,omitempty"`
	MinimumConfidence         string `json:"minimum_confidence,omitempty"`
	Revision                  int    `json:"revision,omitempty"`
	UpdatedAt                 string `json:"updated_at,omitempty"`
}

type calendarEventSet struct {
	SchemaVersion string           `json:"schema_version"`
	GeneratedAt   string           `json:"generated_at"`
	Sources       []calendarSource `json:"sources"`
	Events        []calendarEvent  `json:"events"`
}

type calendarSource struct {
	SourceID        string `json:"source_id"`
	Label           string `json:"label"`
	Kind            string `json:"kind"`
	ReadOnly        bool   `json:"read_only"`
	CoverageStartAt string `json:"coverage_start_at"`
	CoverageEndAt   string `json:"coverage_end_at"`
	LastImportedAt  string `json:"last_imported_at"`
}

type calendarEvent struct {
	EventID        string `json:"event_id"`
	SourceID       string `json:"source_id"`
	SourceRecordID string `json:"source_record_id"`
	Title          string `json:"title"`
	StartAt        string `json:"start_at"`
	EndAt          string `json:"end_at"`
	ZoneID         string `json:"zone_id"`
	AllDay         bool   `json:"all_day"`
	Busy           bool   `json:"busy"`
	Ownership      string `json:"ownership"`
	CreatedAt      string `json:"created_at"`
	Location       string `json:"location,omitempty"`
	Notes          string `json:"notes,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	TaskRevision   int    `json:"task_revision,omitempty"`
	ProposalID     string `json:"proposal_id,omitempty"`
}

type medicationSchedule struct {
	Kind            string   `json:"kind"`
	ZoneID          string   `json:"zone_id,omitempty"`
	CivilTimes      []string `json:"civil_times,omitempty"`
	DaysOn          int      `json:"days_on,omitempty"`
	DaysOff         int      `json:"days_off,omitempty"`
	CycleStartedOn  string   `json:"cycle_started_on,omitempty"`
	ReminderEnabled bool     `json:"reminder_enabled,omitempty"`
}

type medicationItem struct {
	MedicationID  string             `json:"medication_id"`
	Label         string             `json:"label"`
	Form          string             `json:"form,omitempty"`
	StrengthLabel string             `json:"strength_label,omitempty"`
	ClinicianRule string             `json:"clinician_rule,omitempty"`
	Active        bool               `json:"active"`
	Schedule      medicationSchedule `json:"schedule"`
	CreatedAt     string             `json:"created_at"`
	Revision      int                `json:"revision"`
	UpdatedAt     string             `json:"updated_at"`
}

type medicationSet struct {
	SchemaVersion string           `json:"schema_version"`
	GeneratedAt   string           `json:"generated_at"`
	Medications   []medicationItem `json:"medications"`
}

type medicationEventItem struct {
	EventID      string     `json:"event_id"`
	MedicationID string     `json:"medication_id"`
	DoseAt       string     `json:"dose_at"`
	ZoneID       string     `json:"zone_id"`
	Status       string     `json:"status"`
	Scheduled    bool       `json:"scheduled"`
	Note         string     `json:"note,omitempty"`
	Provenance   provenance `json:"provenance"`
}

type medicationEventChanges struct {
	Note string `json:"note,omitempty"`
}

type medicationEventCorrection struct {
	CorrectionID  string                 `json:"correction_id"`
	TargetEventID string                 `json:"target_event_id"`
	CreatedAt     string                 `json:"created_at"`
	Reason        string                 `json:"reason"`
	Changes       medicationEventChanges `json:"changes"`
}

type medicationEventSet struct {
	SchemaVersion string                      `json:"schema_version"`
	GeneratedAt   string                      `json:"generated_at"`
	Events        []medicationEventItem       `json:"events"`
	Corrections   []medicationEventCorrection `json:"corrections"`
}

type medicationDataExport struct {
	SchemaVersion string             `json:"schema_version"`
	GeneratedAt   string             `json:"generated_at"`
	MedicationSet medicationSet      `json:"medication_set"`
	EventSet      medicationEventSet `json:"event_set"`
}

type rhythmMarkerItem struct {
	MarkerID   string                 `json:"marker_id"`
	Kind       string                 `json:"kind"`
	StartAt    string                 `json:"start_at"`
	EndAt      string                 `json:"end_at,omitempty"`
	ZoneID     string                 `json:"zone_id"`
	Note       string                 `json:"note,omitempty"`
	Provenance rhythmMarkerProvenance `json:"provenance"`
}

type rhythmMarkerProvenance struct {
	AcquisitionMethod string `json:"acquisition_method"`
	EvidenceStatus    string `json:"evidence_status"`
	RecordedAt        string `json:"recorded_at"`
}

type rhythmMarkerSet struct {
	SchemaVersion string             `json:"schema_version"`
	GeneratedAt   string             `json:"generated_at"`
	Markers       []rhythmMarkerItem `json:"markers"`
}

type clinicalChartRange struct {
	Mode string `json:"mode"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type clinicalChartInclude struct {
	Forecast      bool `json:"forecast"`
	Medication    bool `json:"medication"`
	RhythmContext bool `json:"rhythm_context"`
}

type clinicalChartRequest struct {
	SchemaVersion string               `json:"schema_version"`
	Range         clinicalChartRange   `json:"range"`
	Orientation   string               `json:"orientation"`
	DayStartHour  int                  `json:"day_start_hour"`
	ZoneID        string               `json:"zone_id"`
	Include       clinicalChartInclude `json:"include"`
	Redactions    []string             `json:"redactions"`
}

type assistantActionTarget struct {
	TaskID                    string `json:"task_id"`
	EarliestStartAt           string `json:"earliest_start_at,omitempty"`
	LatestFinishAt            string `json:"latest_finish_at,omitempty"`
	DurationMinutes           int    `json:"duration_minutes,omitempty"`
	PreferredAfterWakeMinutes int    `json:"preferred_after_wake_minutes,omitempty"`
	ReminderID                string `json:"reminder_id,omitempty"`
}

type assistantAction struct {
	SchemaVersion     string                 `json:"schema_version"`
	RecommendedAction string                 `json:"recommended_action"`
	Target            *assistantActionTarget `json:"target,omitempty"`
	Answer            string                 `json:"answer,omitempty"`
}

type directTask struct {
	TaskID            string `json:"task_id"`
	DurationMinutes   int    `json:"duration_minutes"`
	EarliestStartAt   string `json:"earliest_start_at,omitempty"`
	LatestFinishAt    string `json:"latest_finish_at,omitempty"`
	MinimumConfidence string `json:"minimum_confidence,omitempty"`
}

type directAvailability struct {
	Kind       string `json:"kind"`
	StartAt    string `json:"start_at"`
	EndAt      string `json:"end_at"`
	ZoneID     string `json:"zone_id"`
	Confidence string `json:"confidence"`
}

type directFixedEvent struct {
	EventID string `json:"event_id"`
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
	ZoneID  string `json:"zone_id"`
}

type directPlanningContext struct {
	ZoneID       string               `json:"zone_id"`
	Now          string               `json:"now"`
	EstimateID   string               `json:"estimate_id,omitempty"`
	Tasks        []directTask         `json:"tasks"`
	Availability []directAvailability `json:"availability"`
	FixedEvents  []directFixedEvent   `json:"fixed_events,omitempty"`
}

type directProposalRequest struct {
	SchemaVersion     string                `json:"schema_version"`
	RecommendedAction string                `json:"recommended_action"`
	Target            assistantActionTarget `json:"target"`
	Answer            string                `json:"answer,omitempty"`
	Context           directPlanningContext `json:"context"`
}

type support struct {
	ObservationCount   int    `json:"observation_count"`
	CycleCount         int    `json:"cycle_count"`
	FirstObservationAt string `json:"first_observation_at"`
	LastObservationAt  string `json:"last_observation_at"`
}

type confidence struct {
	Level   string   `json:"level"`
	Reasons []string `json:"reasons"`
}

type uncertainWindow struct {
	EarliestAt string `json:"earliest_at"`
	LatestAt   string `json:"latest_at"`
	ZoneID     string `json:"zone_id"`
}

type minimizedWindow struct {
	EarliestAt string `json:"earliest_at"`
	LatestAt   string `json:"latest_at"`
}

type forecast struct {
	CycleIndex            int             `json:"cycle_index"`
	PredictedSleepWindow  uncertainWindow `json:"predicted_sleep_window"`
	PredictedWakingWindow uncertainWindow `json:"predicted_waking_window"`
}

type phaseEstimate struct {
	SchemaVersion                      string     `json:"schema_version"`
	Status                             string     `json:"status"`
	GeneratedAt                        string     `json:"generated_at"`
	AlgorithmVersion                   string     `json:"algorithm_version"`
	EstimateID                         string     `json:"estimate_id"`
	ObservedSleepStartDriftMinPerCycle int        `json:"observed_sleep_start_drift_minutes_per_cycle"`
	MedianSleepDurationMinutes         int        `json:"median_sleep_duration_minutes"`
	Support                            support    `json:"support"`
	Confidence                         confidence `json:"confidence"`
	Forecasts                          []forecast `json:"forecasts"`
}

type refusal struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type phaseEstimateRefused struct {
	SchemaVersion    string  `json:"schema_version"`
	Status           string  `json:"status"`
	GeneratedAt      string  `json:"generated_at"`
	AlgorithmVersion string  `json:"algorithm_version"`
	Refusal          refusal `json:"refusal"`
}

type task struct {
	TaskID          string `json:"task_id"`
	DurationMinutes int    `json:"duration_minutes"`
	EarliestAt      string `json:"earliest_at"`
	LatestAt        string `json:"latest_at"`
	Preference      string `json:"preference"`
}

type fixedEvent struct {
	EventID string `json:"event_id"`
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
}

type scheduleRequest struct {
	SchemaVersion string       `json:"schema_version"`
	RequestID     string       `json:"request_id"`
	CreatedAt     string       `json:"created_at"`
	ZoneID        string       `json:"zone_id"`
	EstimateID    string       `json:"estimate_id"`
	Tasks         []task       `json:"tasks"`
	FixedEvents   []fixedEvent `json:"fixed_events"`
}

type proposal struct {
	ProposalID       string     `json:"proposal_id"`
	TaskID           string     `json:"task_id"`
	StartAt          string     `json:"start_at"`
	EndAt            string     `json:"end_at"`
	ZoneID           string     `json:"zone_id"`
	Confidence       confidence `json:"confidence"`
	ExplanationCodes []string   `json:"explanation_codes"`
}

type unplaced struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type scheduleProposals struct {
	SchemaVersion    string     `json:"schema_version"`
	RequestID        string     `json:"request_id"`
	GeneratedAt      string     `json:"generated_at"`
	AlgorithmVersion string     `json:"algorithm_version"`
	Proposals        []proposal `json:"proposals"`
	Unplaced         []unplaced `json:"unplaced"`
}

type providerStatus struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
	Model      string `json:"model,omitempty"`
}

type storedProposal struct {
	ProposalID        string            `json:"proposal_id"`
	ActionID          string            `json:"action_id"`
	ScheduleProposals scheduleProposals `json:"schedule_proposals"`
	Answer            string            `json:"answer,omitempty"`
	CreatedAt         string            `json:"created_at"`
	ExpiresAt         string            `json:"expires_at"`
}

type proposalSummary struct {
	ProposalID    string         `json:"proposalId"`
	Status        string         `json:"status"`
	DecisionToken string         `json:"decisionToken"`
	Payload       storedProposal `json:"payload"`
}

type directProposalResponse struct {
	SchemaVersion string            `json:"schema_version"`
	Backend       providerStatus    `json:"backend"`
	Result        string            `json:"result"`
	Action        string            `json:"action"`
	Answer        string            `json:"answer"`
	Proposals     []proposalSummary `json:"proposals"`
}

type permissions struct {
	PredictedSleepWindow  bool `json:"predicted_sleep_window"`
	PredictedWakingWindow bool `json:"predicted_waking_window"`
	Confidence            bool `json:"confidence"`
	Availability          bool `json:"availability"`
}

type shareProfile struct {
	SchemaVersion string      `json:"schema_version"`
	ProfileID     string      `json:"profile_id"`
	State         string      `json:"state"`
	CreatedAt     string      `json:"created_at"`
	ExpiresAt     string      `json:"expires_at"`
	Permissions   permissions `json:"permissions"`
}

type trustedView struct {
	SchemaVersion         string           `json:"schema_version"`
	GeneratedAt           string           `json:"generated_at"`
	ExpiresAt             string           `json:"expires_at"`
	GrantedFields         []string         `json:"granted_fields"`
	PredictedSleepWindow  *minimizedWindow `json:"predicted_sleep_window,omitempty"`
	PredictedWakingWindow *minimizedWindow `json:"predicted_waking_window,omitempty"`
	Confidence            string           `json:"confidence,omitempty"`
	Notice                string           `json:"notice"`
}

type medicationEvent struct {
	Medication     string `json:"medication"`
	CivilTime      string `json:"civilTime"`
	RelativeToWake string `json:"relativeToWake"`
}

type overviewProjection struct {
	SchemaVersion            string            `json:"schema_version"`
	Status                   string            `json:"status"`
	Refusal                  *refusal          `json:"refusal,omitempty"`
	CurrentEstimatedState    string            `json:"currentEstimatedState,omitempty"`
	TimeSinceWake            string            `json:"timeSinceWake,omitempty"`
	PredictedNextSleepWindow string            `json:"predictedNextSleepWindow,omitempty"`
	DriftEstimate            string            `json:"driftEstimate,omitempty"`
	Confidence               string            `json:"confidence,omitempty"`
	ConfidenceReasons        []string          `json:"confidenceReasons,omitempty"`
	NextUsefulTaskWindow     string            `json:"nextUsefulTaskWindow,omitempty"`
	SharingStatus            string            `json:"sharingStatus,omitempty"`
	MedicationEvents         []medicationEvent `json:"medicationEvents"`
	FixtureMode              bool              `json:"fixtureMode"`
	Disclaimer               string            `json:"disclaimer"`
}

type rhythmBand struct {
	ID            string  `json:"id"`
	Day           string  `json:"day"`
	StartHour     float64 `json:"startHour"`
	DurationHours float64 `json:"durationHours"`
	Kind          string  `json:"kind"`
	StartLabel    string  `json:"startLabel"`
	WakeLabel     string  `json:"wakeLabel"`
	DurationLabel string  `json:"durationLabel"`
	Source        string  `json:"source"`
	Confidence    string  `json:"confidence"`
}

type rhythmNow struct {
	Label string  `json:"label"`
	Day   string  `json:"day"`
	Hour  float64 `json:"hour"`
}

type rhythmDriftPoint struct {
	ID           string  `json:"id"`
	Day          string  `json:"day"`
	OnsetHour    float64 `json:"onsetHour"`
	FitHour      float64 `json:"fitHour"`
	BandLowHour  float64 `json:"bandLowHour"`
	BandHighHour float64 `json:"bandHighHour"`
	OnsetLabel   string  `json:"onsetLabel"`
	Source       string  `json:"source"`
	Confidence   string  `json:"confidence"`
}

type rhythmProjectionBody struct {
	FixtureMode     bool               `json:"fixtureMode"`
	ActogramSummary string             `json:"actogramSummary"`
	ObservedRows    []rhythmBand       `json:"observedRows"`
	ForecastRows    []rhythmBand       `json:"forecastRows"`
	Now             rhythmNow          `json:"now"`
	DriftTitle      string             `json:"driftTitle"`
	SlopeLabel      string             `json:"slopeLabel"`
	DriftConfidence string             `json:"driftConfidence"`
	DriftSummary    string             `json:"driftSummary"`
	YMinHour        float64            `json:"yMinHour"`
	YMaxHour        float64            `json:"yMaxHour"`
	DriftPoints     []rhythmDriftPoint `json:"driftPoints"`
}

type rhythmProjection struct {
	SchemaVersion string                `json:"schema_version"`
	Status        string                `json:"status"`
	Refusal       *refusal              `json:"refusal,omitempty"`
	Projection    *rhythmProjectionBody `json:"projection,omitempty"`
}

type calibrationBucket struct {
	Level               string  `json:"level"`
	Count               int     `json:"count"`
	HitRate             float64 `json:"hitRate"`
	MedianAbsErrorHours float64 `json:"medianAbsErrorHours"`
}

type accuracyReport struct {
	Evaluations         int                 `json:"evaluations"`
	Refusals            int                 `json:"refusals"`
	MedianAbsErrorHours float64             `json:"medianAbsErrorHours"`
	MeanAbsErrorHours   float64             `json:"meanAbsErrorHours"`
	P90AbsErrorHours    float64             `json:"p90AbsErrorHours"`
	HitRate             float64             `json:"hitRate"`
	Calibration         []calibrationBucket `json:"calibration"`
}

type accuracyProjection struct {
	SchemaVersion string          `json:"schema_version"`
	Status        string          `json:"status"`
	Refusal       *refusal        `json:"refusal,omitempty"`
	Report        *accuracyReport `json:"report,omitempty"`
}

func uncertain(center time.Time, radius int) uncertainWindow {
	return uncertainWindow{
		EarliestAt: ts(center.Add(-minutes(radius))),
		LatestAt:   ts(center.Add(minutes(radius))),
		ZoneID:     zoneID,
	}
}

func minimized(center time.Time, radius int) *minimizedWindow {
	return &minimizedWindow{
		EarliestAt: ts(center.Add(-minutes(radius))),
		LatestAt:   ts(center.Add(minutes(radius))),
	}
}

// Build returns every fixture file with deterministic, encoded JSON in a
// stable order. It also enforces the same safety invariants the former Python
// generator did.
func Build() ([]File, error) {
	observations := make([]observation, 0, len(durations)+2)
	for i, d := range durations {
		start := baseStart.Add(time.Duration(i) * cycle)
		end := start.Add(minutes(d))
		observations = append(observations, observation{
			ObservationID: fmt.Sprintf("obs_sleep_%02d", i+1),
			Kind:          "sleep_episode",
			StartAt:       ts(start),
			EndAt:         ts(end),
			ZoneID:        zoneID,
			Sleep:         &sleepInfo{Classification: "principal"},
			Provenance: provenance{
				AcquisitionMethod: "synthetic",
				EvidenceStatus:    "directly_observed",
				RecordedAt:        ts(end.Add(minutes(5))),
				SourceRecordID:    fmt.Sprintf("synthetic-sleep-%02d", i+1),
			},
		})
	}

	napStart := baseStart.Add(6 * cycle).Add(13 * time.Hour)
	observations = append(observations, observation{
		ObservationID: "obs_sleep_nap_01",
		Kind:          "sleep_episode",
		StartAt:       ts(napStart),
		EndAt:         ts(napStart.Add(minutes(45))),
		ZoneID:        zoneID,
		Sleep:         &sleepInfo{Classification: "nap"},
		Provenance: provenance{
			AcquisitionMethod: "synthetic",
			EvidenceStatus:    "directly_observed",
			RecordedAt:        ts(napStart.Add(minutes(50))),
			SourceRecordID:    "synthetic-nap-01",
		},
	})

	activityStart := baseStart.Add(8 * cycle).Add(10 * time.Hour)
	observations = append(observations, observation{
		ObservationID: "obs_activity_01",
		Kind:          "activity_interval",
		StartAt:       ts(activityStart),
		EndAt:         ts(activityStart.Add(minutes(30))),
		ZoneID:        zoneID,
		Activity:      &activityInfo{Level: "active"},
		Provenance: provenance{
			AcquisitionMethod: "synthetic",
			EvidenceStatus:    "directly_observed",
			RecordedAt:        ts(activityStart.Add(minutes(35))),
			SourceRecordID:    "synthetic-activity-01",
		},
	})

	forecastSleep1 := baseStart.Add(10 * cycle)
	forecastSleep2 := baseStart.Add(11 * cycle)
	forecastWake1 := forecastSleep1.Add(minutes(480))
	forecastWake2 := forecastSleep2.Add(minutes(480))

	observationsFixture := observationSet{
		SchemaVersion: "v1",
		GeneratedAt:   ts(generatedAt),
		Observations:  observations,
	}

	correctionsFixture := correctionSet{
		SchemaVersion: "v1",
		GeneratedAt:   ts(generatedAt),
		Corrections: []correction{
			{
				CorrectionID:        "cor_sleep_04_start",
				TargetObservationID: "obs_sleep_04",
				CreatedAt:           "2026-03-08T08:25:00Z",
				Reason:              "user_edit",
				Changes:             correctionChanges{StartAt: "2026-03-08T07:10:00Z"},
			},
			{
				CorrectionID:        "cor_nap_excluded",
				TargetObservationID: "obs_sleep_nap_01",
				CreatedAt:           "2026-03-12T08:00:00Z",
				Reason:              "user_edit",
				Changes:             correctionChanges{Excluded: boolPtr(true)},
			},
		},
	}

	sleepDataExportFixture := sleepDataExport{
		SchemaVersion:  "v1",
		GeneratedAt:    ts(generatedAt),
		ObservationSet: observationsFixture,
		CorrectionSet:  correctionsFixture,
	}

	syncBatchFixture := syncBatch{
		SchemaVersion: "v1",
		Cursor:        4,
		Records: []syncRecord{
			{
				Seq:       1,
				RecordID:  observationsFixture.Observations[0].ObservationID,
				Kind:      "observation",
				DeviceID:  "device_desktop_01",
				CreatedAt: observationsFixture.Observations[0].Provenance.RecordedAt,
				Payload:   observationsFixture.Observations[0],
			},
			{
				Seq:       2,
				RecordID:  correctionsFixture.Corrections[0].CorrectionID,
				Kind:      "correction",
				DeviceID:  "device_android_01",
				CreatedAt: correctionsFixture.Corrections[0].CreatedAt,
				Payload:   correctionsFixture.Corrections[0],
			},
			{
				Seq:       3,
				RecordID:  "obs_sleep_erased_01",
				Kind:      "tombstone",
				DeviceID:  "device_desktop_01",
				CreatedAt: ts(generatedAt),
				Payload:   syncTombstonePayload{RecordID: "obs_sleep_erased_01"},
			},
			{
				Seq:       4,
				RecordID:  "task_flexible_01_r2",
				Kind:      "task",
				DeviceID:  "device_desktop_01",
				CreatedAt: ts(generatedAt),
				Payload: taskItem{
					TaskID:            "task_flexible_01",
					Title:             "Synthetic paperwork block (rescoped)",
					DurationMinutes:   60,
					Status:            "open",
					CreatedAt:         ts(generatedAt),
					MinimumConfidence: "low",
					Revision:          2,
					UpdatedAt:         ts(generatedAt),
				},
			},
		},
	}

	syncEraseFixture := syncErase{
		SchemaVersion: "v1",
		RecordIDs:     []string{"obs_sleep_erased_01", "corr_sleep_erased_01"},
	}

	taskSetFixture := taskSet{
		SchemaVersion: "v1",
		GeneratedAt:   ts(generatedAt),
		Tasks: []taskItem{
			{
				TaskID:            "task_flexible_01",
				Title:             "Synthetic paperwork block",
				DurationMinutes:   90,
				Status:            "open",
				CreatedAt:         ts(generatedAt),
				LatestFinishAt:    ts(forecastSleep2),
				MinimumConfidence: "low",
			},
			{
				TaskID:                    "task_flexible_02",
				Title:                     "Synthetic errand",
				DurationMinutes:           30,
				Status:                    "done",
				CreatedAt:                 ts(generatedAt),
				PreferredAfterWakeMinutes: 120,
			},
		},
	}

	medicationSetFixture := medicationSet{
		SchemaVersion: "v1",
		GeneratedAt:   ts(generatedAt),
		Medications: []medicationItem{
			{
				MedicationID:  "med_synthetic_01",
				Label:         "Synthetic medication record",
				Form:          "tablet",
				StrengthLabel: "user-entered strength",
				Active:        true,
				Schedule:      medicationSchedule{Kind: "as_needed"},
				CreatedAt:     ts(generatedAt.Add(-14 * 24 * time.Hour)),
				Revision:      1,
				UpdatedAt:     ts(generatedAt.Add(-14 * 24 * time.Hour)),
			},
		},
	}
	medicationEventSetFixture := medicationEventSet{
		SchemaVersion: "v1",
		GeneratedAt:   ts(generatedAt),
		Events: []medicationEventItem{
			{
				EventID:      "dose_synthetic_01",
				MedicationID: "med_synthetic_01",
				DoseAt:       ts(generatedAt.Add(-3 * time.Hour)),
				ZoneID:       zoneID,
				Status:       "taken",
				Scheduled:    false,
				Note:         "Synthetic local-only note",
				Provenance: provenance{
					AcquisitionMethod: "manual",
					EvidenceStatus:    "user_reported",
					RecordedAt:        ts(generatedAt.Add(-2 * time.Hour)),
					SourceRecordID:    "synthetic-dose-row-01",
				},
			},
		},
		Corrections: []medicationEventCorrection{
			{
				CorrectionID:  "medcorr_synthetic_01",
				TargetEventID: "dose_synthetic_01",
				CreatedAt:     ts(generatedAt.Add(-time.Hour)),
				Reason:        "user_edit",
				Changes:       medicationEventChanges{Note: "Corrected synthetic local-only note"},
			},
		},
	}
	medicationDataExportFixture := medicationDataExport{
		SchemaVersion: "v1",
		GeneratedAt:   ts(generatedAt),
		MedicationSet: medicationSetFixture,
		EventSet:      medicationEventSetFixture,
	}
	rhythmMarkerSetFixture := rhythmMarkerSet{
		SchemaVersion: "v1",
		GeneratedAt:   ts(generatedAt),
		Markers: []rhythmMarkerItem{
			{
				MarkerID: "marker_travel_01",
				Kind:     "travel",
				StartAt:  "2026-03-10T15:00:00Z",
				EndAt:    "2026-03-12T20:00:00Z",
				ZoneID:   zoneID,
				Note:     "Synthetic time-zone travel context",
				Provenance: rhythmMarkerProvenance{
					AcquisitionMethod: "manual",
					EvidenceStatus:    "user_reported",
					RecordedAt:        "2026-03-12T21:00:00Z",
				},
			},
			{
				MarkerID: "marker_illness_01",
				Kind:     "illness",
				StartAt:  "2026-03-13T12:00:00Z",
				EndAt:    "2026-03-14T12:00:00Z",
				ZoneID:   zoneID,
				Provenance: rhythmMarkerProvenance{
					AcquisitionMethod: "manual",
					EvidenceStatus:    "user_reported",
					RecordedAt:        "2026-03-14T13:00:00Z",
				},
			},
			{
				MarkerID: "marker_disruption_01",
				Kind:     "disruption",
				StartAt:  "2026-03-14T22:30:00Z",
				ZoneID:   zoneID,
				Note:     "Synthetic interrupted-sleep context",
				Provenance: rhythmMarkerProvenance{
					AcquisitionMethod: "manual",
					EvidenceStatus:    "user_reported",
					RecordedAt:        "2026-03-14T23:00:00Z",
				},
			},
			{
				MarkerID: "marker_forced_schedule_01",
				Kind:     "forced_schedule",
				StartAt:  "2026-03-14T23:30:00Z",
				EndAt:    "2026-03-15T00:00:00Z",
				ZoneID:   zoneID,
				Provenance: rhythmMarkerProvenance{
					AcquisitionMethod: "manual",
					EvidenceStatus:    "user_reported",
					RecordedAt:        "2026-03-15T00:00:00Z",
				},
			},
		},
	}
	clinicalChartRequestFixture := clinicalChartRequest{
		SchemaVersion: "v1",
		Range:         clinicalChartRange{Mode: "custom", From: "2026-03-01", To: "2026-03-31"},
		Orientation:   "24h",
		DayStartHour:  18,
		ZoneID:        zoneID,
		Include:       clinicalChartInclude{Forecast: false, Medication: true, RhythmContext: true},
		Redactions:    []string{"diagnosis", "location", "clinician_rule", "medication_labels", "medication_notes", "rhythm_context_notes"},
	}
	medicationSetFixtureV2 := medicationSet{
		SchemaVersion: "v2",
		GeneratedAt:   ts(generatedAt),
		Medications: []medicationItem{
			{
				MedicationID:  "med_synthetic_01",
				Label:         "Synthetic medication record",
				Form:          "tablet",
				StrengthLabel: "user-entered strength",
				ClinicianRule: "Synthetic clinician instruction entered verbatim by the user",
				Active:        true,
				Schedule: medicationSchedule{
					Kind:            "fixed_clock",
					ZoneID:          zoneID,
					CivilTimes:      []string{"09:00", "21:00"},
					ReminderEnabled: true,
				},
				CreatedAt: ts(generatedAt.Add(-14 * 24 * time.Hour)),
				Revision:  2,
				UpdatedAt: ts(generatedAt.Add(-time.Hour)),
			},
		},
	}
	medicationEventSetFixtureV2 := medicationEventSetFixture
	medicationEventSetFixtureV2.SchemaVersion = "v2"
	medicationDataExportFixtureV2 := medicationDataExport{
		SchemaVersion: "v2",
		GeneratedAt:   ts(generatedAt),
		MedicationSet: medicationSetFixtureV2,
		EventSet:      medicationEventSetFixtureV2,
	}

	assistantActionFixture := assistantAction{
		SchemaVersion:     "v1",
		RecommendedAction: "propose_place_task",
		Target: &assistantActionTarget{
			TaskID:                    "task_flexible_01",
			EarliestStartAt:           ts(forecastWake1.Add(minutes(30))),
			LatestFinishAt:            ts(forecastSleep2.Add(-2 * time.Hour)),
			DurationMinutes:           30,
			PreferredAfterWakeMinutes: 90,
		},
		Answer: "I can queue a schedule proposal inside a predicted waking window.",
	}

	directProposalRequestFixture := directProposalRequest{
		SchemaVersion:     "v1",
		RecommendedAction: "propose_place_task",
		Target:            assistantActionTarget{TaskID: "task_flexible_01"},
		Context: directPlanningContext{
			ZoneID:     zoneID,
			Now:        ts(generatedAt),
			EstimateID: "estimate_synthetic_01",
			Tasks: []directTask{
				{
					TaskID:            "task_flexible_01",
					DurationMinutes:   30,
					EarliestStartAt:   ts(forecastWake1.Add(minutes(30))),
					LatestFinishAt:    ts(forecastSleep2.Add(-2 * time.Hour)),
					MinimumConfidence: "low",
				},
			},
			Availability: []directAvailability{
				{
					Kind:       "predicted_wake",
					StartAt:    ts(forecastWake1),
					EndAt:      ts(forecastSleep2.Add(-1 * time.Hour)),
					ZoneID:     zoneID,
					Confidence: "medium",
				},
			},
			FixedEvents: []directFixedEvent{
				{
					EventID: "event_fixed_01",
					StartAt: ts(forecastWake1.Add(2 * time.Hour)),
					EndAt:   ts(forecastWake1.Add(3 * time.Hour)),
					ZoneID:  zoneID,
				},
			},
		},
	}

	estimateFixture := phaseEstimate{
		SchemaVersion:                      "v1",
		Status:                             "estimated",
		GeneratedAt:                        ts(generatedAt),
		AlgorithmVersion:                   "theil-sen-sleep-start-v1",
		EstimateID:                         "estimate_synthetic_01",
		ObservedSleepStartDriftMinPerCycle: 50,
		MedianSleepDurationMinutes:         480,
		Support: support{
			ObservationCount:   10,
			CycleCount:         10,
			FirstObservationAt: ts(baseStart),
			LastObservationAt:  ts(baseStart.Add(9 * cycle)),
		},
		Confidence: confidence{
			Level: "medium",
			Reasons: []string{
				"ten principal synthetic sleep episodes support the estimate",
				"forecast uncertainty widens with horizon",
			},
		},
		Forecasts: []forecast{
			{
				CycleIndex:            11,
				PredictedSleepWindow:  uncertain(forecastSleep1, 30),
				PredictedWakingWindow: uncertain(forecastWake1, 45),
			},
			{
				CycleIndex:            12,
				PredictedSleepWindow:  uncertain(forecastSleep2, 45),
				PredictedWakingWindow: uncertain(forecastWake2, 60),
			},
		},
	}

	refusedEstimateFixture := phaseEstimateRefused{
		SchemaVersion:    "v1",
		Status:           "refused",
		GeneratedAt:      ts(generatedAt),
		AlgorithmVersion: "theil-sen-sleep-start-v1",
		Refusal: refusal{
			Code:    "insufficient_data",
			Message: "Need at least seven usable principal sleep episodes; found three.",
		},
	}

	scheduleRequestFixture := scheduleRequest{
		SchemaVersion: "v1",
		RequestID:     "schedule_request_01",
		CreatedAt:     ts(generatedAt),
		ZoneID:        zoneID,
		EstimateID:    "estimate_synthetic_01",
		Tasks: []task{
			{
				TaskID:          "task_flexible_01",
				DurationMinutes: 30,
				EarliestAt:      ts(forecastWake1.Add(minutes(30))),
				LatestAt:        ts(forecastSleep2.Add(-2 * time.Hour)),
				Preference:      "predicted_waking_window",
			},
			{
				TaskID:          "task_flexible_02",
				DurationMinutes: 90,
				EarliestAt:      ts(forecastWake1),
				LatestAt:        ts(forecastWake1.Add(minutes(30))),
				Preference:      "any_available",
			},
		},
		FixedEvents: []fixedEvent{
			{
				EventID: "event_fixed_01",
				StartAt: ts(forecastWake1.Add(2 * time.Hour)),
				EndAt:   ts(forecastWake1.Add(3 * time.Hour)),
			},
		},
	}

	proposalStart := forecastWake1.Add(minutes(30))
	scheduleProposalsFixture := scheduleProposals{
		SchemaVersion:    "v1",
		RequestID:        "schedule_request_01",
		GeneratedAt:      ts(generatedAt),
		AlgorithmVersion: "deterministic-proposal-v1",
		Proposals: []proposal{
			{
				ProposalID: "proposal_task_01",
				TaskID:     "task_flexible_01",
				StartAt:    ts(proposalStart),
				EndAt:      ts(proposalStart.Add(minutes(30))),
				ZoneID:     zoneID,
				Confidence: confidence{
					Level:   "medium",
					Reasons: []string{"proposal uses an uncertain predicted waking window"},
				},
				ExplanationCodes: []string{
					"within_predicted_waking_window",
					"avoids_fixed_event",
					"within_task_bounds",
					"uncertainty_buffer_applied",
				},
			},
		},
		Unplaced: []unplaced{
			{TaskID: "task_flexible_02", Reason: "no_available_interval"},
		},
	}

	coverageStart := generatedAt.Add(-366 * 24 * time.Hour)
	coverageEnd := generatedAt.Add(732 * 24 * time.Hour)
	calendarEventSetFixture := calendarEventSet{
		SchemaVersion: "v1",
		GeneratedAt:   ts(generatedAt),
		Sources: []calendarSource{
			{
				SourceID:        "calendar_source_ics_01",
				Label:           "Synthetic commitments",
				Kind:            "ics",
				ReadOnly:        true,
				CoverageStartAt: ts(coverageStart),
				CoverageEndAt:   ts(coverageEnd),
				LastImportedAt:  ts(generatedAt),
			},
			{
				SourceID:        "calendar_source_zeitboard",
				Label:           "ZeitBoard placements",
				Kind:            "zeitboard",
				ReadOnly:        false,
				CoverageStartAt: ts(coverageStart),
				CoverageEndAt:   ts(coverageEnd),
				LastImportedAt:  ts(generatedAt),
			},
		},
		Events: []calendarEvent{
			{
				EventID:        "calendar_event_imported_01",
				SourceID:       "calendar_source_ics_01",
				SourceRecordID: "fixture-event-01@zeitboard.local/20260315T190000Z",
				Title:          "Synthetic fixed commitment",
				StartAt:        ts(forecastWake1.Add(2 * time.Hour)),
				EndAt:          ts(forecastWake1.Add(3 * time.Hour)),
				ZoneID:         zoneID,
				AllDay:         false,
				Busy:           true,
				Ownership:      "imported",
				CreatedAt:      ts(generatedAt),
				Location:       "Synthetic location",
			},
			{
				EventID:        "calendar_event_owned_01",
				SourceID:       "calendar_source_zeitboard",
				SourceRecordID: "proposal_task_01",
				Title:          "Synthetic paperwork block",
				StartAt:        ts(proposalStart),
				EndAt:          ts(proposalStart.Add(minutes(30))),
				ZoneID:         zoneID,
				AllDay:         false,
				Busy:           true,
				Ownership:      "app_owned",
				CreatedAt:      ts(generatedAt),
				TaskID:         "task_flexible_01",
				TaskRevision:   1,
				ProposalID:     "proposal_task_01",
			},
		},
	}

	directProposalResponseFixture := directProposalResponse{
		SchemaVersion: "v1",
		Backend:       providerStatus{Configured: false, Provider: "disabled"},
		Result:        "proposal_pending",
		Action:        "propose_place_task",
		Answer:        "I queued a schedule proposal for approval. It awaits human approval.",
		Proposals: []proposalSummary{
			{
				ProposalID:    "proposal_task_01",
				Status:        "pending",
				DecisionToken: "fixture_decision_token_01",
				Payload: storedProposal{
					ProposalID:        "proposal_task_01",
					ActionID:          "propose_place_task",
					ScheduleProposals: scheduleProposalsFixture,
					CreatedAt:         ts(generatedAt),
					ExpiresAt:         ts(generatedAt.Add(15 * time.Minute)),
				},
			},
		},
	}

	shareProfileDefaultDeny := shareProfile{
		SchemaVersion: "v1",
		ProfileID:     "share_profile_deny_01",
		State:         "active",
		CreatedAt:     ts(generatedAt),
		ExpiresAt:     ts(generatedAt.Add(24 * time.Hour)),
		Permissions:   permissions{},
	}

	shareProfileAllowlisted := shareProfile{
		SchemaVersion: "v1",
		ProfileID:     "share_profile_allow_01",
		State:         "active",
		CreatedAt:     ts(generatedAt),
		ExpiresAt:     ts(generatedAt.Add(24 * time.Hour)),
		Permissions: permissions{
			PredictedSleepWindow:  true,
			PredictedWakingWindow: true,
			Confidence:            true,
			Availability:          false,
		},
	}

	trustedViewDefaultDeny := trustedView{
		SchemaVersion: "v1",
		GeneratedAt:   ts(generatedAt),
		ExpiresAt:     ts(generatedAt.Add(24 * time.Hour)),
		GrantedFields: []string{},
		Notice:        notice,
	}

	trustedViewFixture := trustedView{
		SchemaVersion: "v1",
		GeneratedAt:   ts(generatedAt),
		ExpiresAt:     ts(generatedAt.Add(24 * time.Hour)),
		GrantedFields: []string{
			"predicted_sleep_window",
			"predicted_waking_window",
			"confidence",
		},
		PredictedSleepWindow:  minimized(forecastSleep1, 30),
		PredictedWakingWindow: minimized(forecastWake1, 45),
		Confidence:            "medium",
		Notice:                notice,
	}

	overviewFixture := overviewProjection{
		SchemaVersion:            "v1",
		Status:                   "estimated",
		CurrentEstimatedState:    "Likely awake",
		TimeSinceWake:            "19 hours 30 minutes",
		PredictedNextSleepWindow: "Mar 15, 8:20 AM EDT to Mar 15, 5:35 PM EDT",
		DriftEstimate:            "+50 minutes per observed sleep cycle",
		Confidence:               "medium",
		ConfidenceReasons: []string{
			"10 usable principal sleep episodes",
			"robust residual spread about 0s",
		},
		NextUsefulTaskWindow: "Task sync is not configured yet.",
		SharingStatus:        "Server projection only; trusted sharing remains default-deny.",
		MedicationEvents:     []medicationEvent{},
		FixtureMode:          false,
		Disclaimer:           "Estimates describe observed sleep-wake timing and uncertainty. This application does not provide medical advice.",
	}

	rhythmFixture := rhythmProjection{
		SchemaVersion: "v1",
		Status:        "estimated",
		Projection: &rhythmProjectionBody{
			FixtureMode:     false,
			ActogramSummary: "Double-plotted actogram of synced sleep with widening predicted sleep windows, all derived from the server estimate.",
			ObservedRows: []rhythmBand{
				{
					ID:            "observed-1",
					Day:           "Mar 14",
					StartHour:     7.67,
					DurationHours: 8,
					Kind:          "observed",
					StartLabel:    "Mar 14, 7:40 AM",
					WakeLabel:     "Mar 14, 3:40 PM",
					DurationLabel: "8 hr 0 min",
					Source:        "Synthetic sleep",
					Confidence:    "High",
				},
				{
					ID:            "observed-2",
					Day:           "Mar 13",
					StartHour:     6.83,
					DurationHours: 8,
					Kind:          "observed",
					StartLabel:    "Mar 13, 6:50 AM",
					WakeLabel:     "Mar 13, 2:50 PM",
					DurationLabel: "8 hr 0 min",
					Source:        "Synthetic sleep",
					Confidence:    "High",
				},
			},
			ForecastRows: []rhythmBand{
				{
					ID:            "forecast-1",
					Day:           "Mar 15",
					StartHour:     8.33,
					DurationHours: 9.25,
					Kind:          "forecast",
					StartLabel:    "Mar 15, 8:20 AM earliest",
					WakeLabel:     "Mar 15, 5:35 PM latest",
					DurationLabel: "9 hr 15 min window",
					Source:        "Forecast cycle 1",
					Confidence:    "Medium",
				},
				{
					ID:            "forecast-2",
					Day:           "Mar 16",
					StartHour:     9,
					DurationHours: 9.75,
					Kind:          "forecast",
					StartLabel:    "Mar 16, 9:00 AM earliest",
					WakeLabel:     "Mar 16, 6:45 PM latest",
					DurationLabel: "9 hr 45 min window",
					Source:        "Forecast cycle 2",
					Confidence:    "Medium",
				},
			},
			Now:             rhythmNow{Label: "now", Day: "Mar 15", Hour: 12},
			DriftTitle:      "Sleep-onset drift",
			SlopeLabel:      "+50 min per cycle",
			DriftConfidence: "Medium",
			DriftSummary:    "Sleep onset drifts later by about 50 minutes per observed sleep cycle with medium confidence.",
			YMinHour:        2.5,
			YMaxHour:        10.5,
			DriftPoints: []rhythmDriftPoint{
				{
					ID:           "drift-1",
					Day:          "Mar 5",
					OnsetHour:    4.5,
					FitHour:      4.5,
					BandLowHour:  4,
					BandHighHour: 5,
					OnsetLabel:   "4:30 AM",
					Source:       "Synthetic sleep",
					Confidence:   "High",
				},
				{
					ID:           "drift-2",
					Day:          "Mar 6",
					OnsetHour:    5.33,
					FitHour:      5.33,
					BandLowHour:  4.83,
					BandHighHour: 5.83,
					OnsetLabel:   "5:20 AM",
					Source:       "Synthetic sleep",
					Confidence:   "High",
				},
			},
		},
	}

	accuracyFixture := accuracyProjection{
		SchemaVersion: "v1",
		Status:        "estimated",
		Report: &accuracyReport{
			Evaluations:         3,
			Refusals:            0,
			MedianAbsErrorHours: 0,
			MeanAbsErrorHours:   0,
			P90AbsErrorHours:    0,
			HitRate:             1,
			Calibration: []calibrationBucket{
				{
					Level:               "medium",
					Count:               3,
					HitRate:             1,
					MedianAbsErrorHours: 0,
				},
			},
		},
	}

	if err := assertSafety(observationsFixture, trustedViewDefaultDeny, trustedViewFixture); err != nil {
		return nil, err
	}

	values := map[fixtureID]any{
		"v1/observations":               observationsFixture,
		"v1/corrections":                correctionsFixture,
		"v1/sleep-data-export":          sleepDataExportFixture,
		"v1/sync-batch":                 syncBatchFixture,
		"v1/sync-erase":                 syncEraseFixture,
		"v1/task-set":                   taskSetFixture,
		"v1/calendar-event-set":         calendarEventSetFixture,
		"v1/medication-set":             medicationSetFixture,
		"v1/medication-event-set":       medicationEventSetFixture,
		"v1/medication-data-export":     medicationDataExportFixture,
		"v1/rhythm-marker-set":          rhythmMarkerSetFixture,
		"v1/clinical-chart-request":     clinicalChartRequestFixture,
		"v1/assistant-action":           assistantActionFixture,
		"v1/direct-proposal-request":    directProposalRequestFixture,
		"v1/phase-estimate":             estimateFixture,
		"v1/phase-estimate-refused":     refusedEstimateFixture,
		"v1/schedule-request":           scheduleRequestFixture,
		"v1/schedule-proposals":         scheduleProposalsFixture,
		"v1/proposal-response":          directProposalResponseFixture,
		"v1/share-profile-default-deny": shareProfileDefaultDeny,
		"v1/share-profile-allowlisted":  shareProfileAllowlisted,
		"v1/trusted-view-default-deny":  trustedViewDefaultDeny,
		"v1/trusted-view":               trustedViewFixture,
		"v1/overview":                   overviewFixture,
		"v1/rhythm":                     rhythmFixture,
		"v1/accuracy":                   accuracyFixture,
		"v2/medication-set":             medicationSetFixtureV2,
		"v2/medication-event-set":       medicationEventSetFixtureV2,
		"v2/medication-data-export":     medicationDataExportFixtureV2,
	}
	return encodeFixtureManifest(fixtureManifest, values)
}

func encodeFixtureManifest(specs []fixtureSpec, values map[fixtureID]any) ([]File, error) {
	files := make([]File, 0, len(specs))
	seenIDs := make(map[fixtureID]struct{}, len(specs))
	seenPaths := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if spec.id == "" || spec.Version == "" || spec.Name == "" || spec.Schema == "" {
			return nil, fmt.Errorf("fixture manifest contains incomplete entry for %q", spec.id)
		}
		if !validPathElement(spec.Version) || !validPathElement(spec.Name) || !validPathElement(spec.Schema) {
			return nil, fmt.Errorf("fixture manifest entry %q contains a non-base path component", spec.id)
		}
		if !strings.HasSuffix(spec.Name, ".json") || !strings.HasSuffix(spec.Schema, ".schema.json") {
			return nil, fmt.Errorf("fixture manifest entry %q has invalid fixture or schema suffix", spec.id)
		}
		if _, duplicate := seenIDs[spec.id]; duplicate {
			return nil, fmt.Errorf("fixture manifest contains duplicate id %q", spec.id)
		}
		seenIDs[spec.id] = struct{}{}
		generatedPath := spec.GeneratedPath()
		if _, duplicate := seenPaths[generatedPath]; duplicate {
			return nil, fmt.Errorf("fixture manifest contains duplicate generated path %q", generatedPath)
		}
		seenPaths[generatedPath] = struct{}{}

		value, ok := values[spec.id]
		if !ok {
			return nil, fmt.Errorf("fixture manifest entry %q has no generated value", spec.id)
		}
		data, err := encode(value)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", generatedPath, err)
		}
		files = append(files, File{ManifestEntry: spec.ManifestEntry, Data: data})
	}

	var unexpected []string
	for id := range values {
		if _, ok := seenIDs[id]; !ok {
			unexpected = append(unexpected, string(id))
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		return nil, fmt.Errorf("generated fixture values are absent from the manifest: %v", unexpected)
	}
	return files, nil
}

func validPathElement(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, `/\`)
}

// encode matches the former Python json.dumps(indent=2) + "\n" output: two
// space indent, no HTML escaping, trailing newline.
func encode(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var forbiddenTrustedKeys = map[string]struct{}{
	"calendar_text": {}, "diagnosis": {}, "location": {}, "medication": {},
	"notes": {}, "observation_id": {}, "profile_id": {}, "provenance": {},
	"raw_activity": {}, "zone_id": {},
}

func assertSafety(observations observationSet, trustedViews ...trustedView) error {
	for _, obs := range observations.Observations {
		if obs.Provenance.AcquisitionMethod != "synthetic" {
			return fmt.Errorf("observation %s is not synthetic", obs.ObservationID)
		}
	}
	for _, view := range trustedViews {
		data, err := json.Marshal(view)
		if err != nil {
			return err
		}
		var decoded any
		if err := json.Unmarshal(data, &decoded); err != nil {
			return err
		}
		if err := walkForbidden(decoded); err != nil {
			return err
		}
	}
	return nil
}

func walkForbidden(value any) error {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if _, bad := forbiddenTrustedKeys[key]; bad {
				return fmt.Errorf("trusted view contains forbidden key: %s", key)
			}
			if err := walkForbidden(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := walkForbidden(child); err != nil {
				return err
			}
		}
	}
	return nil
}

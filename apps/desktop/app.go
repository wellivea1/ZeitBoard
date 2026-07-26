package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"non24.app/core/domain"
	"non24.app/core/estimation"
	"non24.app/core/ingest"
	"non24.app/core/platform/activity"
	"non24.app/core/scheduling"
	storage "non24.app/core/storage/sqlite"
	"non24.app/desktop/platform/tray"
)

const (
	defaultZoneID = "America/New_York"
	disclaimer    = "Estimates describe observed sleep-wake timing and uncertainty. This application does not provide medical advice."
	deleteConfirm = "DELETE"
)

type App struct {
	ctx                context.Context
	collector          *ingest.Manager
	sink               *ingest.MemorySink
	tray               tray.Controller
	store              *storage.Store
	storeErr           error
	configDir          string
	calendarHTTPClient calendarHTTPDoer
	nowFn              func() time.Time
	reminderMu         sync.RWMutex
	reminderCancel     context.CancelFunc
	reminderDone       chan struct{}
	reminderRunning    bool
	reminderLastError  string
}

type RefusalDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type OverviewDTO struct {
	EstimateSource           string               `json:"estimateSource"`
	Status                   string               `json:"status"`
	Empty                    bool                 `json:"empty"`
	Refusal                  *RefusalDTO          `json:"refusal,omitempty"`
	CurrentEstimatedState    string               `json:"currentEstimatedState"`
	TimeSinceWake            string               `json:"timeSinceWake"`
	PredictedNextSleepWindow string               `json:"predictedNextSleepWindow"`
	DriftEstimate            string               `json:"driftEstimate"`
	Confidence               string               `json:"confidence"`
	ConfidenceReasons        []string             `json:"confidenceReasons"`
	NextUsefulTaskWindow     string               `json:"nextUsefulTaskWindow"`
	SharingStatus            string               `json:"sharingStatus"`
	MedicationEvents         []MedicationEventDTO `json:"medicationEvents"`
	FixtureMode              bool                 `json:"fixtureMode"`
	Disclaimer               string               `json:"disclaimer"`
	UpdatedLabel             string               `json:"updatedLabel"`
}

type MedicationEventDTO struct {
	Medication     string `json:"medication"`
	CivilTime      string `json:"civilTime"`
	RelativeToWake string `json:"relativeToWake"`
}

type ProposalDTO struct {
	ID               string   `json:"id"`
	Origin           string   `json:"origin"`
	Kind             string   `json:"kind"`
	Title            string   `json:"title"`
	To               string   `json:"to"`
	RhythmContext    string   `json:"rhythmContext"`
	Confidence       string   `json:"confidence"`
	ExplanationCodes []string `json:"explanationCodes"`
	ReasonLabels     []string `json:"reasonLabels"`
	CreatedLabel     string   `json:"createdLabel"`
	ExpiresLabel     string   `json:"expiresLabel"`
	Decision         string   `json:"decision"`
	CanUndo          bool     `json:"canUndo"`
}

type UnplacedDTO struct {
	Title      string `json:"title"`
	Reason     string `json:"reason"`
	ReasonCode string `json:"reasonCode"`
	NextAction string `json:"nextAction"`
}

type ProposalsDTO struct {
	Status      string        `json:"status"`
	Refusal     *RefusalDTO   `json:"refusal,omitempty"`
	FixtureMode bool          `json:"fixtureMode"`
	Proposals   []ProposalDTO `json:"proposals"`
	Unplaced    []UnplacedDTO `json:"unplaced"`
}

type SleepEntryInput struct {
	StartLocal     string `json:"startLocal"`
	EndLocal       string `json:"endLocal"`
	ZoneID         string `json:"zoneId"`
	Classification string `json:"classification"`
}

type SleepCorrectionInput struct {
	ObservationID  string `json:"observationId"`
	StartLocal     string `json:"startLocal"`
	EndLocal       string `json:"endLocal"`
	ZoneID         string `json:"zoneId"`
	Classification string `json:"classification"`
}

type SleepSuppressInput struct {
	ObservationID string `json:"observationId"`
}

type SleepDeleteInput struct {
	ObservationID string `json:"observationId"`
	Confirmation  string `json:"confirmation"`
}

type SleepDeleteAllInput struct {
	Confirmation string `json:"confirmation"`
}

type SleepEntriesDTO struct {
	Status  string          `json:"status"`
	Empty   bool            `json:"empty"`
	Message string          `json:"message"`
	Entries []SleepEntryDTO `json:"entries"`
}

type SleepDataExportDTO struct {
	FileName         string `json:"fileName"`
	JSON             string `json:"json"`
	GeneratedLabel   string `json:"generatedLabel"`
	ObservationCount int    `json:"observationCount"`
	CorrectionCount  int    `json:"correctionCount"`
}

type SleepEntryDTO struct {
	ObservationID           string               `json:"observationId"`
	StartLocal              string               `json:"startLocal"`
	EndLocal                string               `json:"endLocal"`
	StartLabel              string               `json:"startLabel"`
	EndLabel                string               `json:"endLabel"`
	ZoneID                  string               `json:"zoneId"`
	Classification          string               `json:"classification"`
	EffectiveStartLocal     string               `json:"effectiveStartLocal"`
	EffectiveEndLocal       string               `json:"effectiveEndLocal"`
	EffectiveStartLabel     string               `json:"effectiveStartLabel"`
	EffectiveEndLabel       string               `json:"effectiveEndLabel"`
	EffectiveClassification string               `json:"effectiveClassification"`
	DurationLabel           string               `json:"durationLabel"`
	Suppressed              bool                 `json:"suppressed"`
	SourceLabel             string               `json:"sourceLabel"`
	ProvenanceLabel         string               `json:"provenanceLabel"`
	History                 []SleepCorrectionDTO `json:"history"`
}

type SleepCorrectionDTO struct {
	CorrectionID           string `json:"correctionId"`
	SupersedesCorrectionID string `json:"supersedesCorrectionId,omitempty"`
	CreatedLabel           string `json:"createdLabel"`
	Reason                 string `json:"reason"`
	Summary                string `json:"summary"`
}

type localEstimateState struct {
	Status   string
	Message  string
	Sessions []domain.SleepSession
	Estimate domain.PhaseEstimate
	Refusal  *estimation.EstimationRefusal
}

func NewApp() *App {
	store, err := openDesktopStore()
	return newAppWithStore(store, err)
}

func newAppWithStore(store *storage.Store, storeErr error) *App {
	sink := &ingest.MemorySink{}
	return &App{
		sink:               sink,
		collector:          ingest.NewManager(sink, activity.SafeCollector{ZoneID: defaultZoneID}),
		tray:               tray.New(),
		store:              store,
		storeErr:           storeErr,
		calendarHTTPClient: newCalendarHTTPClient(),
		nowFn:              time.Now,
	}
}

func (a *App) currentTime() time.Time {
	if a.nowFn != nil {
		return a.nowFn()
	}
	return time.Now()
}

func openDesktopStore() (*storage.Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, "ZeitBoard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return storage.Open(filepath.Join(dir, "zeitboard-desktop.db"))
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	_ = a.collector.Start(ctx)
	trayErr := a.tray.Start(tray.Callbacks{
		Show: func() {
			runtime.WindowUnminimise(ctx)
			runtime.WindowShow(ctx)
			runtime.WindowCenter(ctx)
		},
		Quit: func() { runtime.Quit(ctx) },
	})
	if trayErr != nil {
		a.setMedicationReminderError("Desktop notifications are unavailable; enabled reminders will not be shown.")
		return
	}
	a.startMedicationReminderService(ctx)
}

func (a *App) shutdown(ctx context.Context) {
	a.stopMedicationReminderService()
	_ = a.tray.Stop()
	_ = a.collector.Stop(ctx)
	if a.store != nil {
		_ = a.store.Close()
	}
}

func (a *App) beforeClose(ctx context.Context) bool {
	return false
}

func (a *App) HideWindow() {
	if a.ctx != nil {
		runtime.WindowHide(a.ctx)
	}
}

func (a *App) GetCollectorHealth() ingest.ServiceHealth {
	return a.collector.Health(context.Background())
}

func (a *App) AddSleepEntry(input SleepEntryInput) (SleepEntryDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepEntryDTO{}, err
	}
	start, end, zoneID, classification, err := parseSleepInput(input)
	if err != nil {
		return SleepEntryDTO{}, err
	}
	now := time.Now().UTC()
	record := storage.SleepObservationRecord{
		ObservationID: newLocalID("obs_sleep"),
		Kind:          storage.SleepKindEpisode,
		StartAt:       start,
		EndAt:         end,
		ZoneID:        zoneID,
		Sleep:         storage.SleepObservationDetails{Classification: classification},
		Provenance: storage.SleepObservationProvenance{
			AcquisitionMethod: storage.ProvenanceAcquisitionManual,
			EvidenceStatus:    storage.ProvenanceEvidenceUserReported,
			RecordedAt:        now,
			SourceRecordID:    "desktop-manual",
		},
	}
	if err := store.AppendSleepObservation(context.Background(), record); err != nil {
		return SleepEntryDTO{}, err
	}
	return a.sleepEntryByID(record.ObservationID)
}

func (a *App) ListSleepEntries() (SleepEntriesDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepEntriesDTO{
			Status:  "unavailable",
			Empty:   true,
			Message: "Local storage is unavailable: " + err.Error(),
		}, nil
	}
	return a.listSleepEntriesWithStore(context.Background(), store)
}

func (a *App) ExportSleepData() (SleepDataExportDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepDataExportDTO{}, err
	}
	exported, err := store.ExportSleepData(context.Background())
	if err != nil {
		return SleepDataExportDTO{}, err
	}
	encoded, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		return SleepDataExportDTO{}, err
	}
	return SleepDataExportDTO{
		FileName:         "zeitboard-sleep-export-" + exported.GeneratedAt.Format("20060102-150405") + ".json",
		JSON:             string(encoded),
		GeneratedLabel:   exported.GeneratedAt.Local().Format("Jan 2, 2006, 3:04 PM"),
		ObservationCount: len(exported.ObservationSet.Observations),
		CorrectionCount:  len(exported.CorrectionSet.Corrections),
	}, nil
}

func (a *App) DeleteSleepObservation(input SleepDeleteInput) (SleepEntriesDTO, error) {
	if err := requireDeleteConfirmation(input.Confirmation); err != nil {
		return SleepEntriesDTO{}, err
	}
	store, err := a.requireStore()
	if err != nil {
		return SleepEntriesDTO{}, err
	}
	if err := store.DeleteSleepObservation(context.Background(), input.ObservationID); err != nil {
		return SleepEntriesDTO{}, err
	}
	return a.listSleepEntriesWithStore(context.Background(), store)
}

func (a *App) DeleteAllSleepData(input SleepDeleteAllInput) (SleepEntriesDTO, error) {
	if err := requireDeleteConfirmation(input.Confirmation); err != nil {
		return SleepEntriesDTO{}, err
	}
	store, err := a.requireStore()
	if err != nil {
		return SleepEntriesDTO{}, err
	}
	if err := store.DeleteAllSleepData(context.Background()); err != nil {
		return SleepEntriesDTO{}, err
	}
	return a.listSleepEntriesWithStore(context.Background(), store)
}

func (a *App) CorrectSleepEntry(input SleepCorrectionInput) (SleepEntryDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepEntryDTO{}, err
	}
	start, end, _, classification, err := parseSleepInput(SleepEntryInput{
		StartLocal:     input.StartLocal,
		EndLocal:       input.EndLocal,
		ZoneID:         input.ZoneID,
		Classification: input.Classification,
	})
	if err != nil {
		return SleepEntryDTO{}, err
	}
	supersedes, err := store.LatestSleepCorrectionID(context.Background(), input.ObservationID)
	if err != nil {
		return SleepEntryDTO{}, err
	}
	record := storage.SleepCorrectionRecord{
		CorrectionID:           newLocalID("corr_sleep"),
		TargetObservationID:    input.ObservationID,
		SupersedesCorrectionID: supersedes,
		CreatedAt:              time.Now().UTC(),
		Reason:                 storage.CorrectionReasonUserEdit,
		Changes: storage.SleepCorrectionChanges{
			StartAt:             &start,
			EndAt:               &end,
			SleepClassification: &classification,
		},
	}
	if err := store.AppendSleepCorrection(context.Background(), record); err != nil {
		return SleepEntryDTO{}, err
	}
	return a.sleepEntryByID(input.ObservationID)
}

func (a *App) SuppressSleepEntry(input SleepSuppressInput) (SleepEntryDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepEntryDTO{}, err
	}
	entries, err := a.listSleepEntriesWithStore(context.Background(), store)
	if err != nil {
		return SleepEntryDTO{}, err
	}
	var current *SleepEntryDTO
	for i := range entries.Entries {
		if entries.Entries[i].ObservationID == input.ObservationID {
			current = &entries.Entries[i]
			break
		}
	}
	if current == nil {
		return SleepEntryDTO{}, fmt.Errorf("sleep entry %s not found", input.ObservationID)
	}
	start, end, _, classification, err := parseSleepInput(SleepEntryInput{
		StartLocal:     current.EffectiveStartLocal,
		EndLocal:       current.EffectiveEndLocal,
		ZoneID:         current.ZoneID,
		Classification: current.EffectiveClassification,
	})
	if err != nil {
		return SleepEntryDTO{}, err
	}
	supersedes, err := store.LatestSleepCorrectionID(context.Background(), input.ObservationID)
	if err != nil {
		return SleepEntryDTO{}, err
	}
	excluded := true
	record := storage.SleepCorrectionRecord{
		CorrectionID:           newLocalID("corr_sleep"),
		TargetObservationID:    input.ObservationID,
		SupersedesCorrectionID: supersedes,
		CreatedAt:              time.Now().UTC(),
		Reason:                 storage.CorrectionReasonUserEdit,
		Changes: storage.SleepCorrectionChanges{
			StartAt:             &start,
			EndAt:               &end,
			SleepClassification: &classification,
			Excluded:            &excluded,
		},
	}
	if err := store.AppendSleepCorrection(context.Background(), record); err != nil {
		return SleepEntryDTO{}, err
	}
	return a.sleepEntryByID(input.ObservationID)
}

func (a *App) GetOverview() (OverviewDTO, error) {
	now := time.Now().UTC().Truncate(time.Minute)
	if overview, ok := a.serverOverview(context.Background(), now); ok {
		return overview, nil
	}
	state, err := a.localEstimate(context.Background(), now)
	if err != nil {
		return OverviewDTO{}, err
	}
	if state.Status != "estimated" {
		return overviewUnavailable(state, now), nil
	}
	latest, ok := latestPrincipalSession(state.Sessions)
	if !ok {
		return overviewUnavailable(localEstimateState{
			Status:  "refused",
			Message: "Need at least seven usable principal sleep entries before estimating rhythm.",
			Refusal: &estimation.EstimationRefusal{
				Code:    estimation.RefusalInsufficientData,
				Message: "need at least 7 usable principal sleep episodes",
			},
		}, now), nil
	}
	interval := latest.Intervals[0].Interval
	lastWake := interval.End
	currentState := "Likely awake"
	timeSinceWake := "Not available"
	if interval.Contains(now) {
		currentState = "Likely asleep"
		timeSinceWake = "Sleep entry overlaps the current time"
	} else if now.After(lastWake.UTC) {
		timeSinceWake = formatDuration(now.Sub(lastWake.UTC))
	}
	nextSleep := state.Estimate.PredictedSleepWindows[0].Interval
	currentAvailability := domain.AvailabilityWindow{
		ID:         "current-functional-window",
		Kind:       domain.AvailabilityFunctional,
		Interval:   domain.TimeRange{Start: domain.MustZonedInstant(now, state.Estimate.AsOf.ZoneID), End: nextSleep.Start},
		Confidence: state.Estimate.Confidence,
		EstimateID: state.Estimate.ID,
	}
	usefulWindow := "No reliable proposal"
	if currentAvailability.Interval.End.UTC.After(currentAvailability.Interval.Start.UTC) {
		task := domain.FlexibleTask{
			ID:                "local-flexible-task",
			Title:             "Flexible task",
			EstimatedDuration: 45 * time.Minute,
			Constraint:        domain.TaskConstraint{MinimumConfidence: domain.ConfidenceLow, RequiresApproval: true},
		}
		wakeAnchor := domain.WakeAnchor{
			ID:         "latest-wake",
			At:         lastWake,
			Confidence: state.Estimate.Confidence,
		}
		store, storeErr := a.requireStore()
		if storeErr != nil {
			return OverviewDTO{}, storeErr
		}
		fixedEvents, _, eventsErr := store.BusyDomainEvents(
			context.Background(), currentAvailability.Interval.Start.UTC,
			currentAvailability.Interval.End.UTC, state.Estimate.AsOf.ZoneID,
		)
		if eventsErr != nil {
			return OverviewDTO{}, eventsErr
		}
		proposal, proposalErr := (scheduling.Scheduler{}).Propose(scheduling.Request{
			Task: task, Availability: []domain.AvailabilityWindow{currentAvailability}, Events: fixedEvents, WakeAnchor: &wakeAnchor, Now: now,
		})
		if proposalErr == nil {
			usefulWindow = formatRange(proposal.Window)
		}
	}
	return OverviewDTO{
		EstimateSource:           "local",
		Status:                   "estimated",
		CurrentEstimatedState:    currentState,
		TimeSinceWake:            timeSinceWake,
		PredictedNextSleepWindow: formatRange(nextSleep),
		DriftEstimate:            fmt.Sprintf("%+.0f minutes per observed sleep cycle", state.Estimate.ObservedDriftPerCycle.Minutes()),
		Confidence:               string(state.Estimate.Confidence.Level),
		ConfidenceReasons:        state.Estimate.Confidence.Reasons,
		NextUsefulTaskWindow:     usefulWindow,
		SharingStatus:            "No active trusted view; local data only",
		MedicationEvents:         []MedicationEventDTO{},
		FixtureMode:              false,
		Disclaimer:               disclaimer,
		UpdatedLabel:             "Updated from local sleep entries just now",
	}, nil
}

func (a *App) GetRhythm() (estimation.RhythmProjection, error) {
	now := time.Now().UTC().Truncate(time.Minute)
	if rhythm, ok := a.serverRhythm(context.Background(), now); ok {
		return rhythm, nil
	}
	state, err := a.localEstimate(context.Background(), now)
	if err != nil {
		return estimation.RhythmProjection{}, err
	}
	if state.Status != "estimated" {
		return emptyRhythmProjection(state, now), nil
	}
	projection, err := (estimation.RobustEstimator{}).Project(context.Background(), state.Sessions, now)
	if err != nil {
		return estimation.RhythmProjection{}, err
	}
	projection.FixtureMode = false
	projection.Status = "estimated"
	projection.EstimateSource = "local"
	return projection, nil
}

type TaskInput struct {
	TaskID                    string `json:"taskId,omitempty"`
	Title                     string `json:"title"`
	DurationMinutes           int    `json:"durationMinutes"`
	EarliestStartLocal        string `json:"earliestStartLocal,omitempty"`
	LatestFinishLocal         string `json:"latestFinishLocal,omitempty"`
	ZoneID                    string `json:"zoneId,omitempty"`
	PreferredAfterWakeMinutes int    `json:"preferredAfterWakeMinutes,omitempty"`
	MinimumConfidence         string `json:"minimumConfidence,omitempty"`
}

type TaskDTO struct {
	TaskID          string `json:"taskId"`
	Title           string `json:"title"`
	DurationMinutes int    `json:"durationMinutes"`
	DurationLabel   string `json:"durationLabel"`
	Status          string `json:"status"`
	WindowLabel     string `json:"windowLabel,omitempty"`
	AfterWakeLabel  string `json:"afterWakeLabel,omitempty"`
	CreatedLabel    string `json:"createdLabel"`
}

type TasksDTO struct {
	Status  string    `json:"status"`
	Message string    `json:"message,omitempty"`
	Tasks   []TaskDTO `json:"tasks"`
}

type TaskActionInput struct {
	TaskID string `json:"taskId"`
	Done   bool   `json:"done,omitempty"`
}

// AddTask stores a user-owned flexible task. Tasks are planning items the
// scheduler may propose windows for; every placement still requires approval.
func (a *App) AddTask(input TaskInput) (TasksDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return TasksDTO{}, err
	}
	record, err := taskRecordFromInput(input, newLocalID("task"), time.Now().UTC(), storage.TaskStatusOpen)
	if err != nil {
		return TasksDTO{}, err
	}
	if err := store.AddTask(context.Background(), record); err != nil {
		return TasksDTO{}, err
	}
	return a.ListTasks()
}

func (a *App) ListTasks() (TasksDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return TasksDTO{Status: "unavailable", Message: "Task planning needs the desktop app service.", Tasks: []TaskDTO{}}, nil
	}
	records, err := store.ListTasks(context.Background())
	if err != nil {
		return TasksDTO{}, err
	}
	tasks := make([]TaskDTO, 0, len(records))
	for _, record := range records {
		tasks = append(tasks, taskDTO(record))
	}
	return TasksDTO{Status: "ok", Tasks: tasks}, nil
}

// UpdateTask replaces an existing task's fields (status is preserved).
func (a *App) UpdateTask(input TaskInput) (TasksDTO, error) {
	if input.TaskID == "" {
		return TasksDTO{}, errors.New("taskId is required")
	}
	store, err := a.requireStore()
	if err != nil {
		return TasksDTO{}, err
	}
	existing, err := store.ListTasks(context.Background())
	if err != nil {
		return TasksDTO{}, err
	}
	status := storage.TaskStatusOpen
	created := time.Now().UTC()
	found := false
	for _, record := range existing {
		if record.TaskID == input.TaskID {
			status = record.Status
			created = record.CreatedAt
			found = true
			break
		}
	}
	if !found {
		return TasksDTO{}, storage.ErrTaskNotFound
	}
	record, err := taskRecordFromInput(input, input.TaskID, created, status)
	if err != nil {
		return TasksDTO{}, err
	}
	if err := store.UpdateTask(context.Background(), record); err != nil {
		return TasksDTO{}, err
	}
	return a.ListTasks()
}

func (a *App) SetTaskDone(input TaskActionInput) (TasksDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return TasksDTO{}, err
	}
	status := storage.TaskStatusOpen
	if input.Done {
		status = storage.TaskStatusDone
	}
	if err := store.SetTaskStatus(context.Background(), input.TaskID, status); err != nil {
		return TasksDTO{}, err
	}
	return a.ListTasks()
}

func (a *App) DeleteTask(input TaskActionInput) (TasksDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return TasksDTO{}, err
	}
	if err := store.DeleteTask(context.Background(), input.TaskID); err != nil {
		return TasksDTO{}, err
	}
	return a.ListTasks()
}

func taskRecordFromInput(input TaskInput, taskID string, createdAt time.Time, status string) (storage.TaskRecord, error) {
	zoneID := strings.TrimSpace(input.ZoneID)
	if zoneID == "" {
		zoneID = defaultZoneID
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return storage.TaskRecord{}, fmt.Errorf("load time zone %q: %w", zoneID, err)
	}
	record := storage.TaskRecord{
		TaskID:            taskID,
		Title:             strings.TrimSpace(input.Title),
		DurationMinutes:   input.DurationMinutes,
		Status:            status,
		CreatedAt:         createdAt,
		MinimumConfidence: strings.TrimSpace(input.MinimumConfidence),
	}
	if input.EarliestStartLocal != "" {
		parsed, err := parseCivilTime(input.EarliestStartLocal, location)
		if err != nil {
			return storage.TaskRecord{}, fmt.Errorf("earliest start: %w", err)
		}
		utc := parsed.UTC()
		record.EarliestStartAt = &utc
	}
	if input.LatestFinishLocal != "" {
		parsed, err := parseCivilTime(input.LatestFinishLocal, location)
		if err != nil {
			return storage.TaskRecord{}, fmt.Errorf("latest finish: %w", err)
		}
		utc := parsed.UTC()
		record.LatestFinishAt = &utc
	}
	if input.PreferredAfterWakeMinutes > 0 {
		value := input.PreferredAfterWakeMinutes
		record.PreferredAfterWakeMinutes = &value
	}
	return record, nil
}

func taskDTO(record storage.TaskRecord) TaskDTO {
	dto := TaskDTO{
		TaskID:          record.TaskID,
		Title:           record.Title,
		DurationMinutes: record.DurationMinutes,
		DurationLabel:   formatDuration(time.Duration(record.DurationMinutes) * time.Minute),
		Status:          record.Status,
		CreatedLabel:    "Added " + record.CreatedAt.Local().Format("Jan 2"),
	}
	switch {
	case record.EarliestStartAt != nil && record.LatestFinishAt != nil:
		dto.WindowLabel = "Between " + record.EarliestStartAt.Local().Format("Jan 2, 3:04 PM") +
			" and " + record.LatestFinishAt.Local().Format("Jan 2, 3:04 PM")
	case record.EarliestStartAt != nil:
		dto.WindowLabel = "Not before " + record.EarliestStartAt.Local().Format("Jan 2, 3:04 PM")
	case record.LatestFinishAt != nil:
		dto.WindowLabel = "Finish by " + record.LatestFinishAt.Local().Format("Jan 2, 3:04 PM")
	}
	if record.PreferredAfterWakeMinutes != nil {
		dto.AfterWakeLabel = fmt.Sprintf("At least %d min after waking", *record.PreferredAfterWakeMinutes)
	}
	return dto
}

func (a *App) requireStore() (*storage.Store, error) {
	if a.storeErr != nil {
		return nil, a.storeErr
	}
	if a.store == nil {
		return nil, errors.New("local store is not open")
	}
	return a.store, nil
}

func (a *App) localEstimate(ctx context.Context, now time.Time) (localEstimateState, error) {
	store, err := a.requireStore()
	if err != nil {
		return localEstimateState{Status: "unavailable", Message: err.Error()}, nil
	}
	sessions, err := store.EffectiveSleepSessions(ctx)
	if err != nil {
		return localEstimateState{}, err
	}
	if len(sessions) == 0 {
		return localEstimateState{
			Status:   "empty",
			Message:  "Add your first sleep entry to start a local estimate.",
			Sessions: sessions,
		}, nil
	}
	estimate, err := (estimation.RobustEstimator{}).Estimate(ctx, sessions, now)
	if err != nil {
		var refusal *estimation.EstimationRefusal
		if errors.As(err, &refusal) {
			return localEstimateState{
				Status:   "refused",
				Message:  refusal.Message,
				Sessions: sessions,
				Refusal:  refusal,
			}, nil
		}
		return localEstimateState{}, err
	}
	return localEstimateState{Status: "estimated", Sessions: sessions, Estimate: estimate}, nil
}

func overviewUnavailable(state localEstimateState, now time.Time) OverviewDTO {
	title := "No sleep entries yet"
	message := state.Message
	if message == "" {
		message = "Add at least seven principal sleep episodes before the app estimates a rhythm."
	}
	if state.Status == "refused" {
		title = "Need more sleep data"
	} else if state.Status == "unavailable" {
		title = "Local storage unavailable"
	}
	return OverviewDTO{
		EstimateSource:           "local",
		Status:                   state.Status,
		Empty:                    state.Status == "empty",
		Refusal:                  refusalDTO(state.Refusal, message),
		CurrentEstimatedState:    title,
		TimeSinceWake:            "Not available",
		PredictedNextSleepWindow: "Not enough local data",
		DriftEstimate:            "Not enough local data",
		Confidence:               string(domain.ConfidenceLow),
		ConfidenceReasons:        []string{message},
		NextUsefulTaskWindow:     "No reliable proposal",
		SharingStatus:            "No active trusted view; local data only",
		MedicationEvents:         []MedicationEventDTO{},
		FixtureMode:              false,
		Disclaimer:               disclaimer,
		UpdatedLabel:             now.Local().Format("Updated Jan 2, 3:04 PM"),
	}
}

func emptyRhythmProjection(state localEstimateState, now time.Time) estimation.RhythmProjection {
	message := state.Message
	if message == "" {
		message = "Add at least seven principal sleep episodes before the app estimates a rhythm."
	}
	localNow := now.In(locationOrUTC(defaultZoneID))
	return estimation.RhythmProjection{
		FixtureMode:     false,
		EstimateSource:  "local",
		Status:          state.Status,
		Refusal:         state.Refusal,
		ActogramSummary: message,
		ObservedRows:    []estimation.RhythmBand{},
		ForecastRows:    []estimation.RhythmBand{},
		Now: estimation.RhythmNow{
			Label:     "now",
			Day:       localNow.Format("Jan 2"),
			CivilDate: localNow.Format("2006-01-02"),
			ZoneID:    defaultZoneID,
			Hour:      localClockHour(localNow),
		},
		DriftTitle:      "Sleep-onset drift",
		SlopeLabel:      "Not enough data",
		DriftConfidence: "Low",
		DriftSummary:    message,
		YMinHour:        0,
		YMaxHour:        24,
		DriftPoints:     []estimation.RhythmDriftPoint{},
	}
}

func refusalDTO(refusal *estimation.EstimationRefusal, fallback string) *RefusalDTO {
	if refusal == nil {
		if fallback == "" {
			return nil
		}
		return &RefusalDTO{Code: "estimate_unavailable", Message: fallback}
	}
	return &RefusalDTO{Code: string(refusal.Code), Message: refusal.Message}
}

func (a *App) listSleepEntriesWithStore(ctx context.Context, store *storage.Store) (SleepEntriesDTO, error) {
	observations, err := store.ListSleepObservations(ctx)
	if err != nil {
		return SleepEntriesDTO{}, err
	}
	if len(observations) == 0 {
		return SleepEntriesDTO{
			Status:  "empty",
			Empty:   true,
			Message: "No sleep entries yet. Add a sleep interval to start a local estimate.",
			Entries: []SleepEntryDTO{},
		}, nil
	}
	rawSessions, err := store.RawSleepSessions(ctx)
	if err != nil {
		return SleepEntriesDTO{}, err
	}
	correctedSessions, err := store.CorrectedSleepSessions(ctx)
	if err != nil {
		return SleepEntriesDTO{}, err
	}
	corrections, err := store.ListSleepCorrections(ctx)
	if err != nil {
		return SleepEntriesDTO{}, err
	}
	rawByID := map[string]domain.SleepSession{}
	for _, session := range rawSessions {
		rawByID[string(session.ID)] = session
	}
	correctedByID := map[string]domain.SleepSession{}
	for _, session := range correctedSessions {
		correctedByID[string(session.ID)] = session
	}
	history := map[string][]storage.SleepCorrectionRecord{}
	for _, correction := range corrections {
		history[correction.TargetObservationID] = append(history[correction.TargetObservationID], correction)
	}
	sort.Slice(observations, func(i, j int) bool {
		return observations[i].StartAt.After(observations[j].StartAt)
	})
	entries := make([]SleepEntryDTO, 0, len(observations))
	for _, observation := range observations {
		rawSession := rawByID[observation.ObservationID]
		correctedSession := correctedByID[observation.ObservationID]
		if len(correctedSession.Intervals) == 0 {
			correctedSession = rawSession
		}
		entries = append(entries, sleepEntryDTO(observation, rawSession, correctedSession, history[observation.ObservationID]))
	}
	return SleepEntriesDTO{
		Status:  "ready",
		Empty:   false,
		Message: fmt.Sprintf("%d local sleep %s stored on this device.", len(entries), plural(len(entries), "entry", "entries")),
		Entries: entries,
	}, nil
}

func (a *App) sleepEntryByID(observationID string) (SleepEntryDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepEntryDTO{}, err
	}
	entries, err := a.listSleepEntriesWithStore(context.Background(), store)
	if err != nil {
		return SleepEntryDTO{}, err
	}
	for _, entry := range entries.Entries {
		if entry.ObservationID == observationID {
			return entry, nil
		}
	}
	return SleepEntryDTO{}, fmt.Errorf("sleep entry %s not found", observationID)
}

func sleepEntryDTO(observation storage.SleepObservationRecord, rawSession, correctedSession domain.SleepSession, corrections []storage.SleepCorrectionRecord) SleepEntryDTO {
	rawInterval := rawSession.Intervals[0].Interval
	effectiveInterval := correctedSession.Intervals[0].Interval
	sort.Slice(corrections, func(i, j int) bool {
		return corrections[i].CreatedAt.After(corrections[j].CreatedAt)
	})
	history := make([]SleepCorrectionDTO, 0, len(corrections))
	for _, correction := range corrections {
		history = append(history, correctionDTO(correction, observation.ZoneID))
	}
	return SleepEntryDTO{
		ObservationID:           observation.ObservationID,
		StartLocal:              inputValue(rawInterval.Start),
		EndLocal:                inputValue(rawInterval.End),
		StartLabel:              formatInstant(rawInterval.Start),
		EndLabel:                formatInstant(rawInterval.End),
		ZoneID:                  observation.ZoneID,
		Classification:          observation.Sleep.Classification,
		EffectiveStartLocal:     inputValue(effectiveInterval.Start),
		EffectiveEndLocal:       inputValue(effectiveInterval.End),
		EffectiveStartLabel:     formatInstant(effectiveInterval.Start),
		EffectiveEndLabel:       formatInstant(effectiveInterval.End),
		EffectiveClassification: classificationFromSession(correctedSession),
		DurationLabel:           formatDuration(effectiveInterval.Duration()),
		Suppressed:              correctedSession.Suppressed,
		SourceLabel:             correctedSession.SourceLabel,
		ProvenanceLabel:         provenanceLabel(observation.Provenance),
		History:                 history,
	}
}

func correctionDTO(correction storage.SleepCorrectionRecord, zoneID string) SleepCorrectionDTO {
	return SleepCorrectionDTO{
		CorrectionID:           correction.CorrectionID,
		SupersedesCorrectionID: correction.SupersedesCorrectionID,
		CreatedLabel:           correction.CreatedAt.In(locationOrUTC(zoneID)).Format("Jan 2, 3:04 PM"),
		Reason:                 strings.ReplaceAll(correction.Reason, "_", " "),
		Summary:                correctionSummary(correction.Changes, zoneID),
	}
}

func correctionSummary(changes storage.SleepCorrectionChanges, zoneID string) string {
	var parts []string
	location := locationOrUTC(zoneID)
	if changes.StartAt != nil {
		parts = append(parts, "start "+changes.StartAt.In(location).Format("Jan 2, 3:04 PM"))
	}
	if changes.EndAt != nil {
		parts = append(parts, "wake "+changes.EndAt.In(location).Format("Jan 2, 3:04 PM"))
	}
	if changes.SleepClassification != nil {
		parts = append(parts, "classification "+*changes.SleepClassification)
	}
	if changes.Excluded != nil && *changes.Excluded {
		parts = append(parts, "excluded from estimates")
	}
	if len(parts) == 0 {
		return "No visible changes"
	}
	return strings.Join(parts, "; ")
}

func parseSleepInput(input SleepEntryInput) (time.Time, time.Time, string, string, error) {
	zoneID := strings.TrimSpace(input.ZoneID)
	if zoneID == "" {
		zoneID = defaultZoneID
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", fmt.Errorf("load time zone %q: %w", zoneID, err)
	}
	classification := strings.TrimSpace(input.Classification)
	if classification == "" {
		classification = storage.SleepClassificationPrincipal
	}
	if !validInputClassification(classification) {
		return time.Time{}, time.Time{}, "", "", errors.New("classification must be principal or nap")
	}
	start, err := parseCivilTime(strings.TrimSpace(input.StartLocal), location)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", fmt.Errorf("sleep start: %w", err)
	}
	end, err := parseCivilTime(strings.TrimSpace(input.EndLocal), location)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", fmt.Errorf("wake time: %w", err)
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, "", "", errors.New("wake time must be after sleep start")
	}
	return start.UTC(), end.UTC(), zoneID, classification, nil
}

func parseCivilTime(value string, location *time.Location) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("civil time is required")
	}
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, nil
		}
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	return time.Time{}, errors.New("use a local date and time")
}

func validInputClassification(value string) bool {
	return value == storage.SleepClassificationPrincipal || value == storage.SleepClassificationNap
}

func requireDeleteConfirmation(value string) error {
	if value != deleteConfirm {
		return errors.New("type DELETE to confirm permanent erasure")
	}
	return nil
}

// localPlanningAvailability builds the availability windows the scheduler and
// the assistant context share: the current functional window (now until the
// next predicted sleep) followed by the predicted waking windows.
func localPlanningAvailability(state localEstimateState, now time.Time) []domain.AvailabilityWindow {
	zoneID := state.Estimate.AsOf.ZoneID
	availability := append([]domain.AvailabilityWindow{}, state.Estimate.PredictedWakingWindows...)
	if len(state.Estimate.PredictedSleepWindows) > 0 {
		nextSleep := state.Estimate.PredictedSleepWindows[0].Interval
		current := domain.AvailabilityWindow{
			ID:         "current-functional-window",
			Kind:       domain.AvailabilityFunctional,
			Interval:   domain.TimeRange{Start: domain.MustZonedInstant(now, zoneID), End: nextSleep.Start},
			Confidence: state.Estimate.Confidence,
			EstimateID: state.Estimate.ID,
		}
		if current.Interval.End.UTC.After(current.Interval.Start.UTC) {
			availability = append([]domain.AvailabilityWindow{current}, availability...)
		}
	}
	return availability
}

func latestPrincipalSession(sessions []domain.SleepSession) (domain.SleepSession, bool) {
	for i := len(sessions) - 1; i >= 0; i-- {
		if sessions[i].Suppressed || sessions[i].IsNap || len(sessions[i].Intervals) == 0 {
			continue
		}
		return sessions[i], true
	}
	return domain.SleepSession{}, false
}

func unplacedForUnavailableEstimate(tasks []domain.FlexibleTask) []UnplacedDTO {
	result := make([]UnplacedDTO, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, UnplacedDTO{
			Title:      task.Title,
			Reason:     unplacedReasonLabel(scheduling.ReasonEstimateUnavailable),
			ReasonCode: string(scheduling.ReasonEstimateUnavailable),
			NextAction: "Add at least seven principal sleep entries before planning.",
		})
	}
	return result
}

func inputValue(value domain.ZonedInstant) string {
	local, err := value.InLocation()
	if err != nil {
		local = value.UTC
	}
	return local.Format("2006-01-02T15:04")
}

func formatInstant(value domain.ZonedInstant) string {
	local, err := value.InLocation()
	if err != nil {
		return value.UTC.Format(time.RFC3339)
	}
	return local.Format("Mon Jan 2, 3:04 PM MST")
}

func formatRange(value domain.TimeRange) string {
	return formatInstant(value.Start) + " to " + formatInstant(value.End)
}

func formatDuration(value time.Duration) string {
	if value < 0 {
		value = -value
	}
	hours := int(value.Hours())
	minutes := int(value.Minutes()) % 60
	if hours == 0 {
		return fmt.Sprintf("%d minutes", minutes)
	}
	return fmt.Sprintf("%d hours %d minutes", hours, minutes)
}

func rhythmContext(proposal scheduling.Proposal, availability []domain.AvailabilityWindow) string {
	for _, window := range availability {
		if window.Interval.Contains(proposal.Window.Start.UTC) {
			offset := proposal.Window.Start.UTC.Sub(window.Interval.Start.UTC)
			if offset < 5*time.Minute {
				return "at the start of a predicted waking window"
			}
			return "about " + formatDuration(offset) + " into a predicted waking window"
		}
	}
	return "within a predicted waking window"
}

func confidenceTitle(level domain.ConfidenceLevel) string {
	switch level {
	case domain.ConfidenceHigh:
		return "High"
	case domain.ConfidenceMedium:
		return "Medium"
	default:
		return "Low"
	}
}

func reasonLabels(codes []string) []string {
	labels := make([]string, 0, len(codes))
	for _, code := range codes {
		switch code {
		case scheduling.CodeWithinPredictedWakingWindow:
			labels = append(labels, "In a predicted waking window")
		case scheduling.CodeAvoidsFixedEvent:
			labels = append(labels, "Avoids a fixed event")
		case scheduling.CodeWithinTaskBounds:
			labels = append(labels, "Within the task's time bounds")
		case scheduling.CodeUncertaintyBufferApplied:
			labels = append(labels, "Kept a buffer from window edges")
		default:
			labels = append(labels, code)
		}
	}
	return labels
}

func unplacedReasonLabel(reason scheduling.UnplacedReason) string {
	switch reason {
	case scheduling.ReasonNoAvailableInterval:
		return "No open window fits before its limits"
	case scheduling.ReasonOutsideForecastHorizon:
		return "Falls outside the forecast horizon"
	case scheduling.ReasonEstimateUnavailable:
		return "No current estimate to plan against"
	default:
		return "The task constraints conflict"
	}
}

func classificationFromSession(session domain.SleepSession) string {
	if session.IsNap {
		return storage.SleepClassificationNap
	}
	return storage.SleepClassificationPrincipal
}

func provenanceLabel(provenance storage.SleepObservationProvenance) string {
	method := strings.ReplaceAll(provenance.AcquisitionMethod, "_", " ")
	status := strings.ReplaceAll(provenance.EvidenceStatus, "_", " ")
	return method + " / " + status
}

func locationOrUTC(zoneID string) *time.Location {
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return time.UTC
	}
	return location
}

func localClockHour(value time.Time) float64 {
	return float64(value.Hour()) + float64(value.Minute())/60 + float64(value.Second())/3600
}

func plural(count int, singular, pluralValue string) string {
	if count == 1 {
		return singular
	}
	return pluralValue
}

func newLocalID(prefix string) string {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err == nil {
		return prefix + "_" + hex.EncodeToString(random)
	}
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}

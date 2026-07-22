package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"non24.app/core/domain"
	medicationcore "non24.app/core/medication"
	storage "non24.app/core/storage/sqlite"
)

const medicationInteractionDisclaimer = "ZeitBoard records what you enter. It does not check medication interactions; ask a pharmacist or clinician."

type MedicationInput struct {
	Label         string `json:"label"`
	Form          string `json:"form"`
	StrengthLabel string `json:"strengthLabel"`
}

type MedicationUpdateInput struct {
	MedicationID  string `json:"medicationId"`
	Revision      int    `json:"revision"`
	Label         string `json:"label"`
	Form          string `json:"form"`
	StrengthLabel string `json:"strengthLabel"`
	Active        bool   `json:"active"`
}

type MedicationScheduleInput struct {
	MedicationID    string   `json:"medicationId"`
	Revision        int      `json:"revision"`
	Kind            string   `json:"kind"`
	ZoneID          string   `json:"zoneId"`
	CivilTimes      []string `json:"civilTimes"`
	DaysOn          int      `json:"daysOn"`
	DaysOff         int      `json:"daysOff"`
	CycleStartedOn  string   `json:"cycleStartedOn"`
	ReminderEnabled bool     `json:"reminderEnabled"`
	ClinicianRule   string   `json:"clinicianRule"`
}

type MedicationEventInput struct {
	MedicationID string `json:"medicationId"`
	DoseLocal    string `json:"doseLocal"`
	ZoneID       string `json:"zoneId"`
	Status       string `json:"status"`
	Scheduled    bool   `json:"scheduled"`
	Note         string `json:"note"`
}

type MedicationEventCorrectionInput struct {
	EventID   string `json:"eventId"`
	DoseLocal string `json:"doseLocal"`
	ZoneID    string `json:"zoneId"`
	Status    string `json:"status"`
	Scheduled bool   `json:"scheduled"`
	Note      string `json:"note"`
	Excluded  bool   `json:"excluded"`
}

type MedicationDeleteInput struct {
	MedicationID string `json:"medicationId"`
	Confirmation string `json:"confirmation"`
}

type MedicationEventDeleteInput struct {
	EventID      string `json:"eventId"`
	Confirmation string `json:"confirmation"`
}

type MedicationDTO struct {
	MedicationID             string                 `json:"medicationId"`
	Label                    string                 `json:"label"`
	Form                     string                 `json:"form,omitempty"`
	StrengthLabel            string                 `json:"strengthLabel,omitempty"`
	DetailLabel              string                 `json:"detailLabel"`
	ClinicianRule            string                 `json:"clinicianRule,omitempty"`
	ClinicianRuleAttribution string                 `json:"clinicianRuleAttribution,omitempty"`
	Active                   bool                   `json:"active"`
	Revision                 int                    `json:"revision"`
	ScheduleKind             string                 `json:"scheduleKind"`
	Schedule                 *MedicationScheduleDTO `json:"schedule,omitempty"`
	CreatedLabel             string                 `json:"createdLabel"`
	EventCount               int                    `json:"eventCount"`
}

type MedicationScheduleDTO struct {
	Kind            string                        `json:"kind"`
	ZoneID          string                        `json:"zoneId,omitempty"`
	CivilTimes      []string                      `json:"civilTimes"`
	DaysOn          int                           `json:"daysOn,omitempty"`
	DaysOff         int                           `json:"daysOff,omitempty"`
	CycleStartedOn  string                        `json:"cycleStartedOn,omitempty"`
	ReminderEnabled bool                          `json:"reminderEnabled"`
	Summary         string                        `json:"summary"`
	Forecast        MedicationScheduleForecastDTO `json:"forecast"`
}

type MedicationScheduleForecastDTO struct {
	Status              string                            `json:"status"`
	Message             string                            `json:"message"`
	CoveredCount        int                               `json:"coveredCount"`
	CollisionCount      int                               `json:"collisionCount"`
	OutsideHorizonCount int                               `json:"outsideHorizonCount"`
	CoverageEndsAt      string                            `json:"coverageEndsAt,omitempty"`
	CoverageLabel       string                            `json:"coverageLabel,omitempty"`
	Occurrences         []MedicationScheduleOccurrenceDTO `json:"occurrences"`
	Gaps                []MedicationScheduleGapDTO        `json:"gaps"`
}

type MedicationScheduleOccurrenceDTO struct {
	At         string `json:"at"`
	CivilDate  string `json:"civilDate"`
	CivilTime  string `json:"civilTime"`
	CivilLabel string `json:"civilLabel"`
	Status     string `json:"status"`
	Context    string `json:"context"`
	Confidence string `json:"confidence"`
	Ambiguous  bool   `json:"ambiguous"`
	DSTNote    string `json:"dstNote,omitempty"`
}

type MedicationScheduleGapDTO struct {
	CivilDate  string `json:"civilDate"`
	CivilTime  string `json:"civilTime"`
	CivilLabel string `json:"civilLabel"`
	Message    string `json:"message"`
}

type MedicationLogDTO struct {
	EventID           string `json:"eventId"`
	MedicationID      string `json:"medicationId"`
	MedicationLabel   string `json:"medicationLabel"`
	DoseLocal         string `json:"doseLocal"`
	CivilTime         string `json:"civilTime"`
	ZoneID            string `json:"zoneId"`
	Status            string `json:"status"`
	Scheduled         bool   `json:"scheduled"`
	Note              string `json:"note,omitempty"`
	RecordedLabel     string `json:"recordedLabel"`
	WakeRelation      string `json:"wakeRelation"`
	SleepRelation     string `json:"sleepRelation"`
	SleepRelationKind string `json:"sleepRelationKind"`
	Confidence        string `json:"confidence"`
	Excluded          bool   `json:"excluded"`
	CorrectionCount   int    `json:"correctionCount"`
}

type MedicationsDTO struct {
	Status                string             `json:"status"`
	Empty                 bool               `json:"empty"`
	Message               string             `json:"message"`
	EstimateStatus        string             `json:"estimateStatus"`
	EstimateMessage       string             `json:"estimateMessage"`
	Medications           []MedicationDTO    `json:"medications"`
	Events                []MedicationLogDTO `json:"events"`
	FixtureMode           bool               `json:"fixtureMode"`
	Disclaimer            string             `json:"disclaimer"`
	InteractionDisclaimer string             `json:"interactionDisclaimer"`
	ReminderStatus        string             `json:"reminderStatus"`
	ReminderMessage       string             `json:"reminderMessage"`
	UpdatedLabel          string             `json:"updatedLabel"`
}

type MedicationExportDTO struct {
	FileName        string `json:"fileName"`
	JSON            string `json:"json"`
	GeneratedAt     string `json:"generatedAt"`
	GeneratedLabel  string `json:"generatedLabel"`
	MedicationCount int    `json:"medicationCount"`
	EventCount      int    `json:"eventCount"`
}

func (a *App) GetMedications() (MedicationsDTO, error) {
	return a.medicationsAt(a.currentTime().UTC().Truncate(time.Second))
}

func (a *App) AddMedication(input MedicationInput) (MedicationsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return MedicationsDTO{}, err
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	record := storage.MedicationRecord{
		MedicationID:  newLocalID("med"),
		Label:         strings.TrimSpace(input.Label),
		Form:          strings.TrimSpace(input.Form),
		StrengthLabel: strings.TrimSpace(input.StrengthLabel),
		Active:        true,
		CreatedAt:     now,
		Revision:      1,
		UpdatedAt:     now,
	}
	if err := store.CreateMedication(context.Background(), record); err != nil {
		return MedicationsDTO{}, err
	}
	return a.medicationsAt(now)
}

func (a *App) UpdateMedication(input MedicationUpdateInput) (MedicationsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return MedicationsDTO{}, err
	}
	ctx := context.Background()
	current, err := store.MedicationByID(ctx, strings.TrimSpace(input.MedicationID))
	if err != nil {
		return MedicationsDTO{}, err
	}
	if current.Revision != input.Revision {
		return MedicationsDTO{}, storage.ErrMedicationRevisionConflict
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	if now.Before(current.UpdatedAt) {
		now = current.UpdatedAt
	}
	current.Label = strings.TrimSpace(input.Label)
	current.Form = strings.TrimSpace(input.Form)
	current.StrengthLabel = strings.TrimSpace(input.StrengthLabel)
	current.Active = input.Active
	current.Revision++
	current.UpdatedAt = now
	if err := store.UpdateMedication(ctx, current, input.Revision); err != nil {
		return MedicationsDTO{}, err
	}
	return a.medicationsAt(now)
}

func (a *App) UpdateMedicationSchedule(input MedicationScheduleInput) (MedicationsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return MedicationsDTO{}, err
	}
	ctx := context.Background()
	current, err := store.MedicationByID(ctx, strings.TrimSpace(input.MedicationID))
	if err != nil {
		return MedicationsDTO{}, err
	}
	if current.Revision != input.Revision {
		return MedicationsDTO{}, storage.ErrMedicationRevisionConflict
	}
	schedule, clinicianRule, err := medicationScheduleValues(input)
	if err != nil {
		return MedicationsDTO{}, err
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	if now.Before(current.UpdatedAt) {
		now = current.UpdatedAt
	}
	current.Schedule = schedule
	current.ClinicianRule = clinicianRule
	current.Revision++
	current.UpdatedAt = now
	if err := store.UpdateMedication(ctx, current, input.Revision); err != nil {
		return MedicationsDTO{}, err
	}
	return a.medicationsAt(now)
}

func medicationScheduleValues(input MedicationScheduleInput) (*storage.MedicationSchedule, string, error) {
	kind := strings.TrimSpace(input.Kind)
	clinicianRule := strings.TrimSpace(input.ClinicianRule)
	civilTimes := make([]string, len(input.CivilTimes))
	for index, civilTime := range input.CivilTimes {
		civilTimes[index] = strings.TrimSpace(civilTime)
	}
	if kind == "none" {
		if strings.TrimSpace(input.ZoneID) != "" || len(civilTimes) != 0 || input.DaysOn != 0 || input.DaysOff != 0 || strings.TrimSpace(input.CycleStartedOn) != "" || input.ReminderEnabled {
			return nil, "", errors.New("a removed schedule cannot retain clock, cycle, or reminder fields")
		}
		if len(clinicianRule) > 500 {
			return nil, "", errors.New("clinician rule must be 500 characters or fewer")
		}
		return nil, clinicianRule, nil
	}
	schedule := &storage.MedicationSchedule{
		Kind:            kind,
		ZoneID:          strings.TrimSpace(input.ZoneID),
		CivilTimes:      civilTimes,
		DaysOn:          input.DaysOn,
		DaysOff:         input.DaysOff,
		CycleStartedOn:  strings.TrimSpace(input.CycleStartedOn),
		ReminderEnabled: input.ReminderEnabled,
	}
	if err := schedule.Validate(); err != nil {
		return nil, "", err
	}
	if len(clinicianRule) > 500 {
		return nil, "", errors.New("clinician rule must be 500 characters or fewer")
	}
	return schedule, clinicianRule, nil
}

func (a *App) LogMedicationEvent(input MedicationEventInput) (MedicationsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return MedicationsDTO{}, err
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	doseAt, zoneID, status, note, err := medicationEventValues(input.DoseLocal, input.ZoneID, input.Status, input.Note, now)
	if err != nil {
		return MedicationsDTO{}, err
	}
	record := storage.MedicationEventRecord{
		EventID:      newLocalID("dose"),
		MedicationID: strings.TrimSpace(input.MedicationID),
		DoseAt:       doseAt,
		ZoneID:       zoneID,
		Status:       status,
		Scheduled:    input.Scheduled,
		Note:         note,
		Provenance: storage.SleepObservationProvenance{
			AcquisitionMethod: storage.ProvenanceAcquisitionManual,
			EvidenceStatus:    storage.ProvenanceEvidenceUserReported,
			RecordedAt:        now,
		},
	}
	if err := store.AppendMedicationEvent(context.Background(), record); err != nil {
		return MedicationsDTO{}, err
	}
	return a.medicationsAt(now)
}

func (a *App) CorrectMedicationEvent(input MedicationEventCorrectionInput) (MedicationsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return MedicationsDTO{}, err
	}
	ctx := context.Background()
	now := a.currentTime().UTC().Truncate(time.Second)
	doseAt, zoneID, status, note, err := medicationEventValues(input.DoseLocal, input.ZoneID, input.Status, input.Note, now)
	if err != nil {
		return MedicationsDTO{}, err
	}
	effective, err := store.EffectiveMedicationEvents(ctx)
	if err != nil {
		return MedicationsDTO{}, err
	}
	var target *storage.EffectiveMedicationEvent
	for index := range effective {
		if effective[index].Event.EventID == strings.TrimSpace(input.EventID) {
			target = &effective[index]
			break
		}
	}
	if target == nil {
		return MedicationsDTO{}, storage.ErrMedicationEventNotFound
	}
	changes := storage.MedicationEventCorrectionChanges{}
	if !target.Event.DoseAt.Equal(doseAt) {
		changes.DoseAt = &doseAt
	}
	if target.Event.ZoneID != zoneID {
		changes.ZoneID = &zoneID
	}
	if target.Event.Status != status {
		changes.Status = &status
	}
	if target.Event.Scheduled != input.Scheduled {
		changes.Scheduled = &input.Scheduled
	}
	if target.Event.Note != note {
		changes.Note = &note
	}
	if target.Excluded != input.Excluded {
		changes.Excluded = &input.Excluded
	}
	if changes == (storage.MedicationEventCorrectionChanges{}) {
		return MedicationsDTO{}, errors.New("medication event correction did not change any fields")
	}
	if !now.After(target.Event.Provenance.RecordedAt) {
		now = target.Event.Provenance.RecordedAt.Add(time.Nanosecond)
	}
	latest, err := store.LatestMedicationEventCorrectionID(ctx, target.Event.EventID)
	if err != nil {
		return MedicationsDTO{}, err
	}
	if len(target.Corrections) > 0 && !now.After(target.Corrections[len(target.Corrections)-1].CreatedAt) {
		now = target.Corrections[len(target.Corrections)-1].CreatedAt.Add(time.Nanosecond)
	}
	record := storage.MedicationEventCorrectionRecord{
		CorrectionID:           newLocalID("medcorr"),
		TargetEventID:          target.Event.EventID,
		SupersedesCorrectionID: latest,
		CreatedAt:              now,
		Reason:                 storage.MedicationCorrectionUserEdit,
		Changes:                changes,
	}
	if err := store.AppendMedicationEventCorrection(ctx, record); err != nil {
		return MedicationsDTO{}, err
	}
	return a.medicationsAt(now)
}

func (a *App) DeleteMedication(input MedicationDeleteInput) (MedicationsDTO, error) {
	if err := requireDeleteConfirmation(input.Confirmation); err != nil {
		return MedicationsDTO{}, err
	}
	store, err := a.requireStore()
	if err != nil {
		return MedicationsDTO{}, err
	}
	if err := store.DeleteMedication(context.Background(), strings.TrimSpace(input.MedicationID)); err != nil {
		return MedicationsDTO{}, err
	}
	return a.GetMedications()
}

func (a *App) DeleteMedicationEvent(input MedicationEventDeleteInput) (MedicationsDTO, error) {
	if err := requireDeleteConfirmation(input.Confirmation); err != nil {
		return MedicationsDTO{}, err
	}
	store, err := a.requireStore()
	if err != nil {
		return MedicationsDTO{}, err
	}
	if err := store.DeleteMedicationEvent(context.Background(), strings.TrimSpace(input.EventID)); err != nil {
		return MedicationsDTO{}, err
	}
	return a.GetMedications()
}

func (a *App) ExportMedicationData() (MedicationExportDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return MedicationExportDTO{}, err
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	exported, err := store.ExportMedicationData(context.Background(), now)
	if err != nil {
		return MedicationExportDTO{}, err
	}
	encoded, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		return MedicationExportDTO{}, err
	}
	encoded = append(encoded, '\n')
	return MedicationExportDTO{
		FileName:        "zeitboard-medication-data-v2-" + now.Format("20060102") + ".json",
		JSON:            string(encoded),
		GeneratedAt:     now.Format(time.RFC3339),
		GeneratedLabel:  now.Local().Format("Jan 2, 2006, 3:04 PM"),
		MedicationCount: len(exported.MedicationSet.Medications),
		EventCount:      len(exported.EventSet.Events),
	}, nil
}

func (a *App) medicationsAt(now time.Time) (MedicationsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return MedicationsDTO{}, err
	}
	ctx := context.Background()
	medications, err := store.ListMedications(ctx)
	if err != nil {
		return MedicationsDTO{}, err
	}
	effective, err := store.EffectiveMedicationEvents(ctx)
	if err != nil {
		return MedicationsDTO{}, err
	}
	state, err := a.localEstimate(ctx, now)
	if err != nil {
		return MedicationsDTO{}, err
	}
	medicationByID := make(map[string]storage.MedicationRecord, len(medications))
	eventCounts := make(map[string]int, len(medications))
	for _, record := range medications {
		medicationByID[record.MedicationID] = record
	}
	for _, item := range effective {
		eventCounts[item.Event.MedicationID]++
	}
	medicationDTOs := make([]MedicationDTO, 0, len(medications))
	for _, record := range medications {
		item, err := medicationDTO(record, eventCounts[record.MedicationID], state, now)
		if err != nil {
			return MedicationsDTO{}, fmt.Errorf("project medication %s: %w", record.MedicationID, err)
		}
		medicationDTOs = append(medicationDTOs, item)
	}
	sort.SliceStable(medicationDTOs, func(i, j int) bool {
		if medicationDTOs[i].Active != medicationDTOs[j].Active {
			return medicationDTOs[i].Active
		}
		return strings.ToLower(medicationDTOs[i].Label) < strings.ToLower(medicationDTOs[j].Label)
	})
	eventDTOs := make([]MedicationLogDTO, 0, len(effective))
	anchors := medicationWakeAnchors(state.Sessions)
	latestWake := latestMedicationWake(anchors)
	for index := len(effective) - 1; index >= 0; index-- {
		item := effective[index]
		medication, exists := medicationByID[item.Event.MedicationID]
		if !exists {
			return MedicationsDTO{}, fmt.Errorf("medication event %s has no definition", item.Event.EventID)
		}
		eventDTOs = append(eventDTOs, medicationEventDTO(item, medication.Label, state, anchors, latestWake))
	}
	estimateMessage := state.Message
	if state.Status == "estimated" {
		estimateMessage = "Current rhythm estimate available for recent-event context."
	} else if estimateMessage == "" {
		estimateMessage = "Rhythm context is unavailable; raw medication records remain usable."
	}
	status := "ready"
	message := fmt.Sprintf("%d %s and %d medication %s stored only on this device.",
		len(medications), plural(len(medications), "medication", "medications"),
		len(effective), plural(len(effective), "event", "events"),
	)
	if len(medications) == 0 {
		status = "empty"
		message = "No medications yet. Add a private label before logging a taken or skipped event."
	}
	reminderStatus, reminderMessage := a.medicationReminderStatus(medications)
	return MedicationsDTO{
		Status:                status,
		Empty:                 len(medications) == 0,
		Message:               message,
		EstimateStatus:        state.Status,
		EstimateMessage:       estimateMessage,
		Medications:           medicationDTOs,
		Events:                eventDTOs,
		FixtureMode:           false,
		Disclaimer:            "Medication timing shown here is user-entered or derived context, not medical advice.",
		InteractionDisclaimer: medicationInteractionDisclaimer,
		ReminderStatus:        reminderStatus,
		ReminderMessage:       reminderMessage,
		UpdatedLabel:          now.Local().Format("Updated Jan 2, 3:04 PM"),
	}, nil
}

func medicationDTO(record storage.MedicationRecord, eventCount int, state localEstimateState, now time.Time) (MedicationDTO, error) {
	detailParts := make([]string, 0, 2)
	if record.Form != "" {
		detailParts = append(detailParts, record.Form)
	}
	if record.StrengthLabel != "" {
		detailParts = append(detailParts, record.StrengthLabel)
	}
	detail := strings.Join(detailParts, " - ")
	if detail == "" {
		detail = "No form or strength label"
	}
	scheduleKind := "none"
	var scheduleDTO *MedicationScheduleDTO
	if record.Schedule != nil {
		scheduleKind = record.Schedule.Kind
		projected, err := medicationScheduleDTO(*record.Schedule, state, now)
		if err != nil {
			return MedicationDTO{}, err
		}
		scheduleDTO = &projected
	}
	attribution := ""
	if record.ClinicianRule != "" {
		attribution = "Clinician guidance entered verbatim by you"
	}
	return MedicationDTO{
		MedicationID:             record.MedicationID,
		Label:                    record.Label,
		Form:                     record.Form,
		StrengthLabel:            record.StrengthLabel,
		DetailLabel:              detail,
		ClinicianRule:            record.ClinicianRule,
		ClinicianRuleAttribution: attribution,
		Active:                   record.Active,
		Revision:                 record.Revision,
		ScheduleKind:             scheduleKind,
		Schedule:                 scheduleDTO,
		CreatedLabel:             "Added " + record.CreatedAt.Local().Format("Jan 2, 2006"),
		EventCount:               eventCount,
	}, nil
}

func medicationScheduleDTO(schedule storage.MedicationSchedule, state localEstimateState, now time.Time) (MedicationScheduleDTO, error) {
	location, err := time.LoadLocation(schedule.ZoneID)
	if schedule.Kind == storage.MedicationScheduleAsNeeded {
		location = time.UTC
		err = nil
	}
	if err != nil {
		return MedicationScheduleDTO{}, err
	}
	through := now.In(location).AddDate(0, 0, 14)
	expansion, err := medicationcore.ExpandSchedule(schedule, now, through)
	if err != nil {
		return MedicationScheduleDTO{}, err
	}
	var estimate *domain.PhaseEstimate
	if state.Status == "estimated" {
		estimate = &state.Estimate
	}
	forecast, err := medicationcore.AnalyzeCollisions(expansion, estimate, now)
	if err != nil {
		return MedicationScheduleDTO{}, err
	}
	occurrences := make([]MedicationScheduleOccurrenceDTO, 0, len(forecast.Assessments))
	for _, assessment := range forecast.Assessments {
		contextLabel := "Outside the current forecast horizon"
		switch assessment.Status {
		case medicationcore.OccurrenceInsidePredictedSleep:
			contextLabel = "Inside a current predicted sleep window"
		case medicationcore.OccurrenceOutsidePredictedSleep:
			contextLabel = "Not inside a current predicted sleep window"
		}
		dstNote := ""
		if assessment.Occurrence.Ambiguous {
			dstNote = "Repeated civil time; the first occurrence is used."
		}
		occurrences = append(occurrences, MedicationScheduleOccurrenceDTO{
			At:         assessment.Occurrence.At.UTC.Format(time.RFC3339),
			CivilDate:  assessment.Occurrence.CivilDate,
			CivilTime:  assessment.Occurrence.CivilTime,
			CivilLabel: assessment.Occurrence.At.UTC.In(location).Format("Mon Jan 2, 3:04 PM MST"),
			Status:     assessment.Status,
			Context:    contextLabel,
			Confidence: medicationForecastConfidence(assessment.Confidence),
			Ambiguous:  assessment.Occurrence.Ambiguous,
			DSTNote:    dstNote,
		})
	}
	gaps := make([]MedicationScheduleGapDTO, 0, len(forecast.Gaps))
	for _, gap := range forecast.Gaps {
		gaps = append(gaps, MedicationScheduleGapDTO{
			CivilDate:  gap.CivilDate,
			CivilTime:  gap.CivilTime,
			CivilLabel: medicationGapCivilLabel(gap, schedule.ZoneID),
			Message:    "This civil time does not exist on this DST transition; no schedule occurrence is generated.",
		})
	}
	projectedForecast := MedicationScheduleForecastDTO{
		Status:              forecast.Status,
		Message:             medicationForecastMessage(forecast, len(occurrences), len(gaps)),
		CoveredCount:        forecast.CoveredCount,
		CollisionCount:      forecast.CollisionCount,
		OutsideHorizonCount: forecast.OutsideHorizonCount,
		Occurrences:         occurrences,
		Gaps:                gaps,
	}
	if forecast.CoverageEndsAt != nil {
		projectedForecast.CoverageEndsAt = forecast.CoverageEndsAt.UTC().Format(time.RFC3339)
		projectedForecast.CoverageLabel = forecast.CoverageEndsAt.In(location).Format("Jan 2, 3:04 PM MST")
	}
	return MedicationScheduleDTO{
		Kind:            schedule.Kind,
		ZoneID:          schedule.ZoneID,
		CivilTimes:      append([]string{}, schedule.CivilTimes...),
		DaysOn:          schedule.DaysOn,
		DaysOff:         schedule.DaysOff,
		CycleStartedOn:  schedule.CycleStartedOn,
		ReminderEnabled: schedule.ReminderEnabled,
		Summary:         medicationScheduleSummary(schedule),
		Forecast:        projectedForecast,
	}, nil
}

func medicationScheduleSummary(schedule storage.MedicationSchedule) string {
	switch schedule.Kind {
	case storage.MedicationScheduleAsNeeded:
		return "As needed; no clock times"
	case storage.MedicationScheduleFixedClock:
		return strings.Join(schedule.CivilTimes, ", ") + " in " + schedule.ZoneID
	case storage.MedicationScheduleCycling:
		return fmt.Sprintf("%d days on, %d days off; %s in %s", schedule.DaysOn, schedule.DaysOff, strings.Join(schedule.CivilTimes, ", "), schedule.ZoneID)
	default:
		return "Schedule unavailable"
	}
}

func medicationForecastMessage(forecast medicationcore.CollisionForecast, occurrenceCount, gapCount int) string {
	var message string
	switch forecast.Status {
	case medicationcore.ForecastNotApplicable:
		message = "As-needed schedules do not create timed occurrences."
	case medicationcore.ForecastCollision:
		message = fmt.Sprintf("%d of %d covered scheduled occurrences fall inside current predicted sleep windows.", forecast.CollisionCount, forecast.CoveredCount)
	case medicationcore.ForecastNoOverlap:
		message = fmt.Sprintf("None of %d covered scheduled occurrences fall inside current predicted sleep windows.", forecast.CoveredCount)
	default:
		if occurrenceCount == 0 && gapCount == 0 {
			message = "No scheduled occurrences fall in the next 14 civil days."
		} else if occurrenceCount == 0 {
			message = "No occurrence is generated for the reported nonexistent civil time; scheduled times remain unchanged."
		} else if forecast.OutsideHorizonCount > 0 {
			message = "Current rhythm coverage does not extend to these scheduled occurrences; their times remain unchanged."
		} else {
			message = "Current rhythm forecast is unavailable; scheduled times remain unchanged."
		}
	}
	if forecast.OutsideHorizonCount > 0 && forecast.CoveredCount > 0 {
		message += fmt.Sprintf(" %d later %s outside the current forecast horizon.", forecast.OutsideHorizonCount, plural(forecast.OutsideHorizonCount, "occurrence is", "occurrences are"))
	}
	return message
}

func medicationForecastConfidence(confidence domain.InferenceConfidence) string {
	if confidence.Level == domain.ConfidenceUnknown {
		return "Unknown"
	}
	return confidenceTitle(confidence.Level)
}

func medicationGapCivilLabel(gap medicationcore.ScheduleGap, zoneID string) string {
	date, err := time.Parse(time.DateOnly, gap.CivilDate)
	if err != nil {
		return gap.CivilDate + " " + gap.CivilTime + " " + zoneID
	}
	return date.Format("Mon Jan 2") + ", " + gap.CivilTime + " " + zoneID
}

func medicationEventDTO(item storage.EffectiveMedicationEvent, label string, state localEstimateState, anchors []domain.WakeAnchor, latestWake *domain.WakeAnchor) MedicationLogDTO {
	event := item.Event
	instant := domain.MustZonedInstant(event.DoseAt, event.ZoneID)
	var estimate *domain.PhaseEstimate
	if state.Status == "estimated" && latestWake != nil && !event.DoseAt.Before(latestWake.At.UTC) {
		estimate = &state.Estimate
	}
	relative := medicationcore.ResolveRelativeTiming(instant, anchors, estimate)
	wakeRelation := "No prior recorded wake"
	sleepRelation := "No comparable sleep window"
	sleepRelationKind := "unavailable"
	confidence := confidenceTitle(relative.Confidence.Level)
	if confidence == "Low" && relative.Confidence.Level == domain.ConfidenceUnknown {
		confidence = "Unknown"
	}
	if interval, found := containingMedicationSleep(state.Sessions, event.DoseAt); found {
		wakeRelation = "Inside a recorded sleep interval"
		sleepRelation = "Inside a recorded sleep interval"
		sleepRelationKind = "observed"
		confidence = confidenceTitle(medicationEvidenceConfidence(interval.StartEvidence).Level)
	} else {
		if relative.TimeSinceWake != nil {
			wakeRelation = compactMedicationDuration(*relative.TimeSinceWake) + " after recorded wake"
		}
		if nextSleep, found := nextObservedMedicationSleep(state.Sessions, event.DoseAt); found {
			sleepRelation = compactMedicationDuration(nextSleep.Interval.Start.UTC.Sub(event.DoseAt)) + " before next recorded sleep"
			sleepRelationKind = "observed"
			confidence = confidenceTitle(medicationEvidenceConfidence(nextSleep.StartEvidence).Level)
		} else if estimate != nil {
			if predictedSleepContains(*estimate, event.DoseAt) {
				sleepRelation = "Inside a predicted sleep window"
				sleepRelationKind = "predicted"
				confidence = confidenceTitle(estimate.Confidence.Level)
			} else if relative.TimeBeforePredictedSleep != nil {
				sleepRelation = compactMedicationDuration(*relative.TimeBeforePredictedSleep) + " before predicted sleep"
				sleepRelationKind = "predicted"
				confidence = confidenceTitle(estimate.Confidence.Level)
			}
		}
	}
	local, err := instant.InLocation()
	if err != nil {
		local = event.DoseAt.UTC()
	}
	return MedicationLogDTO{
		EventID:           event.EventID,
		MedicationID:      event.MedicationID,
		MedicationLabel:   label,
		DoseLocal:         local.Format("2006-01-02T15:04"),
		CivilTime:         local.Format("Mon Jan 2, 3:04 PM MST"),
		ZoneID:            event.ZoneID,
		Status:            event.Status,
		Scheduled:         event.Scheduled,
		Note:              event.Note,
		RecordedLabel:     "Recorded " + event.Provenance.RecordedAt.Local().Format("Jan 2, 3:04 PM"),
		WakeRelation:      wakeRelation,
		SleepRelation:     sleepRelation,
		SleepRelationKind: sleepRelationKind,
		Confidence:        confidence,
		Excluded:          item.Excluded,
		CorrectionCount:   len(item.Corrections),
	}
}

func medicationEventValues(doseLocal, rawZoneID, rawStatus, rawNote string, now time.Time) (time.Time, string, string, string, error) {
	zoneID := strings.TrimSpace(rawZoneID)
	if zoneID == "" {
		zoneID = defaultZoneID
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return time.Time{}, "", "", "", fmt.Errorf("load time zone %q: %w", zoneID, err)
	}
	doseAt, err := parseCivilTime(strings.TrimSpace(doseLocal), location)
	if err != nil {
		return time.Time{}, "", "", "", fmt.Errorf("dose time: %w", err)
	}
	if doseAt.UTC().After(now.UTC().Add(5 * time.Minute)) {
		return time.Time{}, "", "", "", errors.New("medication events cannot be logged in the future")
	}
	status := strings.TrimSpace(rawStatus)
	if status != storage.MedicationEventTaken && status != storage.MedicationEventSkipped {
		return time.Time{}, "", "", "", errors.New("medication event status must be taken or skipped")
	}
	note := strings.TrimSpace(rawNote)
	if len(note) > 500 {
		return time.Time{}, "", "", "", errors.New("medication event note must be 500 characters or fewer")
	}
	return doseAt.UTC(), zoneID, status, note, nil
}

func medicationWakeAnchors(sessions []domain.SleepSession) []domain.WakeAnchor {
	anchors := make([]domain.WakeAnchor, 0, len(sessions))
	for _, session := range sessions {
		if session.IsNap || session.Suppressed {
			continue
		}
		for index, interval := range session.Intervals {
			anchors = append(anchors, domain.WakeAnchor{
				ID:         fmt.Sprintf("%s-wake-%d", session.ID, index),
				At:         interval.Interval.End,
				Confidence: medicationEvidenceConfidence(interval.EndEvidence),
				Evidence:   interval.EndEvidence,
			})
		}
	}
	return anchors
}

func latestMedicationWake(anchors []domain.WakeAnchor) *domain.WakeAnchor {
	var latest *domain.WakeAnchor
	for index := range anchors {
		if latest == nil || anchors[index].At.UTC.After(latest.At.UTC) {
			latest = &anchors[index]
		}
	}
	return latest
}

func medicationEvidenceConfidence(evidence domain.Evidence) domain.InferenceConfidence {
	level := domain.ConfidenceMedium
	switch evidence.Status {
	case domain.StatusObserved:
		level = domain.ConfidenceHigh
	case domain.StatusInferred:
		level = domain.ConfidenceLow
	}
	return domain.InferenceConfidence{Level: level, Reasons: []string{"derived from recorded sleep evidence"}}
}

func containingMedicationSleep(sessions []domain.SleepSession, at time.Time) (domain.SleepInterval, bool) {
	for _, session := range sessions {
		if session.IsNap || session.Suppressed {
			continue
		}
		for _, interval := range session.Intervals {
			if interval.Interval.Contains(at) {
				return interval, true
			}
		}
	}
	return domain.SleepInterval{}, false
}

func nextObservedMedicationSleep(sessions []domain.SleepSession, at time.Time) (domain.SleepInterval, bool) {
	var next *domain.SleepInterval
	for _, session := range sessions {
		if session.IsNap || session.Suppressed {
			continue
		}
		for index := range session.Intervals {
			interval := &session.Intervals[index]
			if !interval.Interval.Start.UTC.After(at) {
				continue
			}
			if next == nil || interval.Interval.Start.UTC.Before(next.Interval.Start.UTC) {
				copy := *interval
				next = &copy
			}
		}
	}
	if next == nil {
		return domain.SleepInterval{}, false
	}
	return *next, true
}

func predictedSleepContains(estimate domain.PhaseEstimate, at time.Time) bool {
	for _, window := range estimate.PredictedSleepWindows {
		if window.Interval.Contains(at) {
			return true
		}
	}
	return false
}

func compactMedicationDuration(value time.Duration) string {
	if value < 0 {
		value = -value
	}
	totalMinutes := int(value.Round(time.Minute).Minutes())
	hours := totalMinutes / 60
	minutes := totalMinutes % 60
	if hours == 0 {
		return fmt.Sprintf("%d min", minutes)
	}
	if minutes == 0 {
		return fmt.Sprintf("%d h", hours)
	}
	return fmt.Sprintf("%d h %d min", hours, minutes)
}

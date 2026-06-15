package domain

import (
	"encoding/json"
	"time"
)

type (
	DataSourceID         string
	ObservationID        string
	SleepSessionID       string
	CorrectionID         string
	PhaseEstimateID      string
	CalendarEventID      string
	FlexibleTaskID       string
	MedicationID         string
	MedicationEventID    string
	ShareProfileID       string
	AvailabilityWindowID string
)

type AcquisitionKind string

const (
	AcquisitionCollected AcquisitionKind = "collected"
	AcquisitionImported  AcquisitionKind = "imported"
	AcquisitionManual    AcquisitionKind = "manual"
)

type EvidenceStatus string

const (
	StatusObserved      EvidenceStatus = "observed"
	StatusImported      EvidenceStatus = "imported"
	StatusUserConfirmed EvidenceStatus = "user_confirmed"
	StatusInferred      EvidenceStatus = "inferred"
)

type ConfidenceLevel string

const (
	ConfidenceUnknown ConfidenceLevel = "unknown"
	ConfidenceLow     ConfidenceLevel = "low"
	ConfidenceMedium  ConfidenceLevel = "medium"
	ConfidenceHigh    ConfidenceLevel = "high"
)

type InferenceConfidence struct {
	Level   ConfidenceLevel `json:"level"`
	Reasons []string        `json:"reasons,omitempty"`
}

type ZonedInstant struct {
	UTC    time.Time `json:"utc"`
	ZoneID string    `json:"zoneId"`
}

type TimeRange struct {
	Start ZonedInstant `json:"start"`
	End   ZonedInstant `json:"end"`
}

type Evidence struct {
	Acquisition    AcquisitionKind `json:"acquisition"`
	Status         EvidenceStatus  `json:"status"`
	SourceIDs      []DataSourceID  `json:"sourceIds,omitempty"`
	ObservationIDs []ObservationID `json:"observationIds,omitempty"`
	CorrectionIDs  []CorrectionID  `json:"correctionIds,omitempty"`
	RecordedAt     time.Time       `json:"recordedAt"`
	Algorithm      string          `json:"algorithm,omitempty"`
}

type DataSource struct {
	ID           DataSourceID `json:"id"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Enabled      bool         `json:"enabled"`
	Capabilities []string     `json:"capabilities,omitempty"`
	CreatedAt    time.Time    `json:"createdAt"`
}

type SourceObservation struct {
	ID         ObservationID   `json:"id"`
	SourceID   DataSourceID    `json:"sourceId"`
	ExternalID string          `json:"externalId,omitempty"`
	Kind       string          `json:"kind"`
	ObservedAt ZonedInstant    `json:"observedAt"`
	RecordedAt time.Time       `json:"recordedAt"`
	Evidence   Evidence        `json:"evidence"`
	Payload    json.RawMessage `json:"payload"`
}

type ActivityState string

const (
	ActivityActive   ActivityState = "active"
	ActivityIdle     ActivityState = "idle"
	ActivityLocked   ActivityState = "locked"
	ActivityUnlocked ActivityState = "unlocked"
	ActivityStartup  ActivityState = "startup"
	ActivityResume   ActivityState = "resume"
)

type ActivityObservation struct {
	ID       ObservationID `json:"id"`
	Interval TimeRange     `json:"interval"`
	State    ActivityState `json:"state"`
	Evidence Evidence      `json:"evidence"`
}

type SleepInterval struct {
	Interval      TimeRange `json:"interval"`
	StartEvidence Evidence  `json:"startEvidence"`
	EndEvidence   Evidence  `json:"endEvidence"`
}

type SleepSession struct {
	ID          SleepSessionID  `json:"id"`
	Intervals   []SleepInterval `json:"intervals"`
	IsNap       bool            `json:"isNap"`
	Suppressed  bool            `json:"suppressed,omitempty"`
	SourceLabel string          `json:"sourceLabel,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type CorrectionKind string

const (
	CorrectionSetSleepStart CorrectionKind = "set_sleep_start"
	CorrectionSetWakeTime   CorrectionKind = "set_wake_time"
	CorrectionConfirmStart  CorrectionKind = "confirm_sleep_start"
	CorrectionConfirmWake   CorrectionKind = "confirm_wake_time"
	CorrectionClassifyNap   CorrectionKind = "classify_nap"
	CorrectionAwakeInactive CorrectionKind = "awake_but_inactive"
	CorrectionSuppress      CorrectionKind = "suppress_from_inference"
)

type ManualCorrection struct {
	ID            CorrectionID   `json:"id"`
	TargetID      SleepSessionID `json:"targetId"`
	Kind          CorrectionKind `json:"kind"`
	InstantValue  *ZonedInstant  `json:"instantValue,omitempty"`
	BoolValue     *bool          `json:"boolValue,omitempty"`
	IntervalValue *TimeRange     `json:"intervalValue,omitempty"`
	Note          string         `json:"note,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	SupersedesID  *CorrectionID  `json:"supersedesId,omitempty"`
	Active        bool           `json:"active"`
}

type ManualEntryKind string

const (
	ManualEntrySleep      ManualEntryKind = "sleep"
	ManualEntryNap        ManualEntryKind = "nap"
	ManualEntryAwakeIdle  ManualEntryKind = "awake_but_inactive"
	ManualEntryMedication ManualEntryKind = "medication_dose"
	ManualEntryNote       ManualEntryKind = "note"
)

type ManualEntry struct {
	ID              string             `json:"id"`
	Kind            ManualEntryKind    `json:"kind"`
	Interval        *TimeRange         `json:"interval,omitempty"`
	OccurredAt      *ZonedInstant      `json:"occurredAt,omitempty"`
	MedicationEvent *MedicationEventID `json:"medicationEventId,omitempty"`
	Note            string             `json:"note,omitempty"`
	CreatedAt       time.Time          `json:"createdAt"`
}

type WakeAnchor struct {
	ID         string              `json:"id"`
	At         ZonedInstant        `json:"at"`
	Confidence InferenceConfidence `json:"confidence"`
	Evidence   Evidence            `json:"evidence"`
}

type AvailabilityKind string

const (
	AvailabilityPredictedSleep AvailabilityKind = "predicted_sleep"
	AvailabilityPredictedWake  AvailabilityKind = "predicted_wake"
	AvailabilityFunctional     AvailabilityKind = "functional"
	AvailabilityFree           AvailabilityKind = "free"
)

type AvailabilityWindow struct {
	ID          AvailabilityWindowID `json:"id"`
	Kind        AvailabilityKind     `json:"kind"`
	Interval    TimeRange            `json:"interval"`
	Confidence  InferenceConfidence  `json:"confidence"`
	EstimateID  PhaseEstimateID      `json:"estimateId,omitempty"`
	Explanation string               `json:"explanation,omitempty"`
}

type PhaseEstimate struct {
	ID                       PhaseEstimateID      `json:"id"`
	AsOf                     ZonedInstant         `json:"asOf"`
	CharacteristicSleepStart ZonedInstant         `json:"characteristicSleepStart"`
	ObservedCycleLength      time.Duration        `json:"observedCycleLength"`
	ObservedDriftPerCycle    time.Duration        `json:"observedDriftPerCycle"`
	TypicalSleepDuration     time.Duration        `json:"typicalSleepDuration"`
	PredictedSleepWindows    []AvailabilityWindow `json:"predictedSleepWindows"`
	PredictedWakingWindows   []AvailabilityWindow `json:"predictedWakingWindows"`
	Confidence               InferenceConfidence  `json:"confidence"`
	InputSessionIDs          []SleepSessionID     `json:"inputSessionIds"`
	AlgorithmVersion         string               `json:"algorithmVersion"`
	CreatedAt                time.Time            `json:"createdAt"`
}

type CalendarEvent struct {
	ID       CalendarEventID `json:"id"`
	Title    string          `json:"title"`
	Interval TimeRange       `json:"interval"`
	Fixed    bool            `json:"fixed"`
	Location string          `json:"location,omitempty"`
	Notes    string          `json:"notes,omitempty"`
}

type TaskConstraint struct {
	Deadline               *ZonedInstant   `json:"deadline,omitempty"`
	EarliestStart          *ZonedInstant   `json:"earliestStart,omitempty"`
	LatestFinish           *ZonedInstant   `json:"latestFinish,omitempty"`
	PreferredAfterWake     *time.Duration  `json:"preferredAfterWake,omitempty"`
	BusinessHours          bool            `json:"businessHours"`
	BusinessStartLocal     string          `json:"businessStartLocal,omitempty"`
	BusinessEndLocal       string          `json:"businessEndLocal,omitempty"`
	MinimumConfidence      ConfidenceLevel `json:"minimumConfidence"`
	CognitiveDemand        string          `json:"cognitiveDemand,omitempty"`
	PhysicalDemand         string          `json:"physicalDemand,omitempty"`
	AllowAutomaticMovement bool            `json:"allowAutomaticMovement"`
	MaximumAutomaticMoves  int             `json:"maximumAutomaticMoves"`
	RequiresApproval       bool            `json:"requiresApproval"`
}

type FlexibleTask struct {
	ID                FlexibleTaskID `json:"id"`
	Title             string         `json:"title"`
	EstimatedDuration time.Duration  `json:"estimatedDuration"`
	Constraint        TaskConstraint `json:"constraint"`
	AutomaticMoves    int            `json:"automaticMoves"`
}

type Medication struct {
	ID          MedicationID `json:"id"`
	DisplayName string       `json:"displayName"`
	Notes       string       `json:"notes,omitempty"`
}

type OperatingMode string

const (
	ModeObserved  OperatingMode = "observed"
	ModeEstimated OperatingMode = "estimated"
	ModeFixture   OperatingMode = "fixture"
)

type MedicationEvent struct {
	ID                       MedicationEventID   `json:"id"`
	MedicationID             MedicationID        `json:"medicationId"`
	TakenAt                  ZonedInstant        `json:"takenAt"`
	TimeSinceWake            *time.Duration      `json:"timeSinceWake,omitempty"`
	TimeBeforePredictedSleep *time.Duration      `json:"timeBeforePredictedSleep,omitempty"`
	WakeAnchorID             string              `json:"wakeAnchorId,omitempty"`
	EstimateID               PhaseEstimateID     `json:"estimateId,omitempty"`
	Confidence               InferenceConfidence `json:"confidence"`
	Mode                     OperatingMode       `json:"mode"`
	Source                   string              `json:"source"`
	EffectNote               string              `json:"effectNote,omitempty"`
}

type SharePermission string

const (
	ShareLikelyState          SharePermission = "likely_state"
	ShareNextWakeWindow       SharePermission = "next_wake_window"
	ShareBestContactWindow    SharePermission = "best_contact_window"
	ShareSevenDayForecast     SharePermission = "seven_day_forecast"
	ShareUrgentInstructions   SharePermission = "urgent_instructions"
	ShareCalendarAvailability SharePermission = "calendar_availability"
)

type ShareProfile struct {
	ID          ShareProfileID    `json:"id"`
	Label       string            `json:"label"`
	Permissions []SharePermission `json:"permissions"`
	ExpiresAt   *time.Time        `json:"expiresAt,omitempty"`
	RevokedAt   *time.Time        `json:"revokedAt,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
}

type TrustedStatus struct {
	LikelyState               string               `json:"likelyState"`
	NextWakeWindow            *AvailabilityWindow  `json:"nextWakeWindow,omitempty"`
	BestContactWindow         *AvailabilityWindow  `json:"bestContactWindow,omitempty"`
	SevenDayAvailability      []AvailabilityWindow `json:"sevenDayAvailability,omitempty"`
	UrgentContactInstructions string               `json:"urgentContactInstructions,omitempty"`
	CalendarAvailability      []AvailabilityWindow `json:"calendarAvailability,omitempty"`
	PrivateMedicationNames    []string             `json:"-"`
	PrivateDiagnosis          string               `json:"-"`
	PrivateLocation           string               `json:"-"`
}

package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
	"non24.app/core/freshness"
)

const SchemaVersion = "v1"

type Refusal struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type OverviewResponse struct {
	SchemaVersion            string               `json:"schema_version"`
	Status                   string               `json:"status"`
	Refusal                  *Refusal             `json:"refusal,omitempty"`
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

type MedicationEventDTO struct {
	Medication     string `json:"medication"`
	CivilTime      string `json:"civilTime"`
	RelativeToWake string `json:"relativeToWake"`
}

type RhythmResponse struct {
	SchemaVersion string               `json:"schema_version"`
	Status        string               `json:"status"`
	Refusal       *Refusal             `json:"refusal,omitempty"`
	Projection    *RhythmProjectionDTO `json:"projection,omitempty"`
}

// RhythmProjectionDTO is the server/MCP allowlist. Civil dates preserve chart
// chronology already represented by the display labels; raw zone identifiers
// from the core projection remain absent from this network-facing form.
type RhythmProjectionDTO struct {
	FixtureMode    bool   `json:"fixtureMode"`
	EstimateSource string `json:"estimateSource,omitempty"`
	Status         string `json:"status,omitempty"`

	ActogramSummary string          `json:"actogramSummary"`
	ObservedRows    []RhythmBandDTO `json:"observedRows"`
	ForecastRows    []RhythmBandDTO `json:"forecastRows"`
	Now             RhythmNowDTO    `json:"now"`

	DriftTitle      string                `json:"driftTitle"`
	SlopeLabel      string                `json:"slopeLabel"`
	DriftConfidence string                `json:"driftConfidence"`
	DriftSummary    string                `json:"driftSummary"`
	YMinHour        float64               `json:"yMinHour"`
	YMaxHour        float64               `json:"yMaxHour"`
	DriftPoints     []RhythmDriftPointDTO `json:"driftPoints"`
}

type RhythmBandDTO struct {
	ID            string  `json:"id"`
	Day           string  `json:"day"`
	CivilDate     string  `json:"civilDate"`
	StartHour     float64 `json:"startHour"`
	DurationHours float64 `json:"durationHours"`
	Kind          string  `json:"kind"`
	StartLabel    string  `json:"startLabel"`
	WakeLabel     string  `json:"wakeLabel"`
	DurationLabel string  `json:"durationLabel"`
	Source        string  `json:"source"`
	Confidence    string  `json:"confidence"`
}

type RhythmNowDTO struct {
	Label     string  `json:"label"`
	Day       string  `json:"day"`
	CivilDate string  `json:"civilDate"`
	Hour      float64 `json:"hour"`
}

type RhythmDriftPointDTO struct {
	ID           string  `json:"id"`
	Day          string  `json:"day"`
	CivilDate    string  `json:"civilDate"`
	OnsetHour    float64 `json:"onsetHour"`
	FitHour      float64 `json:"fitHour"`
	BandLowHour  float64 `json:"bandLowHour"`
	BandHighHour float64 `json:"bandHighHour"`
	OnsetLabel   string  `json:"onsetLabel"`
	Source       string  `json:"source"`
	Confidence   string  `json:"confidence"`
}

type AccuracyResponse struct {
	SchemaVersion string                     `json:"schema_version"`
	Status        string                     `json:"status"`
	Refusal       *Refusal                   `json:"refusal,omitempty"`
	Report        *estimation.BacktestReport `json:"report,omitempty"`
}

type Service struct {
	Estimator estimation.RobustEstimator
	Now       func() time.Time
}

func (s Service) Overview(ctx context.Context, sessions []domain.SleepSession) (OverviewResponse, error) {
	now := s.now()
	estimator := s.estimator()
	estimate, err := estimator.Estimate(ctx, sessions, now)
	if err != nil {
		refusal, ok := refusalFromError(err)
		if ok {
			return OverviewResponse{
				SchemaVersion:    SchemaVersion,
				Status:           "refused",
				Refusal:          &refusal,
				MedicationEvents: []MedicationEventDTO{},
				FixtureMode:      false,
				Disclaimer:       disclaimer(),
			}, nil
		}
		return OverviewResponse{}, err
	}
	lastWake, err := latestWake(sessions)
	if err != nil {
		return OverviewResponse{}, err
	}
	if len(estimate.PredictedSleepWindows) == 0 {
		return OverviewResponse{}, errors.New("estimate has no predicted sleep windows")
	}
	nextSleep := estimate.PredictedSleepWindows[0].Interval
	// The same shared policy the desktop and the portal use. Without it this
	// projection reports a current state from whatever was last stored, however
	// old, because it recomputes on every request and so always looks current.
	//
	// Only the state string changes. A structured freshness block would be a
	// new field on a strict v1 response, and the desktop's own backend client
	// decodes with DisallowUnknownFields — precisely the "existing strict
	// consumers would reject the payload" case that contracts/README.md
	// reserves for a new contract version. The correction that matters (no
	// confident claim on stale evidence) does not need the new field; carrying
	// the reason to a synced client does, and waits for v2.
	assessment := freshness.Default().Assess(freshnessInputs(sessions, nextSleep, now))
	currentState := estimatedState(now, lastWake, nextSleep)
	if !assessment.MayClaimCurrentState() {
		currentState = "Unknown"
	}
	return OverviewResponse{
		SchemaVersion:            SchemaVersion,
		Status:                   "estimated",
		CurrentEstimatedState:    currentState,
		TimeSinceWake:            formatDuration(now.Sub(lastWake.UTC)),
		PredictedNextSleepWindow: formatRange(nextSleep),
		DriftEstimate:            fmt.Sprintf("%+.0f minutes per observed sleep cycle", estimate.ObservedDriftPerCycle.Minutes()),
		Confidence:               string(estimate.Confidence.Level),
		ConfidenceReasons:        append([]string(nil), estimate.Confidence.Reasons...),
		NextUsefulTaskWindow:     "Task sync is not configured yet.",
		SharingStatus:            "Server projection only; trusted sharing remains default-deny.",
		MedicationEvents:         []MedicationEventDTO{},
		FixtureMode:              false,
		Disclaimer:               disclaimer(),
	}, nil
}

func (s Service) Rhythm(ctx context.Context, sessions []domain.SleepSession) (RhythmResponse, error) {
	projection, err := s.estimator().Project(ctx, sessions, s.now())
	if err != nil {
		refusal, ok := refusalFromError(err)
		if ok {
			return RhythmResponse{SchemaVersion: SchemaVersion, Status: "refused", Refusal: &refusal}, nil
		}
		return RhythmResponse{}, err
	}
	projection.FixtureMode = false
	projection.ActogramSummary = "Double-plotted actogram of synced sleep with widening predicted sleep windows, all derived from the server estimate."
	sanitizeRhythmIDs(&projection)
	dto := rhythmProjectionDTO(projection)
	return RhythmResponse{SchemaVersion: SchemaVersion, Status: "estimated", Projection: &dto}, nil
}

func (s Service) Accuracy(ctx context.Context, sessions []domain.SleepSession) (AccuracyResponse, error) {
	report, err := s.estimator().Backtest(ctx, sessions)
	if err != nil {
		refusal, ok := refusalFromError(err)
		if ok {
			return AccuracyResponse{SchemaVersion: SchemaVersion, Status: "refused", Refusal: &refusal}, nil
		}
		return AccuracyResponse{}, err
	}
	return AccuracyResponse{SchemaVersion: SchemaVersion, Status: "estimated", Report: &report}, nil
}

func (s Service) estimator() estimation.RobustEstimator {
	if s.Estimator.Config.MinimumEpisodes == 0 {
		return estimation.RobustEstimator{}
	}
	return s.Estimator
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func refusalFromError(err error) (Refusal, bool) {
	var refusal *estimation.EstimationRefusal
	if errors.As(err, &refusal) {
		return Refusal{Code: string(refusal.Code), Message: refusal.Message}, true
	}
	return Refusal{}, false
}

func latestWake(sessions []domain.SleepSession) (domain.ZonedInstant, error) {
	var latest domain.ZonedInstant
	for _, session := range sessions {
		if len(session.Intervals) == 0 {
			continue
		}
		end := session.Intervals[0].Interval.End
		if latest.UTC.IsZero() || end.UTC.After(latest.UTC) {
			latest = end
		}
	}
	if latest.UTC.IsZero() {
		return domain.ZonedInstant{}, errors.New("no wake time available")
	}
	return latest, nil
}

func estimatedState(now time.Time, lastWake domain.ZonedInstant, nextSleep domain.TimeRange) string {
	switch {
	case now.Before(lastWake.UTC):
		return "Likely asleep"
	case nextSleep.Contains(now):
		return "Within predicted sleep window"
	default:
		return "Likely awake"
	}
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

func disclaimer() string {
	return "Estimates describe observed sleep-wake timing and uncertainty. This application does not provide medical advice."
}

func sanitizeRhythmIDs(projection *estimation.RhythmProjection) {
	for i := range projection.ObservedRows {
		projection.ObservedRows[i].ID = fmt.Sprintf("observed-%d", i+1)
	}
	for i := range projection.ForecastRows {
		projection.ForecastRows[i].ID = fmt.Sprintf("forecast-%d", i+1)
	}
	for i := range projection.DriftPoints {
		projection.DriftPoints[i].ID = fmt.Sprintf("drift-%d", i+1)
	}
}

func rhythmProjectionDTO(projection estimation.RhythmProjection) RhythmProjectionDTO {
	observed := make([]RhythmBandDTO, len(projection.ObservedRows))
	for i, row := range projection.ObservedRows {
		observed[i] = rhythmBandDTO(row)
	}
	forecast := make([]RhythmBandDTO, len(projection.ForecastRows))
	for i, row := range projection.ForecastRows {
		forecast[i] = rhythmBandDTO(row)
	}
	drift := make([]RhythmDriftPointDTO, len(projection.DriftPoints))
	for i, point := range projection.DriftPoints {
		drift[i] = RhythmDriftPointDTO{
			ID:           point.ID,
			Day:          point.Day,
			CivilDate:    point.CivilDate,
			OnsetHour:    point.OnsetHour,
			FitHour:      point.FitHour,
			BandLowHour:  point.BandLowHour,
			BandHighHour: point.BandHighHour,
			OnsetLabel:   point.OnsetLabel,
			Source:       point.Source,
			Confidence:   point.Confidence,
		}
	}
	return RhythmProjectionDTO{
		FixtureMode:     projection.FixtureMode,
		EstimateSource:  projection.EstimateSource,
		Status:          projection.Status,
		ActogramSummary: projection.ActogramSummary,
		ObservedRows:    observed,
		ForecastRows:    forecast,
		Now: RhythmNowDTO{
			Label:     projection.Now.Label,
			Day:       projection.Now.Day,
			CivilDate: projection.Now.CivilDate,
			Hour:      projection.Now.Hour,
		},
		DriftTitle:      projection.DriftTitle,
		SlopeLabel:      projection.SlopeLabel,
		DriftConfidence: projection.DriftConfidence,
		DriftSummary:    projection.DriftSummary,
		YMinHour:        projection.YMinHour,
		YMaxHour:        projection.YMaxHour,
		DriftPoints:     drift,
	}
}

func rhythmBandDTO(row estimation.RhythmBand) RhythmBandDTO {
	return RhythmBandDTO{
		ID:            row.ID,
		Day:           row.Day,
		CivilDate:     row.CivilDate,
		StartHour:     row.StartHour,
		DurationHours: row.DurationHours,
		Kind:          row.Kind,
		StartLabel:    row.StartLabel,
		WakeLabel:     row.WakeLabel,
		DurationLabel: row.DurationLabel,
		Source:        row.Source,
		Confidence:    row.Confidence,
	}
}

// freshnessInputs derives the shared policy's inputs. The newest evidence is
// when a record was recorded, not when the sleep it describes occurred.
func freshnessInputs(sessions []domain.SleepSession, nextSleep domain.TimeRange, now time.Time) freshness.Inputs {
	in := freshness.Inputs{Now: now, ExpectedSleepOnset: nextSleep.Start.UTC}
	for _, session := range sessions {
		for _, interval := range session.Intervals {
			end := interval.Interval.End.UTC
			if end.After(in.LatestSleepEnd) {
				in.LatestSleepEnd = end
			}
			recorded := interval.EndEvidence.RecordedAt
			if session.CreatedAt.After(recorded) {
				recorded = session.CreatedAt
			}
			if recorded.IsZero() {
				recorded = end
			}
			if recorded.After(in.NewestEvidence) {
				in.NewestEvidence = recorded
			}
		}
	}
	return in
}

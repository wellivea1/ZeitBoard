package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
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
	SchemaVersion string                       `json:"schema_version"`
	Status        string                       `json:"status"`
	Refusal       *Refusal                     `json:"refusal,omitempty"`
	Projection    *estimation.RhythmProjection `json:"projection,omitempty"`
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
	return OverviewResponse{
		SchemaVersion:            SchemaVersion,
		Status:                   "estimated",
		CurrentEstimatedState:    estimatedState(now, lastWake, nextSleep),
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
	return RhythmResponse{SchemaVersion: SchemaVersion, Status: "estimated", Projection: &projection}, nil
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

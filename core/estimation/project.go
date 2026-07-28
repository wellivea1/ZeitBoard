package estimation

import (
	"context"
	"fmt"
	"math"
	"time"

	"non24.app/core/domain"
)

// RhythmProjection is a presentation-ready, but engine-derived, view of the
// estimator's output: the same Theil-Sen fit, robust band, and widening
// forecast that Estimate computes, mapped onto civil clock time for the
// Rhythm actogram and drift charts. It exists so the visualizer reflects the
// real estimation engine instead of a hand-authored fixture that could drift
// from the math.
type RhythmProjection struct {
	FixtureMode    bool               `json:"fixtureMode"`
	EstimateSource string             `json:"estimateSource,omitempty"`
	Status         string             `json:"status,omitempty"`
	Refusal        *EstimationRefusal `json:"refusal,omitempty"`

	ActogramSummary string       `json:"actogramSummary"`
	ObservedRows    []RhythmBand `json:"observedRows"`
	ForecastRows    []RhythmBand `json:"forecastRows"`
	Now             RhythmNow    `json:"now"`

	DriftTitle      string             `json:"driftTitle"`
	SlopeLabel      string             `json:"slopeLabel"`
	DriftConfidence string             `json:"driftConfidence"`
	DriftSummary    string             `json:"driftSummary"`
	YMinHour        float64            `json:"yMinHour"`
	YMaxHour        float64            `json:"yMaxHour"`
	DriftPoints     []RhythmDriftPoint `json:"driftPoints"`
}

// RhythmBand is one sleep interval (observed or forecast) placed on the
// 0-24h actogram track. CivilDate/ZoneID keep hover-time arithmetic anchored
// to structured civil data; StartHour/DurationHours place the visual band.
type RhythmBand struct {
	ID            string  `json:"id"`
	Day           string  `json:"day"`
	CivilDate     string  `json:"civilDate"`
	ZoneID        string  `json:"zoneId"`
	StartHour     float64 `json:"startHour"`
	DurationHours float64 `json:"durationHours"`
	Kind          string  `json:"kind"`
	StartLabel    string  `json:"startLabel"`
	WakeLabel     string  `json:"wakeLabel"`
	DurationLabel string  `json:"durationLabel"`
	Source        string  `json:"source"`
	Confidence    string  `json:"confidence"`
}

// RhythmDriftPoint is one episode on the sleep-onset drift chart. The Hour
// fields are unwrapped (they may run below 0 or above 24) so the free-running
// trend stays a readable line across midnight; OnsetLabel carries the civil
// clock time for the screen-reader table.
type RhythmDriftPoint struct {
	ID           string  `json:"id"`
	Day          string  `json:"day"`
	CivilDate    string  `json:"civilDate"`
	ZoneID       string  `json:"zoneId"`
	OnsetHour    float64 `json:"onsetHour"`
	FitHour      float64 `json:"fitHour"`
	BandLowHour  float64 `json:"bandLowHour"`
	BandHighHour float64 `json:"bandHighHour"`
	OnsetLabel   string  `json:"onsetLabel"`
	Source       string  `json:"source"`
	Confidence   string  `json:"confidence"`
}

type RhythmNow struct {
	Label     string  `json:"label"`
	Day       string  `json:"day"`
	CivilDate string  `json:"civilDate"`
	ZoneID    string  `json:"zoneId"`
	Hour      float64 `json:"hour"`
}

// Project derives the Rhythm visualization from the same sessions and fit the
// estimator uses. It returns the estimator's refusal unchanged when the data
// is insufficient or out of range, so the UI can show an honest refusal rather
// than an invented chart.
func (e RobustEstimator) Project(ctx context.Context, sessions []domain.SleepSession, asOf time.Time) (RhythmProjection, error) {
	estimate, err := e.Estimate(ctx, sessions, asOf)
	if err != nil {
		return RhythmProjection{}, err
	}

	config := e.Config
	if config.MinimumEpisodes == 0 {
		config = DefaultConfig()
	}
	// Estimate already succeeded with these inputs, so the fit recomputation
	// below cannot fail; it reuses the exact helpers Estimate uses.
	episodes, _ := selectEpisodes(sessions, config.MaximumEpisodes)
	indices, _ := cycleIndices(episodes)

	base := episodes[0].Intervals[0].Interval.Start.UTC
	x := make([]float64, len(episodes))
	y := make([]float64, len(episodes))
	for i, episode := range episodes {
		x[i] = float64(indices[i])
		y[i] = episode.Intervals[0].Interval.Start.UTC.Sub(base).Hours()
	}
	slope := theilSenSlope(x, y)
	intercepts := make([]float64, len(x))
	for i := range x {
		intercepts[i] = y[i] - slope*x[i]
	}
	intercept := median(intercepts)
	residuals := make([]float64, len(x))
	for i := range x {
		residuals[i] = math.Abs(y[i] - (intercept + slope*x[i]))
	}
	// Robust 1-sigma band (1.4826 * MAD), floored so it stays visible.
	bandHours := math.Max(0.5, 1.4826*median(residuals))

	// Unwrap the fit's clock-hour series by continuity, then place each onset
	// nearest its fit, so the drift line reads across midnight.
	fitPlot := make([]float64, len(episodes))
	onsetPlot := make([]float64, len(episodes))
	yMin := math.Inf(1)
	yMax := math.Inf(-1)
	driftPoints := make([]RhythmDriftPoint, len(episodes))
	for i, episode := range episodes {
		onset := episode.Intervals[0].Interval.Start
		pointZoneID := onset.ZoneID
		fitInstant, ferr := domain.NewZonedInstant(base.Add(hoursDuration(intercept+slope*x[i])), pointZoneID)
		if ferr != nil {
			return RhythmProjection{}, ferr
		}
		fitClock := clockHour(mustLocal(fitInstant))
		onsetClock := clockHour(mustLocal(onset))
		if i == 0 {
			fitPlot[i] = fitClock
		} else {
			fitPlot[i] = nearest(fitClock, fitPlot[i-1])
		}
		onsetPlot[i] = nearest(onsetClock, fitPlot[i])
		low := fitPlot[i] - bandHours
		high := fitPlot[i] + bandHours
		yMin = math.Min(yMin, math.Min(onsetPlot[i], low))
		yMax = math.Max(yMax, math.Max(onsetPlot[i], high))
		_, conf := episodeKind(episode)
		driftPoints[i] = RhythmDriftPoint{
			ID:           "drift-" + string(episode.ID),
			Day:          mustLocal(onset).Format("Jan 2"),
			CivilDate:    mustLocal(onset).Format("2006-01-02"),
			ZoneID:       pointZoneID,
			OnsetHour:    round2(onsetPlot[i]),
			FitHour:      round2(fitPlot[i]),
			BandLowHour:  round2(low),
			BandHighHour: round2(high),
			OnsetLabel:   mustLocal(onset).Format("3:04 PM"),
			Source:       sourceLabel(episode),
			Confidence:   conf,
		}
	}

	// Actogram observed rows, newest first (matches the design's top-down read).
	observed := make([]RhythmBand, 0, len(episodes))
	for i := len(episodes) - 1; i >= 0; i-- {
		observed = append(observed, observedBand(episodes[i]))
	}

	// Forecast rows from the engine's widening predicted sleep windows.
	forecast := make([]RhythmBand, 0, len(estimate.PredictedSleepWindows))
	for i, window := range estimate.PredictedSleepWindows {
		start := mustLocal(window.Interval.Start)
		end := mustLocal(window.Interval.End)
		forecast = append(forecast, RhythmBand{
			ID:            fmt.Sprintf("forecast-%d", i+1),
			Day:           start.Format("Jan 2"),
			CivilDate:     start.Format("2006-01-02"),
			ZoneID:        window.Interval.Start.ZoneID,
			StartHour:     round2(clockHour(start)),
			DurationHours: round2(window.Interval.Duration().Hours()),
			Kind:          "forecast",
			StartLabel:    start.Format("Jan 2, 3:04 PM") + " earliest",
			WakeLabel:     end.Format("Jan 2, 3:04 PM") + " latest",
			DurationLabel: durationLabel(window.Interval.Duration()) + " window",
			Source:        fmt.Sprintf("Forecast cycle %d", i+1),
			Confidence:    confidenceLabel(window.Confidence.Level),
		})
	}

	nowLocal := mustLocal(estimate.AsOf)
	driftMinutes := int(math.Round(estimate.ObservedDriftPerCycle.Minutes()))
	confLabel := confidenceLabel(estimate.Confidence.Level)

	return RhythmProjection{
		FixtureMode:     false,
		Status:          "estimated",
		ActogramSummary: "Double-plotted actogram of observed sleep with widening predicted sleep windows, all derived from the local estimate.",
		ObservedRows:    observed,
		ForecastRows:    forecast,
		Now: RhythmNow{
			Label:     "now",
			Day:       nowLocal.Format("Jan 2"),
			CivilDate: nowLocal.Format("2006-01-02"),
			ZoneID:    estimate.AsOf.ZoneID,
			Hour:      round2(clockHour(nowLocal)),
		},
		DriftTitle:      "Sleep-onset drift",
		SlopeLabel:      fmt.Sprintf("%+d min per cycle", driftMinutes),
		DriftConfidence: confLabel,
		DriftSummary:    driftSummary(driftMinutes, confLabel),
		YMinHour:        round2(yMin - 0.5),
		YMaxHour:        round2(yMax + 0.5),
		DriftPoints:     driftPoints,
	}, nil
}

func observedBand(episode domain.SleepSession) RhythmBand {
	interval := episode.Intervals[0].Interval
	start := mustLocal(interval.Start)
	end := mustLocal(interval.End)
	kind, conf := episodeKind(episode)
	return RhythmBand{
		ID:            "sleep-" + string(episode.ID),
		Day:           start.Format("Jan 2"),
		CivilDate:     start.Format("2006-01-02"),
		ZoneID:        interval.Start.ZoneID,
		StartHour:     round2(clockHour(start)),
		DurationHours: round2(interval.Duration().Hours()),
		Kind:          kind,
		StartLabel:    start.Format("Jan 2, 3:04 PM"),
		WakeLabel:     end.Format("Jan 2, 3:04 PM"),
		DurationLabel: durationLabel(interval.Duration()),
		Source:        sourceLabel(episode),
		Confidence:    conf,
	}
}

// episodeKind classifies a sleep episode for the actogram legend and the band
// confidence, from its recorded evidence (not from invented styling).
func episodeKind(episode domain.SleepSession) (kind string, confidence string) {
	if episode.IsNapSleep() {
		return "nap", "Low"
	}
	if !episode.IsPrincipalSleep() {
		return "inferred", "Low"
	}
	start := episode.Intervals[0].StartEvidence
	end := episode.Intervals[0].EndEvidence
	if len(start.CorrectionIDs) > 0 || len(end.CorrectionIDs) > 0 {
		return "corrected", "Medium"
	}
	switch start.Status {
	case domain.StatusInferred:
		return "inferred", "Low"
	default:
		return "observed", "High"
	}
}

func sourceLabel(episode domain.SleepSession) string {
	if episode.SourceLabel != "" {
		return episode.SourceLabel
	}
	switch episode.Intervals[0].StartEvidence.Acquisition {
	case domain.AcquisitionImported:
		return "Imported sleep"
	case domain.AcquisitionCollected:
		return "Device activity"
	default:
		return "Manual sleep log"
	}
}

func confidenceLabel(level domain.ConfidenceLevel) string {
	switch level {
	case domain.ConfidenceHigh:
		return "High"
	case domain.ConfidenceMedium:
		return "Medium"
	default:
		return "Low"
	}
}

func driftSummary(minutes int, confidence string) string {
	direction := "later"
	magnitude := minutes
	if minutes < 0 {
		direction = "earlier"
		magnitude = -minutes
	}
	return fmt.Sprintf(
		"Sleep onset drifts %s by about %d minutes per observed sleep cycle with %s confidence.",
		direction, magnitude, lower(confidence),
	)
}

func lower(s string) string {
	switch s {
	case "High":
		return "high"
	case "Medium":
		return "medium"
	default:
		return "low"
	}
}

func mustLocal(instant domain.ZonedInstant) time.Time {
	local, err := instant.InLocation()
	if err != nil {
		return instant.UTC
	}
	return local
}

func clockHour(t time.Time) float64 {
	return float64(t.Hour()) + float64(t.Minute())/60 + float64(t.Second())/3600
}

// nearest shifts value by whole days so it lands within +/-12h of ref, keeping
// a clock-hour series continuous across midnight.
func nearest(value, ref float64) float64 {
	for value-ref > 12 {
		value -= 24
	}
	for value-ref < -12 {
		value += 24
	}
	return value
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func durationLabel(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours == 0 {
		return fmt.Sprintf("%d min", minutes)
	}
	return fmt.Sprintf("%d hr %d min", hours, minutes)
}

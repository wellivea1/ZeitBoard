package main

import (
	"context"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"non24.app/core/domain"
	"non24.app/core/estimation"
	"non24.app/core/ingest"
	"non24.app/core/medication"
	"non24.app/core/platform/activity"
	"non24.app/core/scheduling"
	"non24.app/desktop/platform/tray"
)

type App struct {
	ctx       context.Context
	collector *ingest.Manager
	sink      *ingest.MemorySink
	tray      tray.Controller
}

type OverviewDTO struct {
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
}

type UnplacedDTO struct {
	Title      string `json:"title"`
	Reason     string `json:"reason"`
	ReasonCode string `json:"reasonCode"`
	NextAction string `json:"nextAction"`
}

type ProposalsDTO struct {
	FixtureMode bool          `json:"fixtureMode"`
	Proposals   []ProposalDTO `json:"proposals"`
	Unplaced    []UnplacedDTO `json:"unplaced"`
}

func NewApp() *App {
	sink := &ingest.MemorySink{}
	return &App{
		sink:      sink,
		collector: ingest.NewManager(sink, activity.SafeCollector{ZoneID: "America/New_York"}),
		tray:      tray.New(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	_ = a.collector.Start(ctx)
	_ = a.tray.Start(tray.Callbacks{
		Show: func() {
			runtime.WindowUnminimise(ctx)
			runtime.WindowShow(ctx)
			runtime.WindowCenter(ctx)
		},
		Quit: func() { runtime.Quit(ctx) },
	})
}

func (a *App) shutdown(ctx context.Context) {
	_ = a.tray.Stop()
	_ = a.collector.Stop(ctx)
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

func (a *App) GetOverview() (OverviewDTO, error) {
	now := time.Now().UTC().Truncate(time.Minute)
	zone := "America/New_York"
	sessions := fixtureSessions(now, zone)
	estimate, err := (estimation.RobustEstimator{}).Estimate(context.Background(), sessions, now)
	if err != nil {
		return OverviewDTO{}, err
	}
	lastWake := sessions[len(sessions)-1].Intervals[0].Interval.End
	wakeAnchor := domain.WakeAnchor{
		ID:         "fixture-wake",
		At:         lastWake,
		Confidence: domain.InferenceConfidence{Level: domain.ConfidenceHigh, Reasons: []string{"fixture wake is manually confirmed"}},
	}
	nextSleep := estimate.PredictedSleepWindows[0].Interval
	currentAvailability := domain.AvailabilityWindow{
		ID:         "current-functional-window",
		Kind:       domain.AvailabilityFunctional,
		Interval:   domain.TimeRange{Start: domain.MustZonedInstant(now, zone), End: nextSleep.Start},
		Confidence: estimate.Confidence,
		EstimateID: estimate.ID,
	}
	task := domain.FlexibleTask{
		ID: "fixture-task", Title: "Prepare appointment notes", EstimatedDuration: 45 * time.Minute,
		Constraint: domain.TaskConstraint{MinimumConfidence: domain.ConfidenceLow, RequiresApproval: true},
	}
	proposal, proposalErr := (scheduling.Scheduler{}).Propose(scheduling.Request{
		Task: task, Availability: []domain.AvailabilityWindow{currentAvailability}, WakeAnchor: &wakeAnchor, Now: now,
	})
	usefulWindow := "No reliable proposal"
	if proposalErr == nil {
		usefulWindow = formatRange(proposal.Window)
	}
	medicationEvent := medication.AttachRelativeTiming(domain.MedicationEvent{
		ID: "fixture-dose", MedicationID: "fixture-medication", TakenAt: domain.MustZonedInstant(now.Add(-2*time.Hour), zone),
		Mode: domain.ModeFixture, Source: "manual fixture",
	}, []domain.WakeAnchor{wakeAnchor}, &estimate)
	return OverviewDTO{
		CurrentEstimatedState:    "Likely awake",
		TimeSinceWake:            formatDuration(now.Sub(lastWake.UTC)),
		PredictedNextSleepWindow: formatRange(nextSleep),
		DriftEstimate:            fmt.Sprintf("%+.0f minutes per observed sleep cycle", estimate.ObservedDriftPerCycle.Minutes()),
		Confidence:               string(estimate.Confidence.Level),
		ConfidenceReasons:        estimate.Confidence.Reasons,
		NextUsefulTaskWindow:     usefulWindow,
		SharingStatus:            "Static trusted-view prototype only; no public endpoint",
		MedicationEvents: []MedicationEventDTO{{
			Medication:     "Fixture medication record",
			CivilTime:      formatInstant(medicationEvent.TakenAt),
			RelativeToWake: formatDuration(*medicationEvent.TimeSinceWake) + " after confirmed wake",
		}},
		FixtureMode: true,
		Disclaimer:  "Estimates describe observed sleep-wake timing and uncertainty. This application does not provide medical advice.",
	}, nil
}

// GetRhythm projects the same synthetic sessions and fit that GetOverview uses
// into the Rhythm actogram and drift series, so both screens reflect one engine.
func (a *App) GetRhythm() (estimation.RhythmProjection, error) {
	now := time.Now().UTC().Truncate(time.Minute)
	zone := "America/New_York"
	sessions := fixtureSessions(now, zone)
	return (estimation.RobustEstimator{}).Project(context.Background(), sessions, now)
}

// GetProposals runs the real scheduler over the current estimate so the
// Approvals trust gate shows proposals the engine actually produced — with the
// constraints it enforced (explanation codes), the fixed events it avoided, and
// an honest unplaced list when no safe window exists. Nothing is applied here;
// every proposal still requires explicit approval in the UI.
func (a *App) GetProposals() (ProposalsDTO, error) {
	now := time.Now().UTC().Truncate(time.Minute)
	zone := "America/New_York"
	sessions := fixtureSessions(now, zone)
	estimate, err := (estimation.RobustEstimator{}).Estimate(context.Background(), sessions, now)
	if err != nil {
		return ProposalsDTO{}, err
	}

	lastWake := sessions[len(sessions)-1].Intervals[0].Interval.End
	wakeAnchor := domain.WakeAnchor{
		ID: "fixture-wake", At: lastWake,
		Confidence: domain.InferenceConfidence{Level: domain.ConfidenceHigh},
	}

	// Availability is the current functional window plus the engine's predicted
	// waking windows; a fixed event sits inside the current window so avoidance
	// is exercised against real placement, not a hand-drawn scenario.
	availability := append([]domain.AvailabilityWindow{}, estimate.PredictedWakingWindows...)
	var events []domain.CalendarEvent
	if len(estimate.PredictedSleepWindows) > 0 {
		nextSleep := estimate.PredictedSleepWindows[0].Interval
		current := domain.AvailabilityWindow{
			ID: "current-functional-window", Kind: domain.AvailabilityFunctional,
			Interval:   domain.TimeRange{Start: domain.MustZonedInstant(now, zone), End: nextSleep.Start},
			Confidence: estimate.Confidence, EstimateID: estimate.ID,
		}
		availability = append([]domain.AvailabilityWindow{current}, availability...)
		evStart, evEnd := now.Add(2*time.Hour), now.Add(3*time.Hour)
		if nextSleep.Start.UTC.After(evEnd) {
			events = append(events, domain.CalendarEvent{
				ID: "fixed-clinic", Title: "Clinic appointment", Fixed: true,
				Interval: domain.TimeRange{Start: domain.MustZonedInstant(evStart, zone), End: domain.MustZonedInstant(evEnd, zone)},
			})
		}
	}

	deadline := domain.MustZonedInstant(now.Add(5*24*time.Hour), zone)
	deepWorkEarliest := domain.MustZonedInstant(now.Add(18*time.Hour), zone)
	callEarliest := domain.MustZonedInstant(now, zone)
	callLatest := domain.MustZonedInstant(now.Add(20*time.Minute), zone)

	tasks := []domain.FlexibleTask{
		{ID: "task-email", Title: "Email the clinic", EstimatedDuration: 30 * time.Minute,
			Constraint: domain.TaskConstraint{MinimumConfidence: domain.ConfidenceLow, RequiresApproval: true}},
		{ID: "task-taxes", Title: "Tax paperwork focus block", EstimatedDuration: 90 * time.Minute,
			Constraint: domain.TaskConstraint{MinimumConfidence: domain.ConfidenceLow, RequiresApproval: true, Deadline: &deadline}},
		{ID: "task-deepwork", Title: "Deep work session", EstimatedDuration: 90 * time.Minute,
			Constraint: domain.TaskConstraint{MinimumConfidence: domain.ConfidenceLow, RequiresApproval: true, EarliestStart: &deepWorkEarliest}},
		{ID: "task-call", Title: "Call accountant before noon", EstimatedDuration: 60 * time.Minute,
			Constraint: domain.TaskConstraint{MinimumConfidence: domain.ConfidenceLow, RequiresApproval: true, EarliestStart: &callEarliest, LatestFinish: &callLatest}},
	}

	result := ProposalsDTO{FixtureMode: true}
	scheduler := scheduling.Scheduler{}
	for _, task := range tasks {
		proposal, perr := scheduler.Propose(scheduling.Request{
			Task: task, Availability: availability, Events: events, WakeAnchor: &wakeAnchor, Now: now,
		})
		if perr != nil {
			reason := scheduling.ClassifyUnplaced(perr)
			result.Unplaced = append(result.Unplaced, UnplacedDTO{
				Title:      task.Title,
				ReasonCode: string(reason),
				Reason:     unplacedReasonLabel(reason),
				NextAction: "Keep manual until the next estimate refresh.",
			})
			continue
		}
		result.Proposals = append(result.Proposals, ProposalDTO{
			ID:               "proposal-" + string(task.ID),
			Origin:           "scheduler",
			Kind:             "Place",
			Title:            task.Title,
			To:               formatRange(proposal.Window),
			RhythmContext:    rhythmContext(proposal, availability),
			Confidence:       confidenceTitle(proposal.Confidence.Level),
			ExplanationCodes: proposal.ExplanationCodes,
			ReasonLabels:     reasonLabels(proposal.ExplanationCodes),
			CreatedLabel:     "Proposed by Scheduler",
			ExpiresLabel:     "valid for the current estimate",
		})
	}
	return result, nil
}

func fixtureSessions(now time.Time, zone string) []domain.SleepSession {
	period := 25 * time.Hour
	lastStart := now.Add(-12 * time.Hour)
	sessions := make([]domain.SleepSession, 12)
	for i := range sessions {
		start := lastStart.Add(-time.Duration(11-i) * period)
		evidence := domain.Evidence{Acquisition: domain.AcquisitionManual, Status: domain.StatusUserConfirmed}
		sessions[i] = domain.SleepSession{
			ID:          domain.SleepSessionID(fmt.Sprintf("fixture-sleep-%02d", i+1)),
			SourceLabel: "Manual sleep log",
			Intervals: []domain.SleepInterval{{
				Interval:      domain.TimeRange{Start: domain.MustZonedInstant(start, zone), End: domain.MustZonedInstant(start.Add(8*time.Hour), zone)},
				StartEvidence: evidence, EndEvidence: evidence,
			}},
		}
	}
	return sessions
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

package projection

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
	"non24.app/core/freshness"
)

// CompanionResponse is a private, authenticated device contract. It is never
// passed to the portal or MCP/LLM projection allowlists.
type CompanionResponse struct {
	ContainsSyntheticData bool                 `json:"contains_synthetic_data"`
	SchemaVersion         string               `json:"schema_version"`
	SourceCursor          int64                `json:"source_cursor"`
	GeneratedAt           time.Time            `json:"generated_at"`
	ValidUntil            time.Time            `json:"valid_until"`
	AlgorithmVersion      string               `json:"algorithm_version"`
	Provenance            string               `json:"provenance"`
	Status                string               `json:"status"`
	Refusal               *Refusal             `json:"refusal,omitempty"`
	Freshness             CompanionFreshness   `json:"freshness"`
	Confidence            *CompanionConfidence `json:"confidence,omitempty"`
	Forecasts             []CompanionForecast  `json:"forecasts"`
	Sleep                 []CompanionSleep     `json:"sleep"`
}

type CompanionFreshness struct {
	State       string `json:"state"`
	Reason      string `json:"reason"`
	Explanation string `json:"explanation"`
}

type CompanionConfidence struct {
	Level   string   `json:"level"`
	Reasons []string `json:"reasons"`
}

type CompanionWindow struct {
	StartAt time.Time `json:"start_at"`
	EndAt   time.Time `json:"end_at"`
	ZoneID  string    `json:"zone_id"`
}

type CompanionForecast struct {
	Sleep  CompanionWindow `json:"predicted_sleep_window"`
	Waking CompanionWindow `json:"predicted_waking_window"`
}

type CompanionSleep struct {
	RowID          string    `json:"row_id"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	ZoneID         string    `json:"zone_id"`
	Classification string    `json:"classification"`
	Corrected      bool      `json:"corrected"`
}

func RefusedCompanion(cursor int64, now time.Time, code, message string) CompanionResponse {
	return CompanionResponse{
		SchemaVersion: "v2", SourceCursor: cursor, GeneratedAt: now.UTC(), ValidUntil: now.UTC().Add(15 * time.Minute),
		AlgorithmVersion: estimation.AlgorithmVersion, Provenance: "self_hosted_synced_sleep",
		Status: "refused", Refusal: &Refusal{Code: code, Message: message},
		Freshness: CompanionFreshness{State: "withheld", Reason: code, Explanation: message},
		Forecasts: []CompanionForecast{}, Sleep: []CompanionSleep{},
	}
}

func (s Service) Companion(ctx context.Context, sessions []domain.SleepSession, cursor int64) (CompanionResponse, error) {
	now := s.now()
	result := RefusedCompanion(cursor, now, "insufficient_data", "More usable sleep history is needed.")
	ordered := make([]domain.SleepSession, 0, len(sessions))
	for _, session := range sessions {
		if len(session.Intervals) > 0 {
			ordered = append(ordered, session)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Intervals[0].Interval.Start.UTC.After(ordered[j].Intervals[0].Interval.Start.UTC)
	})
	for _, session := range ordered {
		if session.Suppressed || len(session.Intervals) == 0 {
			continue
		}
		interval := session.Intervals[0]
		rowDigest := sha256.Sum256([]byte(session.ID))
		result.Sleep = append(result.Sleep, CompanionSleep{
			RowID: fmt.Sprintf("sleep_%x", rowDigest[:12]), StartAt: interval.Interval.Start.UTC, EndAt: interval.Interval.End.UTC,
			ZoneID: interval.Interval.Start.ZoneID, Classification: string(session.Classification),
			Corrected: len(interval.StartEvidence.CorrectionIDs)+len(interval.EndEvidence.CorrectionIDs) > 0,
		})
		if len(result.Sleep) == 256 {
			break
		}
	}
	estimate, err := s.estimator().Estimate(ctx, sessions, now)
	if err != nil {
		if refusal, ok := refusalFromError(err); ok {
			result.Refusal = &refusal
			result.Freshness.Reason = refusal.Code
			result.Freshness.Explanation = refusal.Message
			return result, nil
		}
		return CompanionResponse{}, err
	}
	result.Status, result.Refusal = "estimated", nil
	result.Confidence = &CompanionConfidence{Level: string(estimate.Confidence.Level), Reasons: append([]string{}, estimate.Confidence.Reasons...)}
	inputs := freshnessInputs(sessions, estimate.PredictedSleepWindows[0].Interval, now)
	assessment := freshness.Default().Assess(inputs)
	result.Freshness = CompanionFreshness{State: string(assessment.State), Reason: string(assessment.Reason), Explanation: assessment.Explanation}
	if expiry := freshness.Default().NextChange(inputs); !expiry.IsZero() && expiry.Before(result.ValidUntil) {
		result.ValidUntil = expiry
	}
	for i, sleep := range estimate.PredictedSleepWindows {
		if i >= len(estimate.PredictedWakingWindows) {
			break
		}
		wake := estimate.PredictedWakingWindows[i]
		result.Forecasts = append(result.Forecasts, CompanionForecast{
			Sleep:  CompanionWindow{sleep.Interval.Start.UTC, sleep.Interval.End.UTC, sleep.Interval.Start.ZoneID},
			Waking: CompanionWindow{wake.Interval.Start.UTC, wake.Interval.End.UTC, wake.Interval.Start.ZoneID},
		})
	}
	return result, nil
}

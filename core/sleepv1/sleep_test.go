package sleepv1

import (
	"context"
	"fmt"
	"testing"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
)

func TestFoldUsesTargetZoneAndPreservesUnknownClassification(t *testing.T) {
	start := time.Date(2026, 3, 1, 5, 0, 0, 0, time.UTC)
	correctedStart := start.Add(30 * time.Minute)
	unknown := ClassificationUnknown
	observations := []Observation{testObservation("obs_sleep_01", start, start.Add(8*time.Hour), ClassificationPrincipal)}
	corrections := []Correction{{
		CorrectionID:        "cor_sleep_01",
		TargetObservationID: "obs_sleep_01",
		CreatedAt:           start.Add(9 * time.Hour),
		Reason:              CorrectionUserEdit,
		Changes: CorrectionChanges{
			StartAt:             &correctedStart,
			SleepClassification: &unknown,
		},
	}}

	sessions, err := Fold(observations, corrections)
	if err != nil {
		t.Fatal(err)
	}
	got := sessions[0].Intervals[0].Interval.Start
	location, err := time.LoadLocation(got.ZoneID)
	if err != nil {
		t.Fatal(err)
	}
	local := got.UTC.In(location)
	if got.ZoneID != "America/New_York" || local.Hour() != 0 || local.Minute() != 30 {
		t.Fatalf("corrected start = %#v (%s), want 00:30 America/New_York", got, local)
	}
	if sessions[0].Classification != domain.SleepClassificationUnknown {
		t.Fatalf("classification = %q, want unknown", sessions[0].Classification)
	}
	if sessions[0].IsNap || sessions[0].IsPrincipalSleep() {
		t.Fatal("unknown sleep must be neither a nap nor principal sleep")
	}
}

func TestProviderRevisionNeverClaimsHumanConfirmation(t *testing.T) {
	start := time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC)
	observation := testObservation("hc_synthetic", start, start.Add(8*time.Hour), ClassificationPrincipal)
	observation.Provenance.AcquisitionMethod = AcquisitionHealthConnect
	observation.Provenance.EvidenceStatus = EvidenceDirectlyObserved
	revised := start.Add(20 * time.Minute)
	for _, author := range []string{AcquisitionHealthConnect, "", AcquisitionManual} {
		t.Run("author_"+author, func(t *testing.T) {
			correction := Correction{
				CorrectionID: "cor_synthetic", TargetObservationID: observation.ObservationID,
				CreatedAt: start.Add(9 * time.Hour), Reason: CorrectionSourceConflict,
				AcquisitionMethod: author, Changes: CorrectionChanges{StartAt: &revised},
			}
			sessions, err := Fold([]Observation{observation}, []Correction{correction})
			if err != nil {
				t.Fatal(err)
			}
			got := sessions[0].Intervals[0].StartEvidence
			if author != AcquisitionHealthConnect {
				if got.Status != domain.StatusUserConfirmed {
					t.Fatal("manual correction lost confirmation")
				}
			} else {
				if got.Status != domain.StatusObserved || got.Acquisition != domain.AcquisitionImported || !got.RecordedAt.Equal(correction.CreatedAt) {
					t.Fatalf("provider revision invented human confirmation or lost its revision provenance: %#v", got)
				}
			}
			if !observation.StartAt.Equal(start) {
				t.Fatal("source evidence was mutated")
			}
		})
	}
}

func TestProviderRevisionCannotImpersonateManualCorrectionOrAnotherSource(t *testing.T) {
	start := time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC)
	changed := start.Add(15 * time.Minute)
	correction := Correction{CorrectionID: "cor_synthetic", TargetObservationID: "obs_synthetic", CreatedAt: start.Add(9 * time.Hour),
		Reason: CorrectionSourceConflict, AcquisitionMethod: AcquisitionHealthConnect, Changes: CorrectionChanges{StartAt: &changed}}
	observation := testObservation("obs_synthetic", start, start.Add(8*time.Hour), ClassificationPrincipal)
	observation.Provenance.AcquisitionMethod = AcquisitionManual
	if _, err := Fold([]Observation{observation}, []Correction{correction}); err == nil {
		t.Fatal("provider revision rewrote another acquisition source")
	}
	excluded := true
	correction.Changes.Excluded = &excluded
	if err := ValidateCorrection(correction); err == nil {
		t.Fatal("provider revision performed user exclusion")
	}
	correction.Changes.Excluded = nil
	correction.Reason = CorrectionUserEdit
	if err := ValidateCorrection(correction); err == nil {
		t.Fatal("provider revision claimed to be a user edit")
	}
}

func TestNewSourceRevisionCannotSilentlyOverrideAnOlderUserCorrection(t *testing.T) {
	start := time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC)
	observation := testObservation("hc_synthetic", start, start.Add(8*time.Hour), ClassificationPrincipal)
	observation.Provenance.AcquisitionMethod = AcquisitionHealthConnect
	userStart, providerStart := start.Add(20*time.Minute), start.Add(40*time.Minute)
	user := Correction{CorrectionID: "cor_user", TargetObservationID: observation.ObservationID, CreatedAt: start.Add(9 * time.Hour),
		Reason: CorrectionUserEdit, AcquisitionMethod: AcquisitionManual, Changes: CorrectionChanges{StartAt: &userStart}}
	provider := Correction{CorrectionID: "cor_provider", TargetObservationID: observation.ObservationID, CreatedAt: start.Add(10 * time.Hour),
		Reason: CorrectionSourceConflict, AcquisitionMethod: AcquisitionHealthConnect, Changes: CorrectionChanges{StartAt: &providerStart}}
	if _, err := Fold([]Observation{observation}, []Correction{user, provider}); err == nil {
		t.Fatal("new source revision silently overrode an existing user correction")
	}
	user.CreatedAt = start.Add(11 * time.Hour)
	if _, err := Fold([]Observation{observation}, []Correction{user, provider}); err == nil {
		t.Fatal("a later save time was mistaken for review of the new source")
	}
	user.BasedOnSourceRevision = &provider.CreatedAt
	sessions, err := Fold([]Observation{observation}, []Correction{user, provider})
	if err != nil {
		t.Fatal(err)
	}
	if !sessions[0].Intervals[0].Interval.Start.UTC.Equal(userStart) {
		t.Fatal("reviewed user correction was not applied")
	}
}

func TestManualOverlayRetainsProviderEndpointWhenSupersedingAProviderRevision(t *testing.T) {
	start := time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC)
	observation := testObservation("hc_overlay", start, start.Add(8*time.Hour), ClassificationPrincipal)
	observation.Provenance.AcquisitionMethod = AcquisitionHealthConnect
	providerStart, providerEnd, userStart := start.Add(10*time.Minute), start.Add(9*time.Hour), start.Add(20*time.Minute)
	provider := Correction{CorrectionID: "cor_provider", TargetObservationID: observation.ObservationID, CreatedAt: start.Add(10 * time.Hour), Reason: CorrectionSourceConflict,
		AcquisitionMethod: AcquisitionHealthConnect, Changes: CorrectionChanges{StartAt: &providerStart, EndAt: &providerEnd}}
	user := Correction{CorrectionID: "cor_user", TargetObservationID: observation.ObservationID, SupersedesCorrectionIDs: []string{provider.CorrectionID}, BasedOnSourceRevision: &provider.CreatedAt,
		CreatedAt: start.Add(11 * time.Hour), Reason: CorrectionUserEdit, Changes: CorrectionChanges{StartAt: &userStart}}
	sessions, err := Fold([]Observation{observation}, []Correction{user, provider})
	if err != nil {
		t.Fatal(err)
	}
	interval := sessions[0].Intervals[0]
	if !interval.Interval.Start.UTC.Equal(userStart) || !interval.Interval.End.UTC.Equal(providerEnd) {
		t.Fatal("manual overlay lost the unedited provider endpoint")
	}
	if !interval.StartEvidence.RecordedAt.Equal(user.CreatedAt) {
		t.Fatal("correction timestamp was not preserved as evidence")
	}
}

func TestFoldAppliesOnlyActiveCorrectionLeafAndPreservesSuppression(t *testing.T) {
	start := time.Date(2026, 3, 2, 5, 0, 0, 0, time.UTC)
	firstStart := start.Add(15 * time.Minute)
	finalStart := start.Add(45 * time.Minute)
	excluded := true
	observations := []Observation{testObservation("obs_sleep_02", start, start.Add(8*time.Hour), ClassificationPrincipal)}
	corrections := []Correction{
		{
			CorrectionID:        "cor_sleep_02",
			TargetObservationID: "obs_sleep_02",
			CreatedAt:           start.Add(9 * time.Hour),
			Reason:              CorrectionUserEdit,
			Changes:             CorrectionChanges{StartAt: &firstStart},
		},
		{
			CorrectionID:            "cor_sleep_03",
			TargetObservationID:     "obs_sleep_02",
			SupersedesCorrectionIDs: []string{"cor_sleep_02"},
			CreatedAt:               start.Add(10 * time.Hour),
			Reason:                  CorrectionUserEdit,
			Changes: CorrectionChanges{
				StartAt:  &finalStart,
				Excluded: &excluded,
			},
		},
	}

	sessions, err := Fold(observations, corrections)
	if err != nil {
		t.Fatal(err)
	}
	if !sessions[0].Suppressed {
		t.Fatal("active suppression was not preserved")
	}
	if got := sessions[0].Intervals[0].Interval.Start.UTC; !got.Equal(finalStart) {
		t.Fatalf("effective start = %s, want %s", got, finalStart)
	}
	ids := sessions[0].Intervals[0].StartEvidence.CorrectionIDs
	if len(ids) != 1 || ids[0] != "cor_sleep_03" {
		t.Fatalf("active start correction ids = %v, want cor_sleep_03", ids)
	}
	if !observations[0].StartAt.Equal(start) {
		t.Fatal("fold mutated the source observation")
	}
}

func TestResolveOverlapsDoesNotPromoteUnknownOrSuppressedSleep(t *testing.T) {
	start := time.Date(2026, 3, 3, 5, 0, 0, 0, time.UTC)
	principal, err := SessionFromObservation(testObservation("obs_principal", start, start.Add(8*time.Hour), ClassificationPrincipal))
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := SessionFromObservation(testObservation("obs_unknown", start.Add(time.Hour), start.Add(7*time.Hour), ClassificationUnknown))
	if err != nil {
		t.Fatal(err)
	}
	suppressed, err := SessionFromObservation(testObservation("obs_suppressed", start.Add(2*time.Hour), start.Add(6*time.Hour), ClassificationPrincipal))
	if err != nil {
		t.Fatal(err)
	}
	suppressed.Suppressed = true

	resolved, err := ResolveOverlaps([]domain.SleepSession{principal, unknown, suppressed})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 3 {
		t.Fatalf("resolved sessions = %d, want separate principal, unknown, and suppressed sessions", len(resolved))
	}
	for _, session := range resolved {
		if session.Classification == domain.SleepClassificationUnknown && (session.IsNap || session.IsPrincipalSleep()) {
			t.Fatal("unknown session was collapsed into a nap or principal sleep")
		}
		if session.ID == "obs_suppressed" && !session.Suppressed {
			t.Fatal("suppression was lost during overlap resolution")
		}
	}
}

func TestUnknownClassificationDoesNotEnterEstimation(t *testing.T) {
	base := time.Date(2026, 3, 1, 5, 0, 0, 0, time.UTC)
	principal := make([]domain.SleepSession, 0, 8)
	for i := 0; i < 8; i++ {
		start := base.Add(time.Duration(i) * 25 * time.Hour)
		session, err := SessionFromObservation(testObservation(
			fmt.Sprintf("obs_principal_%02d", i),
			start,
			start.Add(8*time.Hour),
			ClassificationPrincipal,
		))
		if err != nil {
			t.Fatal(err)
		}
		principal = append(principal, session)
	}
	now := base.Add(10 * 25 * time.Hour)
	baseline, err := (estimation.RobustEstimator{}).Estimate(context.Background(), principal, now)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := SessionFromObservation(testObservation(
		"obs_unknown_outlier",
		base.Add(4*25*time.Hour+12*time.Hour),
		base.Add(4*25*time.Hour+20*time.Hour),
		ClassificationUnknown,
	))
	if err != nil {
		t.Fatal(err)
	}
	withUnknown, err := (estimation.RobustEstimator{}).Estimate(context.Background(), append(principal, unknown), now)
	if err != nil {
		t.Fatal(err)
	}
	if withUnknown.ObservedCycleLength != baseline.ObservedCycleLength || withUnknown.ObservedDriftPerCycle != baseline.ObservedDriftPerCycle {
		t.Fatalf("unknown sleep changed estimation: baseline=%#v withUnknown=%#v", baseline, withUnknown)
	}
}

func testObservation(id string, start, end time.Time, classification string) Observation {
	return Observation{
		ObservationID: id,
		Kind:          KindEpisode,
		StartAt:       start,
		EndAt:         end,
		ZoneID:        "America/New_York",
		Sleep:         Details{Classification: classification},
		Provenance: Provenance{
			AcquisitionMethod: AcquisitionSynthetic,
			EvidenceStatus:    EvidenceDirectlyObserved,
			RecordedAt:        end.Add(time.Minute),
		},
	}
}

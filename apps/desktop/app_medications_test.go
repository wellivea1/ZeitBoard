package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"non24.app/core/domain"
	medicationcore "non24.app/core/medication"
	storage "non24.app/core/storage/sqlite"
	"non24.app/desktop/platform/tray"
)

func TestMedicationLoggingUsesRealSleepContextAndAppendOnlyCorrections(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Now().UTC().Truncate(time.Minute)
	app.nowFn = func() time.Time { return fixedNow }
	seedSleepEntries(t, app, 12)

	initial, err := app.GetMedications()
	if err != nil {
		t.Fatal(err)
	}
	if !initial.Empty || initial.FixtureMode || len(initial.Medications) != 0 || len(initial.Events) != 0 {
		t.Fatalf("initial medications = %#v", initial)
	}
	created, err := app.AddMedication(MedicationInput{
		Label:         "Private test label",
		Form:          "tablet",
		StrengthLabel: "user-entered strength",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Empty || len(created.Medications) != 1 || created.Medications[0].ScheduleKind != "none" {
		t.Fatalf("created medications = %#v", created)
	}
	medicationID := created.Medications[0].MedicationID
	location := locationOrUTC(defaultZoneID)
	doseLocal := fixedNow.Add(-2 * time.Hour).In(location).Format("2006-01-02T15:04")
	logged, err := app.LogMedicationEvent(MedicationEventInput{
		MedicationID: medicationID,
		DoseLocal:    doseLocal,
		ZoneID:       defaultZoneID,
		Status:       storage.MedicationEventTaken,
		Scheduled:    true,
		Note:         "user-entered note",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(logged.Events) != 1 || logged.Events[0].MedicationLabel != "Private test label" || logged.Events[0].Status != storage.MedicationEventTaken {
		t.Fatalf("logged event = %#v", logged.Events)
	}
	if !strings.Contains(logged.Events[0].WakeRelation, "after recorded wake") || logged.Events[0].SleepRelationKind != "predicted" {
		t.Fatalf("rhythm context = %#v", logged.Events[0])
	}
	event := logged.Events[0]
	corrected, err := app.CorrectMedicationEvent(MedicationEventCorrectionInput{
		EventID:   event.EventID,
		DoseLocal: event.DoseLocal,
		ZoneID:    event.ZoneID,
		Status:    storage.MedicationEventSkipped,
		Scheduled: event.Scheduled,
		Note:      "corrected note",
		Excluded:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(corrected.Events) != 1 || corrected.Events[0].Status != storage.MedicationEventSkipped || corrected.Events[0].Note != "corrected note" || !corrected.Events[0].Excluded || corrected.Events[0].CorrectionCount != 1 {
		t.Fatalf("corrected event = %#v", corrected.Events)
	}
	if corrected.Medications[0].EventCount != 1 {
		t.Fatalf("excluded evidence changed the stored event count: %#v", corrected.Medications[0])
	}

	updated, err := app.UpdateMedication(MedicationUpdateInput{
		MedicationID:  medicationID,
		Revision:      corrected.Medications[0].Revision,
		Label:         "Renamed private label",
		Form:          "capsule",
		StrengthLabel: "revised user label",
		Active:        false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Medications[0].Active || updated.Medications[0].Revision != 2 || updated.Events[0].MedicationLabel != "Renamed private label" {
		t.Fatalf("updated medication = %#v", updated)
	}

	exported, err := app.ExportMedicationData()
	if err != nil {
		t.Fatal(err)
	}
	if exported.MedicationCount != 1 || exported.EventCount != 1 || !strings.HasSuffix(exported.FileName, ".json") {
		t.Fatalf("export = %#v", exported)
	}
	if !strings.Contains(exported.FileName, "-v2-") {
		t.Fatalf("schedule-capable export filename = %q", exported.FileName)
	}
	var payload storage.MedicationDataExport
	if err := json.Unmarshal([]byte(exported.JSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != "v2" {
		t.Fatalf("medication export version = %q", payload.SchemaVersion)
	}
	if payload.EventSet.Events[0].Status != storage.MedicationEventTaken || len(payload.EventSet.Corrections) != 1 {
		t.Fatal("export did not preserve raw event plus append-only correction")
	}

	if _, err := app.DeleteMedicationEvent(MedicationEventDeleteInput{EventID: event.EventID, Confirmation: "delete"}); err == nil {
		t.Fatal("event erasure accepted the wrong confirmation")
	}
	withoutEvent, err := app.DeleteMedicationEvent(MedicationEventDeleteInput{EventID: event.EventID, Confirmation: "DELETE"})
	if err != nil {
		t.Fatal(err)
	}
	if len(withoutEvent.Events) != 0 {
		t.Fatalf("hard-deleted event remained visible: %#v", withoutEvent.Events)
	}
	if _, err := app.DeleteMedication(MedicationDeleteInput{MedicationID: medicationID, Confirmation: "DELETE"}); err != nil {
		t.Fatal(err)
	}
	final, err := app.GetMedications()
	if err != nil {
		t.Fatal(err)
	}
	if !final.Empty || len(final.Medications) != 0 {
		t.Fatalf("hard-deleted medication remained visible: %#v", final)
	}
}

func TestMedicationLoggingRemainsUsableWithoutAnEstimate(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return fixedNow }
	created, err := app.AddMedication(MedicationInput{Label: "Private no-estimate record"})
	if err != nil {
		t.Fatal(err)
	}
	logged, err := app.LogMedicationEvent(MedicationEventInput{
		MedicationID: created.Medications[0].MedicationID,
		DoseLocal:    "2026-07-22T08:00",
		ZoneID:       "UTC",
		Status:       storage.MedicationEventTaken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if logged.EstimateStatus != "empty" || logged.Events[0].WakeRelation != "No prior recorded wake" || logged.Events[0].SleepRelationKind != "unavailable" {
		t.Fatalf("no-estimate medications = %#v", logged)
	}
	if !strings.Contains(logged.InteractionDisclaimer, "does not check medication interactions") {
		t.Fatalf("interaction disclaimer = %q", logged.InteractionDisclaimer)
	}
}

func TestMedicationLoggingRejectsFutureAndAdviceLikeInputs(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return fixedNow }
	created, err := app.AddMedication(MedicationInput{Label: "Private input validation"})
	if err != nil {
		t.Fatal(err)
	}
	medicationID := created.Medications[0].MedicationID
	if _, err := app.LogMedicationEvent(MedicationEventInput{
		MedicationID: medicationID,
		DoseLocal:    "2026-07-23T12:00",
		ZoneID:       "UTC",
		Status:       storage.MedicationEventTaken,
	}); err == nil {
		t.Fatal("future medication evidence was accepted")
	}
	if _, err := app.LogMedicationEvent(MedicationEventInput{
		MedicationID: medicationID,
		DoseLocal:    "2026-07-22T11:00",
		ZoneID:       "UTC",
		Status:       "recommended",
	}); err == nil {
		t.Fatal("non-evidence medication status was accepted")
	}
}

func TestMedicationScheduleUpdateProjectsExplicitRulesAndRealHorizon(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Date(2026, 7, 22, 12, 0, 30, 0, time.UTC)
	app.nowFn = func() time.Time { return fixedNow }
	created, err := app.AddMedication(MedicationInput{Label: "Private schedule label"})
	if err != nil {
		t.Fatal(err)
	}
	medication := created.Medications[0]
	updated, err := app.UpdateMedicationSchedule(MedicationScheduleInput{
		MedicationID:    medication.MedicationID,
		Revision:        medication.Revision,
		Kind:            storage.MedicationScheduleFixedClock,
		ZoneID:          "UTC",
		CivilTimes:      []string{"20:00", "08:00"},
		ReminderEnabled: true,
		ClinicianRule:   "Clinician instruction entered by the user",
	})
	if err != nil {
		t.Fatal(err)
	}
	medication = updated.Medications[0]
	if medication.Revision != 2 || medication.Schedule == nil || medication.Schedule.Kind != storage.MedicationScheduleFixedClock {
		t.Fatalf("updated schedule = %#v", medication)
	}
	if medication.Schedule.CivilTimes[0] != "08:00" || medication.Schedule.CivilTimes[1] != "20:00" {
		t.Fatalf("schedule times were not canonical: %#v", medication.Schedule.CivilTimes)
	}
	if medication.ClinicianRule != "Clinician instruction entered by the user" || !strings.Contains(medication.ClinicianRuleAttribution, "entered verbatim by you") {
		t.Fatalf("clinician rule attribution = %#v", medication)
	}
	forecast := medication.Schedule.Forecast
	if forecast.Status != "unavailable" || forecast.OutsideHorizonCount != 28 || len(forecast.Occurrences) != 28 {
		t.Fatalf("no-estimate forecast = %#v", forecast)
	}
	if updated.ReminderStatus != "unavailable" || !strings.Contains(updated.ReminderMessage, "not running") {
		t.Fatalf("unstarted reminder status = %q, %q", updated.ReminderStatus, updated.ReminderMessage)
	}
	if _, err := app.UpdateMedicationSchedule(MedicationScheduleInput{
		MedicationID: medication.MedicationID,
		Revision:     1,
		Kind:         "none",
	}); !errors.Is(err, storage.ErrMedicationRevisionConflict) {
		t.Fatalf("stale schedule update error = %v", err)
	}
	if _, err := app.UpdateMedicationSchedule(MedicationScheduleInput{
		MedicationID:    medication.MedicationID,
		Revision:        medication.Revision,
		Kind:            "none",
		ReminderEnabled: true,
	}); err == nil {
		t.Fatal("removed schedule retained a reminder")
	}

	exported, err := app.ExportMedicationData()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exported.JSON, `"zone_id": "UTC"`) || !strings.Contains(exported.JSON, `"reminder_enabled": true`) || !strings.Contains(exported.JSON, `"clinician_rule"`) {
		t.Fatalf("schedule export omitted contract fields: %s", exported.JSON)
	}
}

func TestMedicationScheduleForecastUsesNeutralCoveredAndUnknownStates(t *testing.T) {
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	confidence := domain.InferenceConfidence{Level: domain.ConfidenceMedium}
	state := localEstimateState{
		Status: "estimated",
		Estimate: domain.PhaseEstimate{
			ID:         "estimate_medication_dto",
			Confidence: confidence,
			PredictedSleepWindows: []domain.AvailabilityWindow{
				{
					ID:         "sleep_medication_dto_01",
					Interval:   domain.TimeRange{Start: domain.MustZonedInstant(time.Date(2026, 7, 22, 11, 0, 0, 0, time.UTC), "UTC"), End: domain.MustZonedInstant(time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC), "UTC")},
					Confidence: confidence,
				},
				{
					ID:         "sleep_medication_dto_02",
					Interval:   domain.TimeRange{Start: domain.MustZonedInstant(time.Date(2026, 7, 23, 11, 0, 0, 0, time.UTC), "UTC"), End: domain.MustZonedInstant(time.Date(2026, 7, 23, 15, 0, 0, 0, time.UTC), "UTC")},
					Confidence: confidence,
				},
			},
		},
	}
	projected, err := medicationScheduleDTO(storage.MedicationSchedule{
		Kind:       storage.MedicationScheduleFixedClock,
		ZoneID:     "UTC",
		CivilTimes: []string{"12:00", "18:00"},
	}, state, now)
	if err != nil {
		t.Fatal(err)
	}
	forecast := projected.Forecast
	if forecast.Status != "collision" || forecast.CoveredCount != 3 || forecast.CollisionCount != 2 || forecast.OutsideHorizonCount == 0 {
		t.Fatalf("forecast = %#v", forecast)
	}
	wantContext := []string{
		"Inside a current predicted sleep window",
		"Not inside a current predicted sleep window",
		"Inside a current predicted sleep window",
		"Outside the current forecast horizon",
	}
	for index, want := range wantContext {
		if forecast.Occurrences[index].Context != want {
			t.Fatalf("occurrence %d context = %q, want %q", index, forecast.Occurrences[index].Context, want)
		}
	}
	for _, prohibited := range []string{"safe", "unsafe", "right time", "wrong time"} {
		if strings.Contains(strings.ToLower(forecast.Message), prohibited) {
			t.Fatalf("forecast message uses judgmental wording: %q", forecast.Message)
		}
	}
	gapOnly := medicationForecastMessage(medicationcore.CollisionForecast{Status: medicationcore.ForecastUnavailable}, 0, 1)
	if gapOnly != "No occurrence is generated for the reported nonexistent civil time; scheduled times remain unchanged." {
		t.Fatalf("gap-only forecast message = %q", gapOnly)
	}
}

func TestMedicationReminderDeliveryClaimsBeforeNotifyAndNeverRetries(t *testing.T) {
	app := newTestApp(t)
	now := time.Date(2026, 7, 22, 12, 0, 30, 0, time.UTC)
	app.nowFn = func() time.Time { return now }
	fake := &medicationReminderTestTray{}
	app.tray = fake
	created, err := app.AddMedication(MedicationInput{Label: "Private\nreminder label"})
	if err != nil {
		t.Fatal(err)
	}
	medication := created.Medications[0]
	if _, err := app.UpdateMedicationSchedule(MedicationScheduleInput{
		MedicationID:    medication.MedicationID,
		Revision:        medication.Revision,
		Kind:            storage.MedicationScheduleFixedClock,
		ZoneID:          "UTC",
		CivilTimes:      []string{"12:00"},
		ReminderEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	first, err := app.deliverMedicationReminders(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Due != 1 || first.Claimed != 1 || first.Shown != 1 || first.Failed != 0 {
		t.Fatalf("first delivery = %#v", first)
	}
	second, err := app.deliverMedicationReminders(context.Background(), now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if second.Due != 1 || second.Claimed != 0 || second.Shown != 0 {
		t.Fatalf("duplicate delivery = %#v", second)
	}
	notifications := fake.snapshot()
	if len(notifications) != 1 || notifications[0] != "Medication reminder|Reminder you set: Private reminder label." {
		t.Fatalf("notifications = %#v", notifications)
	}
	current, err := app.GetMedications()
	if err != nil {
		t.Fatal(err)
	}
	medication = current.Medications[0]
	archived, err := app.UpdateMedication(MedicationUpdateInput{
		MedicationID:  medication.MedicationID,
		Revision:      medication.Revision,
		Label:         medication.Label,
		Form:          medication.Form,
		StrengthLabel: medication.StrengthLabel,
		Active:        false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if archived.Medications[0].Schedule == nil {
		t.Fatal("archiving removed the user-authored schedule")
	}
	paused, err := app.deliverMedicationReminders(context.Background(), now.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if paused.Due != 0 || len(fake.snapshot()) != 1 {
		t.Fatalf("inactive medication generated a reminder: %#v", paused)
	}

	failureApp := newTestApp(t)
	failureApp.nowFn = func() time.Time { return now }
	failureTray := &medicationReminderTestTray{notifyErr: errors.New("synthetic notification failure")}
	failureApp.tray = failureTray
	created, err = failureApp.AddMedication(MedicationInput{Label: "Never leak this label in status"})
	if err != nil {
		t.Fatal(err)
	}
	medication = created.Medications[0]
	if _, err := failureApp.UpdateMedicationSchedule(MedicationScheduleInput{
		MedicationID:    medication.MedicationID,
		Revision:        medication.Revision,
		Kind:            storage.MedicationScheduleFixedClock,
		ZoneID:          "UTC",
		CivilTimes:      []string{"12:00"},
		ReminderEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	failed, err := failureApp.deliverMedicationReminders(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Claimed != 1 || failed.Failed != 1 || failed.Shown != 0 {
		t.Fatalf("failed delivery = %#v", failed)
	}
	repeated, err := failureApp.deliverMedicationReminders(context.Background(), now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Claimed != 0 || len(failureTray.snapshot()) != 1 {
		t.Fatalf("failed reminder was retried: %#v, %#v", repeated, failureTray.snapshot())
	}
	state := failureApp.medicationReminderServiceState()
	if state.LastError == "" || strings.Contains(state.LastError, "Never leak") {
		t.Fatalf("notification error leaked a private label: %#v", state)
	}
}

type medicationReminderTestTray struct {
	mu            sync.Mutex
	notifications []string
	notifyErr     error
}

func (*medicationReminderTestTray) Start(tray.Callbacks) error { return nil }

func (fake *medicationReminderTestTray) Notify(title, message string) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.notifications = append(fake.notifications, title+"|"+message)
	return fake.notifyErr
}

func (*medicationReminderTestTray) Stop() error { return nil }

func (fake *medicationReminderTestTray) snapshot() []string {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]string(nil), fake.notifications...)
}

func TestMedicationSleepIndexUsesPrincipalHalfOpenIntervals(t *testing.T) {
	firstStart := time.Date(2026, 7, 20, 4, 0, 0, 0, time.UTC)
	secondStart := firstStart.Add(25 * time.Hour)
	interval := func(start time.Time) domain.SleepInterval {
		return domain.SleepInterval{Interval: domain.TimeRange{
			Start: domain.MustZonedInstant(start, "UTC"),
			End:   domain.MustZonedInstant(start.Add(8*time.Hour), "UTC"),
		}}
	}
	sessions := []domain.SleepSession{
		{
			ID: "principal_later", Classification: domain.SleepClassificationPrincipal,
			Intervals: []domain.SleepInterval{interval(secondStart)},
		},
		{
			ID: "unknown_overlap", Classification: domain.SleepClassificationUnknown,
			Intervals: []domain.SleepInterval{interval(firstStart.Add(time.Hour))},
		},
		{
			ID: "principal_first", Classification: domain.SleepClassificationPrincipal,
			Intervals: []domain.SleepInterval{interval(firstStart)},
		},
		{
			ID: "suppressed", Classification: domain.SleepClassificationPrincipal, Suppressed: true,
			Intervals: []domain.SleepInterval{interval(firstStart.Add(10 * time.Hour))},
		},
	}

	index := newMedicationSleepIndex(sessions)
	if len(index.intervals) != 2 {
		t.Fatalf("principal interval count = %d, want 2", len(index.intervals))
	}
	inside, ok := index.containing(firstStart.Add(time.Hour))
	if !ok || !inside.Interval.Start.UTC.Equal(firstStart) {
		t.Fatalf("containing interval = %#v ok=%v", inside, ok)
	}
	if _, ok := index.containing(firstStart.Add(8 * time.Hour)); ok {
		t.Fatal("sleep end was treated as inside a half-open interval")
	}
	next, ok := index.next(firstStart.Add(8 * time.Hour))
	if !ok || !next.Interval.Start.UTC.Equal(secondStart) {
		t.Fatalf("next interval = %#v ok=%v", next, ok)
	}
}

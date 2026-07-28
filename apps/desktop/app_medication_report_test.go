package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
	storage "non24.app/core/storage/sqlite"
)

func TestMedicationClinicianReportUsesRealRecordsAndEnforcesRedaction(t *testing.T) {
	app := newTestApp(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return now }
	startedAt := time.Date(2026, 7, 1, 18, 0, 0, 0, time.UTC)
	seedAssociationSleep(t, app, startedAt)

	created, err := app.AddMedication(MedicationInput{
		Label:         "Private & clinical label",
		Form:          "tablet",
		StrengthLabel: "private strength",
		StartedLocal:  startedAt.Format("2006-01-02T15:04"),
		StartedZoneID: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	medication := created.Medications[0]
	if medication.StartedAt != startedAt.Format(time.RFC3339) || medication.StartedLocal != "2026-07-01T18:00" || medication.StartedZoneID != "UTC" {
		t.Fatalf("start marker projection = %#v", medication)
	}

	for _, input := range []MedicationEventInput{
		{MedicationID: medication.MedicationID, DoseLocal: "2026-07-01T20:00", ZoneID: "UTC", Status: storage.MedicationEventTaken, Scheduled: true, Note: "<private taken note>"},
		{MedicationID: medication.MedicationID, DoseLocal: "2026-07-02T20:10", ZoneID: "UTC", Status: storage.MedicationEventSkipped, Scheduled: true, Note: "private skipped note"},
		{MedicationID: medication.MedicationID, DoseLocal: "2026-07-03T20:20", ZoneID: "UTC", Status: storage.MedicationEventTaken, Scheduled: false, Note: "private as-needed note"},
	} {
		if _, err := app.LogMedicationEvent(input); err != nil {
			t.Fatal(err)
		}
	}
	excluded, err := app.LogMedicationEvent(MedicationEventInput{
		MedicationID: medication.MedicationID,
		DoseLocal:    "2026-07-04T20:30",
		ZoneID:       "UTC",
		Status:       storage.MedicationEventTaken,
		Scheduled:    true,
		Note:         "private excluded note",
	})
	if err != nil {
		t.Fatal(err)
	}
	excludedEvent := excluded.Events[0]
	if _, err := app.CorrectMedicationEvent(MedicationEventCorrectionInput{
		EventID: excludedEvent.EventID, DoseLocal: excludedEvent.DoseLocal, ZoneID: excludedEvent.ZoneID,
		Status: excludedEvent.Status, Scheduled: excludedEvent.Scheduled, Note: excludedEvent.Note, Excluded: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddRhythmMarker(RhythmMarkerInput{
		Kind: storage.RhythmMarkerForcedSchedule, StartLocal: "2026-07-01T19:00", EndLocal: "2026-07-02T03:00", ZoneID: "UTC", Note: "private light intervention context",
	}); err != nil {
		t.Fatal(err)
	}

	input := MedicationClinicalReportInput{
		RangeMode: "custom", FromDate: "2026-06-24", ToDate: "2026-07-09", ZoneID: "UTC", DayStartHour: 18,
		IncludeMedication: true, IncludeRhythmContext: true,
	}
	report, err := app.GetMedicationClinicianReport(input)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "partial" || report.Summary.CalendarRows != 16 || report.Summary.MedicationEvents != 3 || report.Summary.NoDataRows != 3 {
		t.Fatalf("report summary = %#v", report.Summary)
	}
	if report.Summary.RecordedScheduled != 2 || report.Summary.RecordedTaken != 1 || report.Summary.RecordedSkipped != 1 || report.Summary.ExcludedEvents != 1 {
		t.Fatalf("recorded adherence counts = %#v", report.Summary)
	}
	if len(report.Adherence) != 1 || report.Adherence[0].MedicationLabel != "Medication 1" || !strings.Contains(report.Adherence[0].Summary, "No unlogged dose is counted as missed") {
		t.Fatalf("adherence summary = %#v", report.Adherence)
	}
	if len(report.Associations) != 1 || report.Associations[0].Status != "available" || len(report.Associations[0].Context) != 1 {
		t.Fatalf("association = %#v", report.Associations)
	}
	if !strings.Contains(report.Associations[0].Message, "does not establish cause") || report.Associations[0].Context[0].Note != "" {
		t.Fatalf("association boundary = %#v", report.Associations[0])
	}
	if !reportLegendHas(report.Actogram.Legend, "medication_taken") || !reportLegendHas(report.Actogram.Legend, "medication_skipped") || !reportLegendHas(report.Actogram.Legend, "medication_start") || !reportLegendHas(report.Actogram.Legend, "context_forced_schedule") || reportLegendHas(report.Actogram.Legend, "forecast") {
		t.Fatalf("report legend = %#v", report.Actogram.Legend)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"zoneId"`) {
		t.Fatal("redacted report retained an IANA location identifier")
	}
	for _, private := range []string{"Private & clinical label", "private strength", "private taken note", "private skipped note", "private light intervention context"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("redacted report leaked %q", private)
		}
	}
	if _, err := app.ExportMedicationClinicianReport(MedicationClinicalReportExportInput{Report: input, Confirmation: "export"}); err == nil {
		t.Fatal("clinician export accepted the wrong confirmation")
	}
	exported, err := app.ExportMedicationClinicianReport(MedicationClinicalReportExportInput{Report: input, Confirmation: "EXPORT"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(exported.FileName, ".html") || !strings.Contains(exported.HTML, "@page") || !strings.Contains(exported.HTML, "default-src 'none'") || !strings.Contains(exported.HTML, `class="drift-band"`) || strings.Contains(exported.HTML, "<script") {
		t.Fatalf("printable export boundary was not preserved: %q", exported.FileName)
	}
	if strings.TrimSpace(exported.HTML) != exported.HTML {
		t.Fatal("printable export was not returned as a canonical HTML document")
	}
	if !strings.Contains(exported.HTML, "Clinical chart text alternative for every calendar row") ||
		!strings.Contains(exported.HTML, "Clinician-entered medication guidance omitted") {
		t.Fatal("printable export omitted its full chart alternative or mandatory redaction")
	}
	for _, private := range []string{"Private & clinical label", "private taken note", "private light intervention context"} {
		if strings.Contains(exported.HTML, private) {
			t.Fatalf("redacted HTML leaked %q", private)
		}
	}

	input.IncludeMedicationLabels = true
	input.IncludeMedicationNotes = true
	input.IncludeRhythmContextNotes = true
	privateExport, err := app.ExportMedicationClinicianReport(MedicationClinicalReportExportInput{Report: input, Confirmation: "EXPORT"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(privateExport.HTML, "Private &amp; clinical label") || !strings.Contains(privateExport.HTML, "&lt;private taken note&gt;") || !strings.Contains(privateExport.HTML, "private light intervention context") {
		t.Fatal("explicitly included private report fields were missing or not HTML-escaped")
	}
}

func TestMedicationClinicianReportSixPMAnchorKeepsOvernightSleepOnOneRow(t *testing.T) {
	app := newTestApp(t)
	app.nowFn = func() time.Time { return time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC) }
	if _, err := app.AddSleepEntry(SleepEntryInput{StartLocal: "2026-07-01T22:00", EndLocal: "2026-07-02T06:00", ZoneID: "UTC", Classification: storage.SleepClassificationPrincipal}); err != nil {
		t.Fatal(err)
	}
	report, err := app.GetMedicationClinicianReport(MedicationClinicalReportInput{RangeMode: "custom", FromDate: "2026-07-01", ToDate: "2026-07-02", ZoneID: "UTC", DayStartHour: 18})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Actogram.Rows[0].Sleep) != 1 || len(report.Actogram.Rows[1].Sleep) != 0 {
		t.Fatalf("overnight sleep split across clinical rows: %#v", report.Actogram.Rows)
	}
	segment := report.Actogram.Rows[0].Sleep[0]
	if segment.StartPercent != 16.67 || segment.WidthPercent != 33.33 {
		t.Fatalf("overnight civil placement = %#v", segment)
	}
}

func TestMedicationClinicianReportDriftUsesOnlySelectedRange(t *testing.T) {
	app := newTestApp(t)
	app.nowFn = func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) }
	for _, series := range []struct {
		start  time.Time
		drift  time.Duration
		prefix string
	}{
		{start: time.Date(2026, 6, 1, 22, 0, 0, 0, time.UTC), drift: 30 * time.Minute, prefix: "June"},
		{start: time.Date(2026, 7, 1, 22, 0, 0, 0, time.UTC), drift: 50 * time.Minute, prefix: "July"},
	} {
		for index := 0; index < 7; index++ {
			start := series.start.Add(time.Duration(index) * (24*time.Hour + series.drift))
			if _, err := app.AddSleepEntry(SleepEntryInput{
				StartLocal: start.Format("2006-01-02T15:04"),
				EndLocal:   start.Add(8 * time.Hour).Format("2006-01-02T15:04"),
				ZoneID:     "UTC", Classification: storage.SleepClassificationPrincipal,
			}); err != nil {
				t.Fatalf("seed %s range: %v", series.prefix, err)
			}
		}
	}

	report, err := app.GetMedicationClinicianReport(MedicationClinicalReportInput{
		RangeMode: "custom", FromDate: "2026-06-01", ToDate: "2026-06-12", ZoneID: "UTC", DayStartHour: 18,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift.Status != "estimated" || !strings.Contains(report.Drift.SlopeLabel, "+30") || len(report.Drift.Points) != 7 {
		t.Fatalf("selected-range drift = %#v", report.Drift)
	}
	for _, point := range report.Drift.Points {
		if point.CivilDate < "2026-06-01" || point.CivilDate > "2026-06-12" {
			t.Fatalf("out-of-range drift point = %#v", point)
		}
	}
}

func TestMedicationClinicianReportAssociationDoesNotReadBeyondSelectedRange(t *testing.T) {
	app := newTestApp(t)
	app.nowFn = func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) }
	startedAt := time.Date(2026, 7, 1, 18, 0, 0, 0, time.UTC)
	seedAssociationSleep(t, app, startedAt)
	if _, err := app.AddMedication(MedicationInput{
		Label: "Range boundary", StartedLocal: "2026-07-01T18:00", StartedZoneID: "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []RhythmMarkerInput{
		{Kind: storage.RhythmMarkerIllness, StartLocal: "2026-06-28T19:00", ZoneID: "UTC", Note: "outside selected range"},
		{Kind: storage.RhythmMarkerDisruption, StartLocal: "2026-07-01T19:00", ZoneID: "UTC", Note: "inside selected range"},
	} {
		if _, err := app.AddRhythmMarker(marker); err != nil {
			t.Fatal(err)
		}
	}

	report, err := app.GetMedicationClinicianReport(MedicationClinicalReportInput{
		RangeMode: "custom", FromDate: "2026-06-29", ToDate: "2026-07-03", ZoneID: "UTC", DayStartHour: 18,
		IncludeMedication: true, IncludeRhythmContext: true, IncludeRhythmContextNotes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Associations) != 1 {
		t.Fatalf("associations = %#v", report.Associations)
	}
	association := report.Associations[0]
	if association.Status != estimation.TemporalAssociationInsufficient || association.Before.EpisodeCount >= 5 || association.After.EpisodeCount >= 5 {
		t.Fatalf("range-limited association = %#v", association)
	}
	if len(association.Context) != 1 || association.Context[0].Note != "inside selected range" {
		t.Fatalf("range-limited context = %#v", association.Context)
	}
}

func TestMedicationClinicianReportCivilRowsHonorDSTLength(t *testing.T) {
	for _, test := range []struct {
		name string
		date string
		want time.Duration
	}{
		{name: "spring forward", date: "2026-03-07", want: 23 * time.Hour},
		{name: "fall back", date: "2026-10-31", want: 25 * time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			reportRange, err := resolveMedicationReportRange(MedicationClinicalReportInput{
				RangeMode: "custom", FromDate: test.date, ToDate: test.date,
				ZoneID: "America/New_York", DayStartHour: 18,
			}, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := reportRange.endAt.Sub(reportRange.startAt); got != test.want {
				t.Fatalf("civil row duration = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMedicationReportOverlappingRowsUseDSTBoundaries(t *testing.T) {
	starts := []time.Time{
		time.Date(2026, 3, 7, 23, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 8, 22, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 9, 22, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 10, 22, 0, 0, 0, time.UTC),
	}
	tests := []struct {
		name                string
		start, end          time.Time
		wantFirst, wantLast int
	}{
		{name: "clipped at report start", start: starts[0].Add(-time.Hour), end: starts[0].Add(time.Hour), wantFirst: 0, wantLast: 1},
		{name: "crosses short DST row", start: starts[0].Add(22 * time.Hour), end: starts[1].Add(time.Hour), wantFirst: 0, wantLast: 2},
		{name: "ends at row boundary", start: starts[0].Add(time.Hour), end: starts[1], wantFirst: 0, wantLast: 1},
		{name: "outside before report", start: starts[0].Add(-2 * time.Hour), end: starts[0].Add(-time.Hour)},
		{name: "outside after report", start: starts[3], end: starts[3].Add(time.Hour)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, last := medicationReportOverlappingRows(starts, test.start, test.end)
			if first != test.wantFirst || last != test.wantLast {
				t.Fatalf("indexes = [%d,%d), want [%d,%d)", first, last, test.wantFirst, test.wantLast)
			}
		})
	}
}

func seedAssociationSleep(t *testing.T, app *App, startedAt time.Time) {
	t.Helper()
	beforeStart := startedAt.Add(-6 * (24*time.Hour + 50*time.Minute))
	for _, series := range []struct {
		start  time.Time
		count  int
		period time.Duration
	}{
		{start: beforeStart, count: 6, period: 24*time.Hour + 50*time.Minute},
		{start: startedAt, count: 6, period: 24*time.Hour + 10*time.Minute},
	} {
		for index := 0; index < series.count; index++ {
			start := series.start.Add(time.Duration(index) * series.period)
			if _, err := app.AddSleepEntry(SleepEntryInput{
				StartLocal: start.Format("2006-01-02T15:04"), EndLocal: start.Add(8 * time.Hour).Format("2006-01-02T15:04"),
				ZoneID: "UTC", Classification: storage.SleepClassificationPrincipal,
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func reportLegendHas(legend []MedicationClinicalLegendDTO, kind string) bool {
	for _, item := range legend {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func TestMedicationStartedValuesRejectFutureAndIncompletePairs(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	if _, _, err := medicationStartedValues("2026-06-01T08:00", "", now); err == nil {
		t.Fatal("incomplete start pair was accepted")
	}
	if _, _, err := medicationStartedValues("2026-07-02T08:00", "UTC", now); err == nil {
		t.Fatal("future start was accepted")
	}
	started, zoneID, err := medicationStartedValues("2026-06-01T08:00", "UTC", now)
	if err != nil || started == nil || zoneID != "UTC" || !started.Equal(time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("valid start = %v, %q, %v", started, zoneID, err)
	}
	if _, err := domain.NewZonedInstant(*started, zoneID); err != nil {
		t.Fatal(err)
	}
}

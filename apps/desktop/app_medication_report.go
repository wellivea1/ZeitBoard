package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
	storage "non24.app/core/storage/sqlite"
)

const (
	medicationReportExportConfirmation = "EXPORT"
	medicationReportMaximumRows        = 3660
	medicationReportNotice             = "This report summarizes recorded sleep, medication events, and self-reported context. Estimated windows are uncertain. The report does not diagnose, establish treatment effects, recommend medication timing, or check interactions."
)

type MedicationClinicalReportInput struct {
	RangeMode                 string `json:"rangeMode"`
	FromDate                  string `json:"fromDate"`
	ToDate                    string `json:"toDate"`
	ZoneID                    string `json:"zoneId"`
	DayStartHour              int    `json:"dayStartHour"`
	IncludeForecast           bool   `json:"includeForecast"`
	IncludeMedication         bool   `json:"includeMedication"`
	IncludeMedicationLabels   bool   `json:"includeMedicationLabels"`
	IncludeMedicationNotes    bool   `json:"includeMedicationNotes"`
	IncludeRhythmContext      bool   `json:"includeRhythmContext"`
	IncludeRhythmContextNotes bool   `json:"includeRhythmContextNotes"`
}

type MedicationClinicalReportExportInput struct {
	Report       MedicationClinicalReportInput `json:"report"`
	Confirmation string                        `json:"confirmation"`
}

type MedicationClinicalReportRangeDTO struct {
	Mode          string `json:"mode"`
	FromDate      string `json:"fromDate"`
	ToDate        string `json:"toDate"`
	Label         string `json:"label"`
	DayStartHour  int    `json:"dayStartHour"`
	DayStartLabel string `json:"dayStartLabel"`
}

type MedicationClinicalReportSummaryDTO struct {
	CalendarRows          int `json:"calendarRows"`
	ObservedSleepSegments int `json:"observedSleepSegments"`
	NoDataRows            int `json:"noDataRows"`
	MedicationEvents      int `json:"medicationEvents"`
	RecordedScheduled     int `json:"recordedScheduled"`
	RecordedTaken         int `json:"recordedTaken"`
	RecordedSkipped       int `json:"recordedSkipped"`
	ExcludedEvents        int `json:"excludedEvents"`
	RhythmContextMarkers  int `json:"rhythmContextMarkers"`
}

type MedicationClinicalSleepSegmentDTO struct {
	Kind          string  `json:"kind"`
	StartPercent  float64 `json:"startPercent"`
	WidthPercent  float64 `json:"widthPercent"`
	StartLabel    string  `json:"startLabel"`
	WakeLabel     string  `json:"wakeLabel"`
	DurationLabel string  `json:"durationLabel"`
	Source        string  `json:"source"`
	Confidence    string  `json:"confidence"`
}

type MedicationClinicalAnnotationDTO struct {
	Kind            string  `json:"kind"`
	PositionPercent float64 `json:"positionPercent"`
	Label           string  `json:"label"`
	AtLabel         string  `json:"atLabel"`
	Detail          string  `json:"detail,omitempty"`
}

type MedicationClinicalActogramRowDTO struct {
	CivilDate   string                              `json:"civilDate"`
	DayLabel    string                              `json:"dayLabel"`
	MonthLabel  string                              `json:"monthLabel,omitempty"`
	Weekend     bool                                `json:"weekend"`
	NoData      bool                                `json:"noData"`
	Sleep       []MedicationClinicalSleepSegmentDTO `json:"sleep"`
	Annotations []MedicationClinicalAnnotationDTO   `json:"annotations"`
}

type MedicationClinicalLegendDTO struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type MedicationClinicalActogramDTO struct {
	AxisLabels []string                           `json:"axisLabels"`
	Rows       []MedicationClinicalActogramRowDTO `json:"rows"`
	Legend     []MedicationClinicalLegendDTO      `json:"legend"`
	Summary    string                             `json:"summary"`
}

type MedicationClinicalDriftPointDTO struct {
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

type MedicationClinicalDriftDTO struct {
	Status     string                            `json:"status"`
	SlopeLabel string                            `json:"slopeLabel"`
	Confidence string                            `json:"confidence"`
	Summary    string                            `json:"summary"`
	YMinHour   float64                           `json:"yMinHour"`
	YMaxHour   float64                           `json:"yMaxHour"`
	Points     []MedicationClinicalDriftPointDTO `json:"points"`
}

type MedicationClinicalAdherenceSummaryDTO struct {
	MedicationLabel   string `json:"medicationLabel"`
	RecordedScheduled int    `json:"recordedScheduled"`
	Taken             int    `json:"taken"`
	Skipped           int    `json:"skipped"`
	AsNeeded          int    `json:"asNeeded"`
	Summary           string `json:"summary"`
}

type MedicationClinicalAdherenceEventDTO struct {
	MedicationLabel string `json:"medicationLabel"`
	CivilTime       string `json:"civilTime"`
	Status          string `json:"status"`
	ScheduleContext string `json:"scheduleContext"`
	WakeContext     string `json:"wakeContext"`
	SleepContext    string `json:"sleepContext"`
	Confidence      string `json:"confidence"`
	Note            string `json:"note,omitempty"`
}

type MedicationClinicalAssociationSegmentDTO struct {
	EpisodeCount int    `json:"episodeCount"`
	RangeLabel   string `json:"rangeLabel"`
	SlopeLabel   string `json:"slopeLabel"`
	Confidence   string `json:"confidence"`
}

type MedicationClinicalAssociationContextDTO struct {
	KindLabel   string `json:"kindLabel"`
	RangeLabel  string `json:"rangeLabel"`
	TimingLabel string `json:"timingLabel"`
	Note        string `json:"note,omitempty"`
}

type MedicationClinicalAssociationDTO struct {
	MedicationLabel string                                    `json:"medicationLabel"`
	StartedLabel    string                                    `json:"startedLabel"`
	Status          string                                    `json:"status"`
	Message         string                                    `json:"message"`
	Before          MedicationClinicalAssociationSegmentDTO   `json:"before"`
	After           MedicationClinicalAssociationSegmentDTO   `json:"after"`
	Context         []MedicationClinicalAssociationContextDTO `json:"context"`
}

type MedicationClinicalReportDTO struct {
	Status         string                                  `json:"status"`
	Message        string                                  `json:"message"`
	GeneratedAt    string                                  `json:"generatedAt"`
	GeneratedLabel string                                  `json:"generatedLabel"`
	Range          MedicationClinicalReportRangeDTO        `json:"range"`
	Summary        MedicationClinicalReportSummaryDTO      `json:"summary"`
	Redactions     []string                                `json:"redactions"`
	Actogram       MedicationClinicalActogramDTO           `json:"actogram"`
	Drift          MedicationClinicalDriftDTO              `json:"drift"`
	Adherence      []MedicationClinicalAdherenceSummaryDTO `json:"adherence"`
	Events         []MedicationClinicalAdherenceEventDTO   `json:"events"`
	Associations   []MedicationClinicalAssociationDTO      `json:"associations"`
	Provenance     []string                                `json:"provenance"`
	Notice         string                                  `json:"notice"`
}

type MedicationClinicalReportExportDTO struct {
	FileName       string   `json:"fileName"`
	HTML           string   `json:"html"`
	GeneratedAt    string   `json:"generatedAt"`
	GeneratedLabel string   `json:"generatedLabel"`
	RowCount       int      `json:"rowCount"`
	EventCount     int      `json:"eventCount"`
	Redactions     []string `json:"redactions"`
}

type medicationReportRange struct {
	mode     string
	fromDate time.Time
	toDate   time.Time
	location *time.Location
	zoneID   string
	dayStart int
	startAt  time.Time
	endAt    time.Time
}

func (a *App) GetMedicationClinicianReport(input MedicationClinicalReportInput) (MedicationClinicalReportDTO, error) {
	return a.medicationClinicianReportAt(a.applicationContext(), input, a.currentTime().UTC().Truncate(time.Second))
}

func (a *App) ExportMedicationClinicianReport(input MedicationClinicalReportExportInput) (MedicationClinicalReportExportDTO, error) {
	if strings.TrimSpace(input.Confirmation) != medicationReportExportConfirmation {
		return MedicationClinicalReportExportDTO{}, errors.New("type EXPORT to confirm creation of a clinician report file")
	}
	report, err := a.medicationClinicianReportAt(a.applicationContext(), input.Report, a.currentTime().UTC().Truncate(time.Second))
	if err != nil {
		return MedicationClinicalReportExportDTO{}, err
	}
	html, err := renderMedicationClinicalReportHTML(report)
	if err != nil {
		return MedicationClinicalReportExportDTO{}, err
	}
	return MedicationClinicalReportExportDTO{
		FileName:       "zeitboard-clinician-report-" + report.Range.FromDate + "-to-" + report.Range.ToDate + ".html",
		HTML:           html,
		GeneratedAt:    report.GeneratedAt,
		GeneratedLabel: report.GeneratedLabel,
		RowCount:       report.Summary.CalendarRows,
		EventCount:     report.Summary.MedicationEvents,
		Redactions:     append([]string(nil), report.Redactions...),
	}, nil
}

func (a *App) medicationClinicianReportAt(ctx context.Context, input MedicationClinicalReportInput, now time.Time) (MedicationClinicalReportDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return MedicationClinicalReportDTO{}, err
	}
	medications, err := store.ListMedications(ctx)
	if err != nil {
		return MedicationClinicalReportDTO{}, err
	}
	events, err := store.EffectiveMedicationEvents(ctx)
	if err != nil {
		return MedicationClinicalReportDTO{}, err
	}
	markers, err := store.ListRhythmMarkers(ctx)
	if err != nil {
		return MedicationClinicalReportDTO{}, err
	}
	state, err := a.localEstimate(ctx, now)
	if err != nil {
		return MedicationClinicalReportDTO{}, err
	}
	reportRange, err := resolveMedicationReportRange(input, now, state.Sessions, medications, events, markers)
	if err != nil {
		return MedicationClinicalReportDTO{}, err
	}
	aliases := medicationReportAliases(medications, input.IncludeMedicationLabels)
	rows, rowStarts, err := medicationReportRows(reportRange)
	if err != nil {
		return MedicationClinicalReportDTO{}, err
	}
	legendKinds := map[string]bool{}
	observedSegments := addMedicationReportSleep(rows, rowStarts, reportRange, state.Sessions, legendKinds)
	if input.IncludeForecast && state.Status == "estimated" {
		addMedicationReportForecast(rows, rowStarts, reportRange, state.Estimate, legendKinds)
	}
	_, contextCount := addMedicationReportAnnotations(rows, reportRange, medications, events, markers, aliases, input, legendKinds)
	noDataRows := 0
	for index := range rows {
		hasRecordedSleep := false
		for _, segment := range rows[index].Sleep {
			if segment.Kind != "forecast" {
				hasRecordedSleep = true
				break
			}
		}
		rows[index].NoData = !hasRecordedSleep
		if rows[index].NoData {
			noDataRows++
		}
	}
	adherence, eventRows, adherenceCounts := medicationReportAdherence(events, medications, aliases, state, reportRange, input)
	associations := medicationReportAssociations(medications, aliases, markers, state.Sessions, reportRange, input)
	drift := medicationReportDrift(state.Sessions, reportRange, now)
	status := "ready"
	message := fmt.Sprintf("%d calendar rows generated from effective local records.", len(rows))
	if observedSegments == 0 {
		status = "insufficient"
		message = "No recorded sleep overlaps this range. Calendar rows remain visible as no-data rows; no sleep was invented."
	} else if noDataRows > 0 {
		status = "partial"
		message = fmt.Sprintf("%d of %d calendar rows have no recorded sleep. Gaps remain visible and no sleep was invented.", noDataRows, len(rows))
	}
	redactions := medicationReportRedactions(input)
	axisStart := reportRange.dayStart
	return MedicationClinicalReportDTO{
		Status:         status,
		Message:        message,
		GeneratedAt:    now.UTC().Format(time.RFC3339),
		GeneratedLabel: now.Local().Format("Jan 2, 2006, 3:04 PM"),
		Range: MedicationClinicalReportRangeDTO{
			Mode:          reportRange.mode,
			FromDate:      reportRange.fromDate.Format(time.DateOnly),
			ToDate:        reportRange.toDate.Format(time.DateOnly),
			Label:         reportRange.fromDate.Format("Jan 2, 2006") + " to " + reportRange.toDate.Format("Jan 2, 2006"),
			DayStartHour:  reportRange.dayStart,
			DayStartLabel: formatReportClock(reportRange.dayStart) + " to " + formatReportClock(reportRange.dayStart) + " next day",
		},
		Summary: MedicationClinicalReportSummaryDTO{
			CalendarRows:          len(rows),
			ObservedSleepSegments: observedSegments,
			NoDataRows:            noDataRows,
			MedicationEvents:      len(eventRows),
			RecordedScheduled:     adherenceCounts.scheduled,
			RecordedTaken:         adherenceCounts.taken,
			RecordedSkipped:       adherenceCounts.skipped,
			ExcludedEvents:        adherenceCounts.excluded,
			RhythmContextMarkers:  contextCount,
		},
		Redactions: redactions,
		Actogram: MedicationClinicalActogramDTO{
			AxisLabels: []string{formatReportClock(axisStart), formatReportClock((axisStart + 6) % 24), formatReportClock((axisStart + 12) % 24), formatReportClock((axisStart + 18) % 24), formatReportClock(axisStart)},
			Rows:       rows,
			Legend:     medicationReportLegend(legendKinds),
			Summary:    "Single-plot clinical actogram. Each row is one civil day; forecast is included only when explicitly selected.",
		},
		Drift:        drift,
		Adherence:    adherence,
		Events:       eventRows,
		Associations: associations,
		Provenance: []string{
			"Sleep bands use effective local sleep observations after append-only corrections; gaps are not filled.",
			"Medication rows use effective user-recorded taken or skipped events; excluded events are counted but omitted.",
			"Start comparisons use robust descriptive sleep-onset slopes on each side of a user-recorded medication start.",
			"Rhythm context is self-reported and does not alter the estimate.",
		},
		Notice: medicationReportNotice,
	}, nil
}

func resolveMedicationReportRange(input MedicationClinicalReportInput, now time.Time, sessions []domain.SleepSession, medications []storage.MedicationRecord, events []storage.EffectiveMedicationEvent, markers []storage.RhythmMarkerRecord) (medicationReportRange, error) {
	zoneID := strings.TrimSpace(input.ZoneID)
	if zoneID == "" {
		zoneID = defaultZoneID
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return medicationReportRange{}, fmt.Errorf("load report time zone %q: %w", zoneID, err)
	}
	if input.DayStartHour != 12 && input.DayStartHour != 18 {
		return medicationReportRange{}, errors.New("clinical report day start must be noon or 6 PM")
	}
	mode := strings.TrimSpace(input.RangeMode)
	if mode == "" {
		mode = "custom"
	}
	var fromDate, toDate time.Time
	switch mode {
	case "custom":
		fromDate, err = time.Parse(time.DateOnly, strings.TrimSpace(input.FromDate))
		if err != nil {
			return medicationReportRange{}, errors.New("report start must use YYYY-MM-DD")
		}
		toDate, err = time.Parse(time.DateOnly, strings.TrimSpace(input.ToDate))
		if err != nil {
			return medicationReportRange{}, errors.New("report end must use YYYY-MM-DD")
		}
	case "all":
		fromDate, toDate = medicationReportAllDates(now, location, input.DayStartHour, sessions, medications, events, markers)
	default:
		return medicationReportRange{}, errors.New("report range mode must be custom or all")
	}
	if toDate.Before(fromDate) {
		return medicationReportRange{}, errors.New("report end must not precede report start")
	}
	rowCount := int(toDate.Sub(fromDate).Hours()/24) + 1
	if rowCount < 1 || rowCount > medicationReportMaximumRows {
		return medicationReportRange{}, fmt.Errorf("report range must contain 1 to %d calendar rows", medicationReportMaximumRows)
	}
	startAt, err := medicationReportRowStart(fromDate, location, input.DayStartHour)
	if err != nil {
		return medicationReportRange{}, err
	}
	endAt, err := medicationReportRowStart(toDate.AddDate(0, 0, 1), location, input.DayStartHour)
	if err != nil {
		return medicationReportRange{}, err
	}
	return medicationReportRange{mode: mode, fromDate: fromDate, toDate: toDate, location: location, zoneID: zoneID, dayStart: input.DayStartHour, startAt: startAt, endAt: endAt}, nil
}

func medicationReportAllDates(now time.Time, location *time.Location, dayStart int, sessions []domain.SleepSession, medications []storage.MedicationRecord, events []storage.EffectiveMedicationEvent, markers []storage.RhythmMarkerRecord) (time.Time, time.Time) {
	instants := []time.Time{now}
	for _, session := range sessions {
		if session.Suppressed {
			continue
		}
		for _, interval := range session.Intervals {
			instants = append(instants, interval.Interval.Start.UTC, interval.Interval.End.UTC)
		}
	}
	for _, medication := range medications {
		if medication.StartedAt != nil {
			instants = append(instants, *medication.StartedAt)
		}
	}
	for _, event := range events {
		instants = append(instants, event.Event.DoseAt)
	}
	for _, marker := range markers {
		instants = append(instants, marker.StartAt)
		if marker.EndAt != nil {
			instants = append(instants, *marker.EndAt)
		}
	}
	sort.Slice(instants, func(i, j int) bool { return instants[i].Before(instants[j]) })
	return medicationReportRowDate(instants[0], location, dayStart), medicationReportRowDate(instants[len(instants)-1], location, dayStart)
}

func medicationReportRows(reportRange medicationReportRange) ([]MedicationClinicalActogramRowDTO, []time.Time, error) {
	rowCount := int(reportRange.toDate.Sub(reportRange.fromDate).Hours()/24) + 1
	rows := make([]MedicationClinicalActogramRowDTO, 0, rowCount)
	starts := make([]time.Time, 0, rowCount+1)
	var previousMonth time.Month
	for date := reportRange.fromDate; !date.After(reportRange.toDate); date = date.AddDate(0, 0, 1) {
		start, err := medicationReportRowStart(date, reportRange.location, reportRange.dayStart)
		if err != nil {
			return nil, nil, err
		}
		starts = append(starts, start)
		monthLabel := ""
		if len(rows) == 0 || date.Month() != previousMonth {
			monthLabel = date.Format("January 2006")
		}
		previousMonth = date.Month()
		weekday := date.Weekday()
		rows = append(rows, MedicationClinicalActogramRowDTO{
			CivilDate:   date.Format(time.DateOnly),
			DayLabel:    date.Format("02 Mon"),
			MonthLabel:  monthLabel,
			Weekend:     weekday == time.Saturday || weekday == time.Sunday,
			Sleep:       []MedicationClinicalSleepSegmentDTO{},
			Annotations: []MedicationClinicalAnnotationDTO{},
		})
	}
	end, err := medicationReportRowStart(reportRange.toDate.AddDate(0, 0, 1), reportRange.location, reportRange.dayStart)
	if err != nil {
		return nil, nil, err
	}
	starts = append(starts, end)
	return rows, starts, nil
}

func addMedicationReportSleep(rows []MedicationClinicalActogramRowDTO, rowStarts []time.Time, reportRange medicationReportRange, sessions []domain.SleepSession, legend map[string]bool) int {
	count := 0
	for _, session := range sessions {
		if session.Suppressed {
			continue
		}
		for _, interval := range session.Intervals {
			kind := "sleep_observed"
			if session.IsNapSleep() {
				kind = "sleep_nap"
			} else if !session.IsPrincipalSleep() || interval.StartEvidence.Status == domain.StatusInferred || interval.EndEvidence.Status == domain.StatusInferred {
				kind = "sleep_inferred"
			}
			first, last := medicationReportOverlappingRows(rowStarts, interval.Interval.Start.UTC, interval.Interval.End.UTC)
			for index := first; index < last; index++ {
				start := interval.Interval.Start.UTC
				if start.Before(rowStarts[index]) {
					start = rowStarts[index]
				}
				end := interval.Interval.End.UTC
				if end.After(rowStarts[index+1]) {
					end = rowStarts[index+1]
				}
				startHour := medicationReportRelativeCivilHour(reportRange.fromDate.AddDate(0, 0, index), start, reportRange.location, reportRange.dayStart)
				endHour := medicationReportRelativeCivilHour(reportRange.fromDate.AddDate(0, 0, index), end, reportRange.location, reportRange.dayStart)
				startHour = math.Max(0, math.Min(24, startHour))
				endHour = math.Max(startHour, math.Min(24, endHour))
				if endHour <= startHour {
					continue
				}
				localStart := start.In(reportRange.location)
				localEnd := end.In(reportRange.location)
				source := strings.TrimSpace(session.SourceLabel)
				if source == "" {
					source = string(interval.StartEvidence.Acquisition) + " sleep evidence"
				}
				rows[index].Sleep = append(rows[index].Sleep, MedicationClinicalSleepSegmentDTO{
					Kind:          kind,
					StartPercent:  roundReportPercent(startHour / 24 * 100),
					WidthPercent:  roundReportPercent((endHour - startHour) / 24 * 100),
					StartLabel:    localStart.Format("Jan 2, 3:04 PM MST"),
					WakeLabel:     localEnd.Format("Jan 2, 3:04 PM MST"),
					DurationLabel: compactMedicationDuration(end.Sub(start)),
					Source:        source,
					Confidence:    confidenceTitle(medicationEvidenceConfidence(interval.StartEvidence).Level),
				})
				legend[kind] = true
				count++
			}
		}
	}
	return count
}

func addMedicationReportForecast(rows []MedicationClinicalActogramRowDTO, rowStarts []time.Time, reportRange medicationReportRange, estimate domain.PhaseEstimate, legend map[string]bool) {
	for _, window := range estimate.PredictedSleepWindows {
		first, last := medicationReportOverlappingRows(rowStarts, window.Interval.Start.UTC, window.Interval.End.UTC)
		for index := first; index < last; index++ {
			start := window.Interval.Start.UTC
			if start.Before(rowStarts[index]) {
				start = rowStarts[index]
			}
			end := window.Interval.End.UTC
			if end.After(rowStarts[index+1]) {
				end = rowStarts[index+1]
			}
			rowDate := reportRange.fromDate.AddDate(0, 0, index)
			startHour := medicationReportRelativeCivilHour(rowDate, start, reportRange.location, reportRange.dayStart)
			endHour := medicationReportRelativeCivilHour(rowDate, end, reportRange.location, reportRange.dayStart)
			startHour = math.Max(0, math.Min(24, startHour))
			endHour = math.Max(startHour, math.Min(24, endHour))
			if endHour <= startHour {
				continue
			}
			rows[index].Sleep = append(rows[index].Sleep, MedicationClinicalSleepSegmentDTO{
				Kind:          "forecast",
				StartPercent:  roundReportPercent(startHour / 24 * 100),
				WidthPercent:  roundReportPercent((endHour - startHour) / 24 * 100),
				StartLabel:    start.In(reportRange.location).Format("Jan 2, 3:04 PM MST") + " earliest",
				WakeLabel:     end.In(reportRange.location).Format("Jan 2, 3:04 PM MST") + " latest",
				DurationLabel: compactMedicationDuration(end.Sub(start)) + " uncertain window",
				Source:        "Current local forecast",
				Confidence:    confidenceTitle(window.Confidence.Level),
			})
			legend["forecast"] = true
		}
	}
}

func addMedicationReportAnnotations(rows []MedicationClinicalActogramRowDTO, reportRange medicationReportRange, medications []storage.MedicationRecord, events []storage.EffectiveMedicationEvent, markers []storage.RhythmMarkerRecord, aliases map[string]string, input MedicationClinicalReportInput, legend map[string]bool) (int, int) {
	annotationCount := 0
	contextCount := 0
	appendAnnotation := func(at time.Time, annotation MedicationClinicalAnnotationDTO) {
		rowDate := medicationReportRowDate(at, reportRange.location, reportRange.dayStart)
		index := int(rowDate.Sub(reportRange.fromDate).Hours() / 24)
		if index < 0 || index >= len(rows) {
			return
		}
		hour := medicationReportRelativeCivilHour(rowDate, at, reportRange.location, reportRange.dayStart)
		annotation.PositionPercent = roundReportPercent(math.Max(0, math.Min(100, hour/24*100)))
		rows[index].Annotations = append(rows[index].Annotations, annotation)
		legend[annotation.Kind] = true
		annotationCount++
	}
	if input.IncludeMedication {
		for _, medication := range medications {
			if medication.StartedAt == nil {
				continue
			}
			appendAnnotation(*medication.StartedAt, MedicationClinicalAnnotationDTO{
				Kind:    "medication_start",
				Label:   aliases[medication.MedicationID] + " start",
				AtLabel: medication.StartedAt.In(reportRange.location).Format("Jan 2, 3:04 PM MST"),
				Detail:  "User-recorded start marker; temporal association only",
			})
		}
		for _, item := range events {
			if item.Excluded {
				continue
			}
			kind := "medication_taken"
			if item.Event.Status == storage.MedicationEventSkipped {
				kind = "medication_skipped"
			}
			appendAnnotation(item.Event.DoseAt, MedicationClinicalAnnotationDTO{
				Kind:    kind,
				Label:   aliases[item.Event.MedicationID],
				AtLabel: item.Event.DoseAt.In(reportRange.location).Format("Jan 2, 3:04 PM MST"),
				Detail:  "User-recorded " + item.Event.Status + " event",
			})
		}
	}
	if input.IncludeRhythmContext {
		for _, marker := range markers {
			before := annotationCount
			detail := "Self-reported context; does not alter the estimate"
			if input.IncludeRhythmContextNotes && marker.Note != "" {
				detail += ": " + marker.Note
			}
			appendAnnotation(marker.StartAt, MedicationClinicalAnnotationDTO{
				Kind:    "context_" + marker.Kind,
				Label:   rhythmMarkerKindLabel(marker.Kind),
				AtLabel: marker.StartAt.In(reportRange.location).Format("Jan 2, 3:04 PM MST"),
				Detail:  detail,
			})
			if annotationCount > before {
				contextCount++
			}
		}
	}
	return annotationCount, contextCount
}

type medicationReportAdherenceCounts struct {
	scheduled int
	taken     int
	skipped   int
	excluded  int
}

func medicationReportAdherence(events []storage.EffectiveMedicationEvent, medications []storage.MedicationRecord, aliases map[string]string, state localEstimateState, reportRange medicationReportRange, input MedicationClinicalReportInput) ([]MedicationClinicalAdherenceSummaryDTO, []MedicationClinicalAdherenceEventDTO, medicationReportAdherenceCounts) {
	if !input.IncludeMedication {
		return []MedicationClinicalAdherenceSummaryDTO{}, []MedicationClinicalAdherenceEventDTO{}, medicationReportAdherenceCounts{}
	}
	medicationByID := make(map[string]storage.MedicationRecord, len(medications))
	for _, medication := range medications {
		medicationByID[medication.MedicationID] = medication
	}
	type summary struct{ scheduled, taken, skipped, asNeeded int }
	summaries := map[string]*summary{}
	effective := append([]storage.EffectiveMedicationEvent(nil), events...)
	sort.SliceStable(effective, func(i, j int) bool { return effective[i].Event.DoseAt.Before(effective[j].Event.DoseAt) })
	anchors := medicationWakeAnchors(state.Sessions)
	sleepIndex := newMedicationSleepIndex(state.Sessions)
	latestWake := latestMedicationWake(anchors)
	rows := make([]MedicationClinicalAdherenceEventDTO, 0, len(effective))
	counts := medicationReportAdherenceCounts{}
	for _, item := range effective {
		if item.Event.DoseAt.Before(reportRange.startAt) || !item.Event.DoseAt.Before(reportRange.endAt) {
			continue
		}
		if item.Excluded {
			counts.excluded++
			continue
		}
		_, ok := medicationByID[item.Event.MedicationID]
		if !ok {
			continue
		}
		entry := summaries[item.Event.MedicationID]
		if entry == nil {
			entry = &summary{}
			summaries[item.Event.MedicationID] = entry
		}
		if item.Event.Scheduled {
			entry.scheduled++
			counts.scheduled++
			if item.Event.Status == storage.MedicationEventTaken {
				entry.taken++
				counts.taken++
			} else {
				entry.skipped++
				counts.skipped++
			}
		} else {
			entry.asNeeded++
		}
		projected := medicationEventDTO(item, aliases[item.Event.MedicationID], state, sleepIndex, anchors, latestWake)
		note := ""
		if input.IncludeMedicationNotes {
			note = item.Event.Note
		}
		scheduleContext := "As-needed / not marked scheduled"
		if item.Event.Scheduled {
			scheduleContext = "Recorded scheduled event"
		}
		rows = append(rows, MedicationClinicalAdherenceEventDTO{
			MedicationLabel: aliases[item.Event.MedicationID],
			CivilTime:       item.Event.DoseAt.In(reportRange.location).Format("Jan 2, 2006, 3:04 PM MST"),
			Status:          item.Event.Status,
			ScheduleContext: scheduleContext,
			WakeContext:     projected.WakeRelation,
			SleepContext:    projected.SleepRelation,
			Confidence:      projected.Confidence,
			Note:            note,
		})
	}
	result := make([]MedicationClinicalAdherenceSummaryDTO, 0, len(summaries))
	for _, medication := range medications {
		entry := summaries[medication.MedicationID]
		if entry == nil {
			continue
		}
		result = append(result, MedicationClinicalAdherenceSummaryDTO{
			MedicationLabel:   aliases[medication.MedicationID],
			RecordedScheduled: entry.scheduled,
			Taken:             entry.taken,
			Skipped:           entry.skipped,
			AsNeeded:          entry.asNeeded,
			Summary:           fmt.Sprintf("%d of %d explicitly recorded scheduled events were marked taken; %d were marked skipped. No unlogged dose is counted as missed.", entry.taken, entry.scheduled, entry.skipped),
		})
	}
	return result, rows, counts
}

func medicationReportAssociations(medications []storage.MedicationRecord, aliases map[string]string, markers []storage.RhythmMarkerRecord, sessions []domain.SleepSession, reportRange medicationReportRange, input MedicationClinicalReportInput) []MedicationClinicalAssociationDTO {
	if !input.IncludeMedication {
		return []MedicationClinicalAssociationDTO{}
	}
	selectedSessions := medicationReportSessionsInRange(sessions, reportRange)
	result := make([]MedicationClinicalAssociationDTO, 0)
	for _, medication := range medications {
		if medication.StartedAt == nil || medication.StartedAt.Before(reportRange.startAt) || !medication.StartedAt.Before(reportRange.endAt) {
			continue
		}
		association := estimation.DescribeTemporalAssociation(selectedSessions, *medication.StartedAt)
		windowStart := association.WindowStart
		if windowStart.IsZero() || windowStart.Before(reportRange.startAt) {
			windowStart = reportRange.startAt
		}
		windowEnd := association.WindowEnd
		if windowEnd.IsZero() || windowEnd.After(reportRange.endAt) {
			windowEnd = reportRange.endAt
		}
		contexts := make([]MedicationClinicalAssociationContextDTO, 0)
		if input.IncludeRhythmContext {
			for _, marker := range markers {
				markerEnd := marker.StartAt.Add(time.Nanosecond)
				if marker.EndAt != nil {
					markerEnd = *marker.EndAt
				}
				if !markerEnd.After(windowStart) || !marker.StartAt.Before(windowEnd) {
					continue
				}
				projected, err := rhythmMarkerDTO(marker)
				if err != nil {
					continue
				}
				note := ""
				if input.IncludeRhythmContextNotes {
					note = marker.Note
				}
				contexts = append(contexts, MedicationClinicalAssociationContextDTO{
					KindLabel:   projected.KindLabel,
					RangeLabel:  projected.RangeLabel,
					TimingLabel: medicationReportMarkerTiming(marker, *medication.StartedAt),
					Note:        note,
				})
			}
		}
		message := association.Message
		if input.IncludeRhythmContext {
			message += " Included self-reported context in the comparison window is listed separately as possible confounding, not explanation."
		} else {
			message += " Self-reported context was redacted from this report."
		}
		result = append(result, MedicationClinicalAssociationDTO{
			MedicationLabel: aliases[medication.MedicationID],
			StartedLabel:    medication.StartedAt.In(reportRange.location).Format("Jan 2, 2006, 3:04 PM MST"),
			Status:          association.Status,
			Message:         message,
			Before:          medicationReportAssociationSegment(association.Before, reportRange.location),
			After:           medicationReportAssociationSegment(association.After, reportRange.location),
			Context:         contexts,
		})
	}
	return result
}

func medicationReportAssociationSegment(segment estimation.DriftSegment, location *time.Location) MedicationClinicalAssociationSegmentDTO {
	rangeLabel := "Not enough episodes"
	slopeLabel := "Unavailable"
	confidence := "Unknown"
	if !segment.FromAt.IsZero() && !segment.ToAt.IsZero() {
		rangeLabel = segment.FromAt.In(location).Format("Jan 2, 2006") + " to " + segment.ToAt.In(location).Format("Jan 2, 2006")
	}
	if segment.EpisodeCount >= 5 && !segment.FromAt.IsZero() {
		slopeLabel = fmt.Sprintf("%+.0f min per observed cycle", segment.Drift.Minutes())
		confidence = confidenceTitle(segment.Confidence.Level)
	}
	return MedicationClinicalAssociationSegmentDTO{EpisodeCount: segment.EpisodeCount, RangeLabel: rangeLabel, SlopeLabel: slopeLabel, Confidence: confidence}
}

func medicationReportMarkerTiming(marker storage.RhythmMarkerRecord, startedAt time.Time) string {
	if !marker.StartAt.After(startedAt) && (marker.EndAt == nil || !marker.EndAt.Before(startedAt)) {
		return "Overlapped the recorded medication start"
	}
	delta := marker.StartAt.Sub(startedAt)
	direction := "after"
	if delta < 0 {
		direction = "before"
		delta = -delta
	}
	days := int(math.Round(delta.Hours() / 24))
	if days == 0 {
		return "Began within 24 hours " + direction + " the recorded medication start"
	}
	return fmt.Sprintf("Began %d %s %s the recorded medication start", days, plural(days, "day", "days"), direction)
}

func medicationReportDrift(sessions []domain.SleepSession, reportRange medicationReportRange, now time.Time) MedicationClinicalDriftDTO {
	selected := medicationReportSessionsInRange(sessions, reportRange)
	asOf := now
	if reportRange.endAt.Before(asOf) {
		asOf = reportRange.endAt
	}
	projection, err := (estimation.RobustEstimator{}).Project(context.Background(), selected, asOf)
	if err != nil {
		status := "unavailable"
		message := "Sleep-onset drift is unavailable for the selected range."
		var refusal *estimation.EstimationRefusal
		if errors.As(err, &refusal) {
			status = string(refusal.Code)
			message = refusal.Message
		}
		return MedicationClinicalDriftDTO{Status: status, SlopeLabel: "Unavailable", Confidence: "Unknown", Summary: message, Points: []MedicationClinicalDriftPointDTO{}}
	}
	points := make([]MedicationClinicalDriftPointDTO, 0, len(projection.DriftPoints))
	for _, point := range projection.DriftPoints {
		points = append(points, MedicationClinicalDriftPointDTO{
			ID: point.ID, Day: point.Day, CivilDate: point.CivilDate,
			OnsetHour: point.OnsetHour, FitHour: point.FitHour,
			BandLowHour: point.BandLowHour, BandHighHour: point.BandHighHour,
			OnsetLabel: point.OnsetLabel, Source: point.Source, Confidence: point.Confidence,
		})
	}
	return MedicationClinicalDriftDTO{Status: "estimated", SlopeLabel: projection.SlopeLabel, Confidence: projection.DriftConfidence, Summary: projection.DriftSummary, YMinHour: projection.YMinHour, YMaxHour: projection.YMaxHour, Points: points}
}

func medicationReportSessionsInRange(sessions []domain.SleepSession, reportRange medicationReportRange) []domain.SleepSession {
	selected := make([]domain.SleepSession, 0, len(sessions))
	for _, session := range sessions {
		if len(session.Intervals) == 0 {
			continue
		}
		onset := session.Intervals[0].Interval.Start.UTC
		if onset.Before(reportRange.startAt) || !onset.Before(reportRange.endAt) {
			continue
		}
		selected = append(selected, session)
	}
	return selected
}

func medicationReportAliases(medications []storage.MedicationRecord, includeLabels bool) map[string]string {
	aliases := make(map[string]string, len(medications))
	for index, medication := range medications {
		label := fmt.Sprintf("Medication %d", index+1)
		if includeLabels {
			label = medication.Label
			details := make([]string, 0, 2)
			if medication.Form != "" {
				details = append(details, medication.Form)
			}
			if medication.StrengthLabel != "" {
				details = append(details, medication.StrengthLabel)
			}
			if len(details) > 0 {
				label += " (" + strings.Join(details, ", ") + ")"
			}
		}
		aliases[medication.MedicationID] = label
	}
	return aliases
}

func medicationReportRedactions(input MedicationClinicalReportInput) []string {
	redactions := []string{
		"Personal diagnostic information omitted",
		"Calendar and location information omitted",
		"Clinician-entered medication guidance omitted",
	}
	if !input.IncludeMedication {
		redactions = append(redactions, "Medication events, labels, notes, and markers omitted")
	} else {
		if !input.IncludeMedicationLabels {
			redactions = append(redactions, "Medication labels, forms, and strength labels replaced with neutral aliases")
		}
		if !input.IncludeMedicationNotes {
			redactions = append(redactions, "Medication notes omitted")
		}
	}
	if !input.IncludeRhythmContext {
		redactions = append(redactions, "Self-reported rhythm context omitted")
	} else if !input.IncludeRhythmContextNotes {
		redactions = append(redactions, "Private rhythm-context notes omitted")
	}
	if !input.IncludeForecast {
		redactions = append(redactions, "Forecast bands omitted")
	}
	return redactions
}

// medicationReportOverlappingRows narrows an interval to the only rows it can
// touch. rowStarts contains one extra boundary and remains ordered across DST.
func medicationReportOverlappingRows(rowStarts []time.Time, start, end time.Time) (int, int) {
	rowCount := len(rowStarts) - 1
	if rowCount <= 0 {
		return 0, 0
	}
	start = start.UTC()
	end = end.UTC()
	first := sort.Search(rowCount, func(index int) bool {
		return rowStarts[index+1].UTC().After(start)
	})
	last := sort.Search(rowCount, func(index int) bool {
		return !rowStarts[index].UTC().Before(end)
	})
	if first >= last {
		return 0, 0
	}
	return first, last
}

func medicationReportLegend(kinds map[string]bool) []MedicationClinicalLegendDTO {
	order := []string{"sleep_observed", "sleep_inferred", "sleep_nap", "forecast", "medication_taken", "medication_skipped", "medication_start", "context_travel", "context_illness", "context_disruption", "context_forced_schedule"}
	labels := map[string]string{
		"sleep_observed":          "Recorded sleep",
		"sleep_inferred":          "Inferred sleep",
		"sleep_nap":               "Recorded nap",
		"forecast":                "Current uncertain forecast",
		"medication_taken":        "Recorded taken event",
		"medication_skipped":      "Recorded skipped event",
		"medication_start":        "User-recorded medication start",
		"context_travel":          "Self-reported travel context",
		"context_illness":         "Self-reported illness context",
		"context_disruption":      "Self-reported sleep disruption",
		"context_forced_schedule": "Self-reported forced schedule",
	}
	result := make([]MedicationClinicalLegendDTO, 0, len(kinds))
	for _, kind := range order {
		if kinds[kind] {
			result = append(result, MedicationClinicalLegendDTO{Kind: kind, Label: labels[kind]})
		}
	}
	return result
}

func medicationReportRowStart(date time.Time, location *time.Location, dayStart int) (time.Time, error) {
	resolution, err := domain.ResolveCivilTime(location, date.Year(), date.Month(), date.Day(), dayStart, 0, 0)
	if err != nil {
		return time.Time{}, fmt.Errorf("resolve report row start: %w", err)
	}
	return resolution.Time.UTC(), nil
}

func medicationReportRowDate(at time.Time, location *time.Location, dayStart int) time.Time {
	local := at.In(location)
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	if local.Hour() < dayStart {
		date = date.AddDate(0, 0, -1)
	}
	return date
}

func medicationReportRelativeCivilHour(rowDate time.Time, at time.Time, location *time.Location, dayStart int) float64 {
	local := at.In(location)
	localDate := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	days := localDate.Sub(rowDate).Hours() / 24
	return days*24 + float64(local.Hour()) + float64(local.Minute())/60 + float64(local.Second())/3600 - float64(dayStart)
}

func formatReportClock(hour int) string {
	value := time.Date(2000, 1, 1, hour, 0, 0, 0, time.UTC)
	return value.Format("3 PM")
}

func roundReportPercent(value float64) float64 {
	return math.Round(value*100) / 100
}

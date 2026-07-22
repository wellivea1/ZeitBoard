package main

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

type medicationReportHTMLMonth struct {
	Label string
	Rows  []MedicationClinicalActogramRowDTO
}

type medicationReportHTMLDriftPoint struct {
	Day        string
	OnsetLabel string
	Source     string
	Confidence string
	X          float64
	ObservedY  float64
	FitY       float64
	BandLowY   float64
	BandHighY  float64
}

type medicationReportHTMLDriftSegment struct {
	X1 float64
	Y1 float64
	X2 float64
	Y2 float64
}

type medicationReportHTMLView struct {
	Report           MedicationClinicalReportDTO
	Months           []medicationReportHTMLMonth
	DriftPoints      []medicationReportHTMLDriftPoint
	ObservedSegments []medicationReportHTMLDriftSegment
	FitSegments      []medicationReportHTMLDriftSegment
	DriftBand        string
}

func renderMedicationClinicalReportHTML(report MedicationClinicalReportDTO) (string, error) {
	view := medicationReportHTMLView{Report: report, Months: medicationReportHTMLMonths(report.Actogram.Rows)}
	view.DriftPoints, view.ObservedSegments, view.FitSegments, view.DriftBand = medicationReportHTMLDrift(report.Drift)
	tmpl, err := template.New("clinician-report").Funcs(template.FuncMap{
		"chartX": func(percent float64) float64 { return math.Round(percent*2.4*100) / 100 },
		"chartW": func(percent float64) float64 { return math.Max(0.7, math.Round(percent*2.4*100)/100) },
		"title": func(value string) string {
			if value == "" {
				return ""
			}
			return strings.ToUpper(value[:1]) + value[1:]
		},
	}).Parse(medicationClinicalReportTemplate)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	if err := tmpl.Execute(&output, view); err != nil {
		return "", err
	}
	return strings.TrimSpace(output.String()), nil
}

func medicationReportHTMLMonths(rows []MedicationClinicalActogramRowDTO) []medicationReportHTMLMonth {
	months := make([]medicationReportHTMLMonth, 0)
	for _, row := range rows {
		if row.MonthLabel != "" || len(months) == 0 {
			label := row.MonthLabel
			if label == "" {
				label = row.CivilDate[:7]
			}
			months = append(months, medicationReportHTMLMonth{Label: label, Rows: []MedicationClinicalActogramRowDTO{}})
		}
		months[len(months)-1].Rows = append(months[len(months)-1].Rows, row)
	}
	return months
}

func medicationReportHTMLDrift(drift MedicationClinicalDriftDTO) ([]medicationReportHTMLDriftPoint, []medicationReportHTMLDriftSegment, []medicationReportHTMLDriftSegment, string) {
	if len(drift.Points) == 0 || drift.YMaxHour <= drift.YMinHour {
		return []medicationReportHTMLDriftPoint{}, []medicationReportHTMLDriftSegment{}, []medicationReportHTMLDriftSegment{}, ""
	}
	points := make([]medicationReportHTMLDriftPoint, 0, len(drift.Points))
	mapY := func(value float64) float64 {
		return 160 - ((value-drift.YMinHour)/(drift.YMaxHour-drift.YMinHour))*140
	}
	for index, point := range drift.Points {
		x := 20.0
		if len(drift.Points) > 1 {
			x += float64(index) / float64(len(drift.Points)-1) * 680
		}
		points = append(points, medicationReportHTMLDriftPoint{
			Day: point.Day, OnsetLabel: point.OnsetLabel, Source: point.Source, Confidence: point.Confidence,
			X: math.Round(x*100) / 100, ObservedY: math.Round(mapY(point.OnsetHour)*100) / 100, FitY: math.Round(mapY(point.FitHour)*100) / 100,
			BandLowY: math.Round(mapY(point.BandLowHour)*100) / 100, BandHighY: math.Round(mapY(point.BandHighHour)*100) / 100,
		})
	}
	bandParts := make([]string, 0, len(points)*2)
	for _, point := range points {
		bandParts = append(bandParts, fmt.Sprintf("%.2f,%.2f", point.X, point.BandHighY))
	}
	for index := len(points) - 1; index >= 0; index-- {
		bandParts = append(bandParts, fmt.Sprintf("%.2f,%.2f", points[index].X, points[index].BandLowY))
	}
	observed := make([]medicationReportHTMLDriftSegment, 0, len(points)-1)
	fit := make([]medicationReportHTMLDriftSegment, 0, len(points)-1)
	for index := 1; index < len(points); index++ {
		observed = append(observed, medicationReportHTMLDriftSegment{X1: points[index-1].X, Y1: points[index-1].ObservedY, X2: points[index].X, Y2: points[index].ObservedY})
		fit = append(fit, medicationReportHTMLDriftSegment{X1: points[index-1].X, Y1: points[index-1].FitY, X2: points[index].X, Y2: points[index].FitY})
	}
	return points, observed, fit, strings.Join(bandParts, " ")
}

const medicationClinicalReportTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:">
  <title>Sleep and medication context report</title>
  <style>
    :root { color-scheme: light; font-family: Inter, "Segoe UI", Arial, sans-serif; color: #17211c; background: #fff; }
    * { box-sizing: border-box; }
    body { margin: 0; font-size: 10pt; line-height: 1.35; }
    main { max-width: 11in; margin: 0 auto; padding: .35in; }
    h1, h2, h3, p { margin-top: 0; }
    h1 { margin-bottom: 4pt; font-size: 20pt; letter-spacing: -.02em; }
    h2 { margin-bottom: 8pt; padding-bottom: 4pt; border-bottom: 1.5pt solid #233f34; font-size: 13pt; }
    h3 { margin: 10pt 0 4pt; font-size: 10.5pt; }
    .report-meta { display: grid; grid-template-columns: repeat(4, 1fr); margin: 12pt 0; border-top: 1pt solid #819188; border-bottom: 1pt solid #819188; }
    .report-meta div { padding: 6pt 8pt; border-right: 1pt solid #d4dbd7; }
    .report-meta div:last-child { border-right: 0; }
    .report-meta dt { color: #58665f; font-size: 7.5pt; font-weight: 700; letter-spacing: .06em; text-transform: uppercase; }
    .report-meta dd { margin: 2pt 0 0; font-weight: 650; }
    .notice { margin: 10pt 0; padding: 7pt 9pt; border-left: 3pt solid #365d4c; background: #f2f5f3; }
    .redactions { margin: 0; padding-left: 16pt; columns: 2; }
    .month { break-before: page; page-break-before: always; break-after: page; page-break-after: always; }
    .month:last-child { break-after: auto; page-break-after: auto; }
    .month-heading { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 4pt; }
    .month-heading h2 { flex: 1; margin: 0; }
    .axis { display: grid; grid-template-columns: 48pt repeat(5, 1fr); margin: 3pt 0; color: #5b6862; font-size: 7pt; }
    .axis span:not(:first-child) { text-align: center; }
    .actogram-row { display: grid; grid-template-columns: 48pt 1fr; min-height: 13pt; border-top: .5pt solid #d8ddda; break-inside: avoid; }
    .actogram-row.weekend { background: #f4f6f5; }
    .actogram-row .date { padding: 1.5pt 4pt; font-size: 7.2pt; font-variant-numeric: tabular-nums; }
    .actogram-row .track { border-left: .5pt solid #9ba7a1; background-image: linear-gradient(to right, transparent calc(25% - .25pt), #d9dfdc calc(25% - .25pt), #d9dfdc 25%, transparent 25%, transparent calc(50% - .25pt), #d9dfdc calc(50% - .25pt), #d9dfdc 50%, transparent 50%, transparent calc(75% - .25pt), #d9dfdc calc(75% - .25pt), #d9dfdc 75%, transparent 75%); }
    svg { display: block; width: 100%; height: 12pt; overflow: visible; }
    rect.sleep_observed { fill: #315e91; }
    rect.sleep_inferred { fill: #9eb4c8; stroke: #315e91; stroke-width: .6; stroke-dasharray: 2 1; }
    rect.sleep_nap { fill: #6f8fae; }
    rect.forecast { fill: #d7e1ea; stroke: #758b9f; stroke-width: .6; stroke-dasharray: 2 1; }
    line.annotation { stroke-width: 1.5; }
    line.medication_taken { stroke: #255b46; }
    line.medication_skipped { stroke: #745b28; stroke-dasharray: 1.5 1; }
    line.medication_start { stroke: #4e3e78; stroke-width: 2; }
    line.context_travel, line.context_illness, line.context_disruption, line.context_forced_schedule { stroke: #7a4d3a; }
    .no-data { fill: #6a756f; font-size: 7px; }
    .legend { display: flex; flex-wrap: wrap; gap: 5pt 12pt; margin: 7pt 0 0; padding-top: 5pt; border-top: .5pt solid #9ba7a1; font-size: 7.5pt; }
    .legend span::before { content: ""; display: inline-block; width: 10pt; height: 4pt; margin-right: 4pt; background: #315e91; vertical-align: middle; }
    .legend .medication_taken::before, .legend .medication_skipped::before, .legend .medication_start::before, .legend [class^="context_"]::before { width: 1.5pt; height: 8pt; background: #4e3e78; }
    .legend .sleep_inferred::before, .legend .forecast::before { background: #d7e1ea; border: .5pt dashed #315e91; }
    table { width: 100%; border-collapse: collapse; margin: 5pt 0 12pt; font-size: 8pt; }
    th { padding: 4pt; border-top: 1pt solid #617069; border-bottom: 1pt solid #617069; text-align: left; font-size: 7pt; letter-spacing: .04em; text-transform: uppercase; }
    td { padding: 4pt; border-bottom: .5pt solid #d5dcd8; vertical-align: top; }
    tbody tr:nth-child(even) { background: #f7f8f7; }
    .summary-grid { display: grid; grid-template-columns: repeat(4, 1fr); border-top: 1pt solid #617069; border-bottom: 1pt solid #617069; margin: 8pt 0 14pt; }
    .summary-grid div { padding: 6pt; border-right: .5pt solid #d5dcd8; }
    .summary-grid div:last-child { border-right: 0; }
    .summary-grid strong { display: block; font-size: 13pt; }
    .summary-grid span { color: #5b6862; font-size: 7.5pt; }
    .drift-chart { width: 100%; height: 180px; border: .5pt solid #cbd3cf; background: #fbfcfb; }
    .drift-band { fill: #315e91; opacity: .14; }
    .drift-observed { stroke: #315e91; stroke-width: 1.5; }
    .drift-fit { stroke: #365d4c; stroke-width: 1.25; stroke-dasharray: 4 2; }
    .drift-point { fill: #315e91; }
    .association { break-inside: avoid; margin: 10pt 0 14pt; padding-top: 6pt; border-top: 1pt solid #617069; }
    .context-list { margin: 4pt 0 0; padding-left: 16pt; }
    .footer { margin-top: 14pt; padding-top: 8pt; border-top: 1pt solid #617069; color: #44524b; font-size: 7.5pt; }
    .sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0,0,0,0); white-space: nowrap; border: 0; }
    @media print {
      @page { size: letter landscape; margin: .35in; }
      main { max-width: none; padding: 0; }
      .print-help { display: none; }
    }
  </style>
</head>
<body>
<main>
  <header>
    <h1>Sleep and medication context report</h1>
    <p>Owner-generated local record for clinical review. This artifact contains observations and descriptive projections, not instructions.</p>
    <dl class="report-meta">
      <div><dt>Row-date range</dt><dd>{{.Report.Range.Label}}</dd></div>
      <div><dt>Clinical row</dt><dd>{{.Report.Range.DayStartLabel}}</dd></div>
      <div><dt>Generated</dt><dd>{{.Report.GeneratedLabel}}</dd></div>
      <div><dt>Completeness</dt><dd>{{title .Report.Status}}</dd></div>
    </dl>
    <p class="print-help">Open this file in a browser and use Print to create a PDF. No network connection is used.</p>
    <p class="notice"><strong>Interpretation boundary.</strong> {{.Report.Notice}}</p>
    <h2>Applied redactions</h2>
    <ul class="redactions">{{range .Report.Redactions}}<li>{{.}}</li>{{end}}</ul>
  </header>

  <section>
    <h2>Range summary</h2>
    <div class="summary-grid">
      <div><strong>{{.Report.Summary.CalendarRows}}</strong><span>calendar rows</span></div>
      <div><strong>{{.Report.Summary.ObservedSleepSegments}}</strong><span>recorded sleep segments</span></div>
      <div><strong>{{.Report.Summary.NoDataRows}}</strong><span>rows with no sleep data</span></div>
      <div><strong>{{.Report.Summary.MedicationEvents}}</strong><span>included medication events</span></div>
    </div>
    <p>{{.Report.Message}}</p>
  </section>

  {{range .Months}}
  <section class="month">
    <div class="month-heading"><h2>{{.Label}}</h2></div>
    <div class="axis"><span>Row date</span>{{range $.Report.Actogram.AxisLabels}}<span>{{.}}</span>{{end}}</div>
    {{range .Rows}}
    <div class="actogram-row{{if .Weekend}} weekend{{end}}">
      <span class="date">{{.DayLabel}}</span>
      <div class="track">
        <svg viewBox="0 0 240 12" role="img" aria-label="{{.CivilDate}}: {{if .NoData}}no recorded sleep{{else}}{{len .Sleep}} sleep segment(s){{end}}, {{len .Annotations}} annotation(s)">
          {{if .NoData}}<text class="no-data" x="4" y="9">no data</text>{{end}}
          {{range .Sleep}}<rect class="{{.Kind}}" x="{{chartX .StartPercent}}" y="3" width="{{chartW .WidthPercent}}" height="6"><title>{{.StartLabel}} to {{.WakeLabel}}; {{.Source}}; {{.Confidence}} confidence</title></rect>{{end}}
          {{range .Annotations}}<line class="annotation {{.Kind}}" x1="{{chartX .PositionPercent}}" x2="{{chartX .PositionPercent}}" y1="1" y2="11"><title>{{.Label}} at {{.AtLabel}}{{with .Detail}}; {{.}}{{end}}</title></line>{{end}}
        </svg>
      </div>
    </div>
    {{end}}
    <div class="legend" aria-label="Chart legend">{{range $.Report.Actogram.Legend}}<span class="{{.Kind}}">{{.Label}}</span>{{end}}</div>
  </section>
  {{end}}

  <table class="sr-only">
    <caption>Clinical chart text alternative for every calendar row</caption>
    <thead><tr><th>Date</th><th>Sleep and forecast segments</th><th>Recorded timing annotations</th></tr></thead>
    <tbody>
      {{range .Report.Actogram.Rows}}
      <tr>
        <td>{{.CivilDate}}</td>
        <td>{{if .Sleep}}{{range $index, $segment := .Sleep}}{{if $index}}; {{end}}{{$segment.StartLabel}} to {{$segment.WakeLabel}}, {{$segment.Source}}, {{$segment.Confidence}} confidence{{end}}{{else}}No recorded sleep{{end}}</td>
        <td>{{if .Annotations}}{{range $index, $annotation := .Annotations}}{{if $index}}; {{end}}{{$annotation.Label}} at {{$annotation.AtLabel}}{{with $annotation.Detail}}, {{.}}{{end}}{{end}}{{else}}None{{end}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>

  <section>
    <h2>Sleep-onset drift</h2>
    <p><strong>{{.Report.Drift.SlopeLabel}}</strong> | {{.Report.Drift.Confidence}} confidence. {{.Report.Drift.Summary}}</p>
    {{if .DriftPoints}}
    <svg class="drift-chart" viewBox="0 0 720 180" role="img" aria-label="Observed sleep-onset drift and robust fitted trajectory">
      <polygon class="drift-band" points="{{.DriftBand}}"><title>Ordinal uncertainty band from the current robust fit</title></polygon>
      {{range .ObservedSegments}}<line class="drift-observed" x1="{{.X1}}" y1="{{.Y1}}" x2="{{.X2}}" y2="{{.Y2}}" />{{end}}
      {{range .FitSegments}}<line class="drift-fit" x1="{{.X1}}" y1="{{.Y1}}" x2="{{.X2}}" y2="{{.Y2}}" />{{end}}
      {{range .DriftPoints}}<circle class="drift-point" cx="{{.X}}" cy="{{.ObservedY}}" r="2.4"><title>{{.Day}} {{.OnsetLabel}}; {{.Source}}; {{.Confidence}} confidence</title></circle>{{end}}
    </svg>
    <table>
      <thead><tr><th>Date</th><th>Observed onset</th><th>Source</th><th>Confidence</th></tr></thead>
      <tbody>{{range .DriftPoints}}<tr><td>{{.Day}}</td><td>{{.OnsetLabel}}</td><td>{{.Source}}</td><td>{{.Confidence}}</td></tr>{{end}}</tbody>
    </table>
    {{else}}<p>No drift series is available for the current effective sleep data.</p>{{end}}
  </section>

  {{if .Report.Adherence}}
  <section>
    <h2>Recorded adherence in rhythm context</h2>
    <p>Only events explicitly marked scheduled contribute to taken/skipped counts. Absence of a log is never converted into a missed dose.</p>
    <table>
      <thead><tr><th>Medication</th><th>Recorded scheduled</th><th>Taken</th><th>Skipped</th><th>As-needed records</th><th>Interpretation</th></tr></thead>
      <tbody>{{range .Report.Adherence}}<tr><td>{{.MedicationLabel}}</td><td>{{.RecordedScheduled}}</td><td>{{.Taken}}</td><td>{{.Skipped}}</td><td>{{.AsNeeded}}</td><td>{{.Summary}}</td></tr>{{end}}</tbody>
    </table>
    <h3>Event detail</h3>
    <table>
      <thead><tr><th>Civil time</th><th>Medication</th><th>Status</th><th>Schedule record</th><th>Wake context</th><th>Sleep context</th><th>Confidence</th><th>Note</th></tr></thead>
      <tbody>{{range .Report.Events}}<tr><td>{{.CivilTime}}</td><td>{{.MedicationLabel}}</td><td>{{title .Status}}</td><td>{{.ScheduleContext}}</td><td>{{.WakeContext}}</td><td>{{.SleepContext}}</td><td>{{.Confidence}}</td><td>{{if .Note}}{{.Note}}{{else}}-{{end}}</td></tr>{{end}}</tbody>
    </table>
  </section>
  {{end}}

  {{if .Report.Associations}}
  <section>
    <h2>Temporal association around recorded starts</h2>
    <p>These descriptive before/after segments do not isolate a medication effect. Simultaneous self-reported context is listed so it cannot be silently ignored.</p>
    {{range .Report.Associations}}
    <article class="association">
      <h3>{{.MedicationLabel}} | start recorded {{.StartedLabel}}</h3>
      <p>{{.Message}}</p>
      <table>
        <thead><tr><th>Segment</th><th>Usable episodes</th><th>Range</th><th>Descriptive slope</th><th>Confidence</th></tr></thead>
        <tbody><tr><td>Before</td><td>{{.Before.EpisodeCount}}</td><td>{{.Before.RangeLabel}}</td><td>{{.Before.SlopeLabel}}</td><td>{{.Before.Confidence}}</td></tr><tr><td>After</td><td>{{.After.EpisodeCount}}</td><td>{{.After.RangeLabel}}</td><td>{{.After.SlopeLabel}}</td><td>{{.After.Confidence}}</td></tr></tbody>
      </table>
      <h3>Simultaneous context and possible confounding</h3>
      {{if .Context}}<ul class="context-list">{{range .Context}}<li><strong>{{.KindLabel}}</strong> | {{.RangeLabel}} | {{.TimingLabel}}{{with .Note}} | private note: {{.}}{{end}}</li>{{end}}</ul>{{else}}<p>No included self-reported context overlaps the descriptive comparison window.</p>{{end}}
    </article>
    {{end}}
  </section>
  {{end}}

  <footer class="footer">
    <h2>Provenance and limitations</h2>
    <ul>{{range .Report.Provenance}}<li>{{.}}</li>{{end}}</ul>
    <p>{{.Report.Notice}}</p>
  </footer>
</main>
</body>
</html>
`

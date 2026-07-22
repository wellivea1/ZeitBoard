import { Fragment, useMemo, useState } from "react";

import type {
  MedicationClinicalActogramRow,
  MedicationClinicalAnnotationKind,
  MedicationClinicalDriftPoint,
  MedicationClinicalReport,
  MedicationClinicalSleepKind,
} from "../data/medicationReport";

const rowsPerPage = 31;

function annotationSymbol(kind: MedicationClinicalAnnotationKind): string {
  switch (kind) {
    case "medication_taken":
      return "T";
    case "medication_skipped":
      return "S";
    case "medication_start":
      return "M";
    default:
      return "C";
  }
}

function sleepKindLabel(kind: MedicationClinicalSleepKind): string {
  switch (kind) {
    case "sleep_observed":
      return "Observed sleep";
    case "sleep_inferred":
      return "Inferred sleep";
    case "sleep_nap":
      return "Nap";
    case "forecast":
      return "Forecast sleep";
  }
}

function monthLabel(civilDate: string): string {
  return new Intl.DateTimeFormat(undefined, {
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(`${civilDate}T12:00:00Z`));
}

export function MedicationReportActogram({ report }: { report: MedicationClinicalReport }) {
  const [page, setPage] = useState(0);
  const pageCount = Math.max(1, Math.ceil(report.actogram.rows.length / rowsPerPage));
  const safePage = Math.min(page, pageCount - 1);
  const start = safePage * rowsPerPage;
  const rows = report.actogram.rows.slice(start, start + rowsPerPage);

  return (
    <section className="medication-report-figure" aria-labelledby="clinical-actogram-title">
      <header className="medication-report-subheading">
        <div>
          <p className="section-kicker">Clinical-day view</p>
          <h3 id="clinical-actogram-title">Sleep and recorded timing</h3>
        </div>
        <span>{report.range.dayStartLabel} anchor</span>
      </header>

      <p className="medication-report-figure-summary">{report.actogram.summary}</p>
      <div className="clinical-actogram" role="img" aria-label={report.actogram.summary}>
        <div className="clinical-actogram-axis" aria-hidden="true">
          <span />
          {report.actogram.axisLabels.map((label, index) => (
            <span key={`${label}-${index}`}>{label}</span>
          ))}
        </div>
        {rows.map((row, index) => (
          <Fragment key={row.civilDate}>
            {(row.monthLabel || index === 0) && (
              <div className="clinical-actogram-month">
                <span>{row.monthLabel || monthLabel(row.civilDate)}</span>
              </div>
            )}
            <ClinicalActogramRow row={row} />
          </Fragment>
        ))}
      </div>

      <div className="medication-report-legend" aria-label="Items present in this chart">
        {report.actogram.legend.map((item) => (
          <span key={item.kind}>
            <i data-kind={item.kind} aria-hidden="true" />
            {item.label}
          </span>
        ))}
      </div>

      {pageCount > 1 && (
        <nav className="medication-report-pagination" aria-label="Clinical chart pages">
          <button
            className="button ghost compact"
            type="button"
            disabled={safePage === 0}
            onClick={() => setPage((current) => Math.max(0, current - 1))}
          >
            Previous 31 days
          </button>
          <span>
            Rows {start + 1}-{start + rows.length} of {report.actogram.rows.length}
          </span>
          <button
            className="button ghost compact"
            type="button"
            disabled={safePage === pageCount - 1}
            onClick={() => setPage((current) => Math.min(pageCount - 1, current + 1))}
          >
            Next 31 days
          </button>
        </nav>
      )}

      <table className="sr-table">
        <caption>
          Sleep and timing details for rows {start + 1} through {start + rows.length}
        </caption>
        <thead>
          <tr>
            <th>Date</th>
            <th>Sleep</th>
            <th>Recorded timing</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.civilDate}>
              <td>{row.dayLabel}</td>
              <td>
                {row.noData
                  ? "No recorded sleep"
                  : row.sleep
                      .map(
                        (segment) =>
                          `${sleepKindLabel(segment.kind)}, ${segment.startLabel} to ${segment.wakeLabel}, ${segment.durationLabel}, ${segment.confidence} confidence`,
                      )
                      .join("; ")}
              </td>
              <td>
                {row.annotations.length === 0
                  ? "None"
                  : row.annotations
                      .map((annotation) =>
                        [annotation.label, annotation.atLabel, annotation.detail]
                          .filter(Boolean)
                          .join(", "),
                      )
                      .join("; ")}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function ClinicalActogramRow({ row }: { row: MedicationClinicalActogramRow }) {
  return (
    <div
      className="clinical-actogram-row"
      data-weekend={row.weekend || undefined}
      data-month={Boolean(row.monthLabel) || undefined}
    >
      <span className="clinical-actogram-date">{row.dayLabel}</span>
      <div className="clinical-actogram-track">
        {row.noData && <span className="clinical-actogram-gap">No data</span>}
        {row.sleep.map((segment, index) => (
          <span
            className="clinical-sleep-segment"
            data-kind={segment.kind}
            style={{ left: `${segment.startPercent}%`, width: `${segment.widthPercent}%` }}
            title={`${sleepKindLabel(segment.kind)}: ${segment.startLabel} to ${segment.wakeLabel}`}
            key={`${segment.kind}-${segment.startLabel}-${index}`}
          />
        ))}
        {row.annotations.map((annotation, index) => (
          <span
            className="clinical-annotation"
            data-kind={annotation.kind}
            style={{ left: `${annotation.positionPercent}%` }}
            title={`${annotation.label}, ${annotation.atLabel}${annotation.detail ? `, ${annotation.detail}` : ""}`}
            key={`${annotation.kind}-${annotation.atLabel}-${index}`}
          >
            {annotationSymbol(annotation.kind)}
          </span>
        ))}
      </div>
    </div>
  );
}

function driftX(index: number, count: number): number {
  return count < 2 ? 50 : 2 + (index / (count - 1)) * 96;
}

function driftY(value: number, minimum: number, maximum: number): number {
  const span = Math.max(1, maximum - minimum);
  return 96 - ((value - minimum) / span) * 92;
}

export function MedicationReportDrift({ report }: { report: MedicationClinicalReport }) {
  const points = report.drift.points;
  const band = useMemo(() => {
    if (points.length === 0) return "";
    const upper = points.map(
      (point, index) =>
        `${driftX(index, points.length)},${driftY(point.bandHighHour, report.drift.yMinHour, report.drift.yMaxHour)}`,
    );
    const lower = [...points].reverse().map((point, reverseIndex) => {
      const index = points.length - 1 - reverseIndex;
      return `${driftX(index, points.length)},${driftY(point.bandLowHour, report.drift.yMinHour, report.drift.yMaxHour)}`;
    });
    return [...upper, ...lower].join(" ");
  }, [points, report.drift.yMaxHour, report.drift.yMinHour]);
  const fit = points
    .map(
      (point, index) =>
        `${driftX(index, points.length)},${driftY(point.fitHour, report.drift.yMinHour, report.drift.yMaxHour)}`,
    )
    .join(" ");

  return (
    <section className="medication-report-figure" aria-labelledby="clinical-drift-title">
      <header className="medication-report-subheading">
        <div>
          <p className="section-kicker">Observed phase</p>
          <h3 id="clinical-drift-title">Sleep-onset drift</h3>
        </div>
        <span>{report.drift.slopeLabel}</span>
      </header>
      <p className="medication-report-figure-summary">{report.drift.summary}</p>
      {points.length > 0 ? (
        <div className="clinical-drift-plot" role="img" aria-label={report.drift.summary}>
          <svg viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
            <line x1="0" x2="100" y1="50" y2="50" className="clinical-drift-grid" />
            <polygon points={band} className="clinical-drift-band" />
            <polyline
              points={fit}
              className="clinical-drift-fit"
              vectorEffect="non-scaling-stroke"
            />
            {points.map((point, index) => (
              <circle
                cx={driftX(index, points.length)}
                cy={driftY(point.onsetHour, report.drift.yMinHour, report.drift.yMaxHour)}
                r="1.5"
                className="clinical-drift-point"
                vectorEffect="non-scaling-stroke"
                key={point.id}
              />
            ))}
          </svg>
          <div className="clinical-drift-axis" aria-hidden="true">
            <span>{points[0]?.day}</span>
            <span>{points.at(-1)?.day}</span>
          </div>
        </div>
      ) : (
        <p className="medication-report-empty">No usable drift points in this range.</p>
      )}
      <table className="sr-table">
        <caption>Observed sleep-onset drift points</caption>
        <thead>
          <tr>
            <th>Date</th>
            <th>Onset</th>
            <th>Source</th>
            <th>Confidence</th>
          </tr>
        </thead>
        <tbody>
          {points.map((point: MedicationClinicalDriftPoint) => (
            <tr key={point.id}>
              <td>{point.civilDate}</td>
              <td>{point.onsetLabel}</td>
              <td>{point.source}</td>
              <td>{point.confidence}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

export function MedicationReportTables({ report }: { report: MedicationClinicalReport }) {
  return (
    <div className="medication-report-tables">
      <section aria-labelledby="adherence-summary-title">
        <header className="medication-report-subheading">
          <div>
            <p className="section-kicker">Recorded evidence only</p>
            <h3 id="adherence-summary-title">Adherence summary</h3>
          </div>
          <span>{report.summary.recordedScheduled} scheduled records</span>
        </header>
        <p className="medication-report-figure-summary">
          Missing logs are not interpreted as missed doses. Excluded events are omitted.
        </p>
        {report.adherence.length > 0 ? (
          <div className="medication-report-table-wrap">
            <table className="medication-report-table">
              <thead>
                <tr>
                  <th>Medication</th>
                  <th>Recorded scheduled</th>
                  <th>Taken</th>
                  <th>Skipped</th>
                  <th>As needed</th>
                </tr>
              </thead>
              <tbody>
                {report.adherence.map((item, index) => (
                  <tr key={`${item.medicationLabel}-${index}`}>
                    <th scope="row">{item.medicationLabel}</th>
                    <td>{item.recordedScheduled}</td>
                    <td>{item.taken}</td>
                    <td>{item.skipped}</td>
                    <td>{item.asNeeded}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="medication-report-empty">No included medication events.</p>
        )}
      </section>

      <section aria-labelledby="medication-event-table-title">
        <header className="medication-report-subheading">
          <div>
            <p className="section-kicker">Included records</p>
            <h3 id="medication-event-table-title">Medication timing</h3>
          </div>
          <span>{report.events.length} events</span>
        </header>
        {report.events.length > 0 ? (
          <div className="medication-report-table-wrap">
            <table className="medication-report-table">
              <thead>
                <tr>
                  <th>Time</th>
                  <th>Medication</th>
                  <th>Status</th>
                  <th>Schedule</th>
                  <th>Rhythm context</th>
                </tr>
              </thead>
              <tbody>
                {report.events.map((event, index) => (
                  <tr key={`${event.civilTime}-${event.medicationLabel}-${index}`}>
                    <td>{event.civilTime}</td>
                    <td>{event.medicationLabel}</td>
                    <td>{event.status}</td>
                    <td>{event.scheduleContext}</td>
                    <td>
                      {event.wakeContext}; {event.sleepContext} ({event.confidence})
                      {event.note && <small>{event.note}</small>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="medication-report-empty">No included medication events.</p>
        )}
      </section>
    </div>
  );
}

export function MedicationReportAssociations({ report }: { report: MedicationClinicalReport }) {
  if (report.associations.length === 0) return null;
  return (
    <section className="medication-report-associations" aria-labelledby="association-title">
      <header className="medication-report-subheading">
        <div>
          <p className="section-kicker">Descriptive comparison</p>
          <h3 id="association-title">Timing around recorded starts</h3>
        </div>
        <span>No causal inference</span>
      </header>
      {report.associations.map((association, index) => (
        <article key={`${association.medicationLabel}-${association.startedLabel}-${index}`}>
          <h4>{association.medicationLabel}</h4>
          <p>
            Recorded start: {association.startedLabel}. {association.message}
          </p>
          <dl>
            <div>
              <dt>Before</dt>
              <dd>
                {association.before.slopeLabel}; {association.before.episodeCount} episodes;{" "}
                {association.before.confidence}
              </dd>
            </div>
            <div>
              <dt>After</dt>
              <dd>
                {association.after.slopeLabel}; {association.after.episodeCount} episodes;{" "}
                {association.after.confidence}
              </dd>
            </div>
          </dl>
          {association.context.length > 0 && (
            <div className="medication-report-context">
              <strong>Other recorded context in the comparison window</strong>
              <ul>
                {association.context.map((context, index) => (
                  <li key={`${context.kindLabel}-${context.rangeLabel}-${index}`}>
                    {context.kindLabel}: {context.timingLabel}, {context.rangeLabel}
                    {context.note ? `; ${context.note}` : ""}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </article>
      ))}
    </section>
  );
}

export function MedicationReportSummary({
  report,
  stale,
}: {
  report: MedicationClinicalReport;
  stale: boolean;
}) {
  return (
    <>
      <div className="medication-report-status" data-status={report.status}>
        <div>
          <strong>{stale ? "Preview controls changed" : report.message}</strong>
          <span>
            {stale
              ? "Generate the preview again before exporting."
              : `${report.range.label}; generated ${report.generatedLabel}`}
          </span>
        </div>
        <span>{report.status}</span>
      </div>
      <dl className="medication-report-metrics">
        <div>
          <dt>Calendar rows</dt>
          <dd>{report.summary.calendarRows}</dd>
        </div>
        <div>
          <dt>Sleep segments</dt>
          <dd>{report.summary.observedSleepSegments}</dd>
        </div>
        <div>
          <dt>No-data rows</dt>
          <dd>{report.summary.noDataRows}</dd>
        </div>
        <div>
          <dt>Medication events</dt>
          <dd>{report.summary.medicationEvents}</dd>
        </div>
        <div>
          <dt>Excluded events</dt>
          <dd>{report.summary.excludedEvents}</dd>
        </div>
        <div>
          <dt>Context markers</dt>
          <dd>{report.summary.rhythmContextMarkers}</dd>
        </div>
      </dl>
      <div className="medication-report-redactions">
        <strong>Applied before preview and export</strong>
        <ul>
          {report.redactions.map((redaction) => (
            <li key={redaction}>{redaction}</li>
          ))}
        </ul>
      </div>
    </>
  );
}

import {
  medicationReportExportConfirmation,
  type MedicationClinicalReport,
  type MedicationClinicalReportInput,
} from "../data/medicationReport";
import {
  MedicationReportActogram,
  MedicationReportAssociations,
  MedicationReportDrift,
  MedicationReportSummary,
  MedicationReportTables,
} from "./MedicationClinicalReportPreview";

type UpdateReportInput = (next: Partial<MedicationClinicalReportInput>) => void;

export function MedicationReportControls({
  input,
  busy,
  canGenerate,
  loading,
  hasReport,
  onChange,
  onRecentRange,
  onSubmit,
}: {
  input: MedicationClinicalReportInput;
  busy: boolean;
  canGenerate: boolean;
  loading: boolean;
  hasReport: boolean;
  onChange: UpdateReportInput;
  onRecentRange: (days: 7 | 31) => void;
  onSubmit: () => void;
}) {
  return (
    <form
      className="medication-report-controls"
      aria-label="Clinician report controls"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
    >
      <fieldset>
        <legend>Range and clinical day</legend>
        <label>
          <span>Range</span>
          <select
            value={input.rangeMode}
            disabled={busy}
            onChange={(event) => onChange({ rangeMode: event.target.value as "custom" | "all" })}
          >
            <option value="custom">Custom dates</option>
            <option value="all">All local records</option>
          </select>
        </label>
        <div
          className="medication-report-range-shortcuts"
          role="group"
          aria-label="Quick date ranges"
        >
          <span>Quick range</span>
          <div>
            <button
              className="button ghost compact"
              type="button"
              disabled={busy}
              onClick={() => onRecentRange(7)}
            >
              Past 7 days
            </button>
            <button
              className="button ghost compact"
              type="button"
              disabled={busy}
              onClick={() => onRecentRange(31)}
            >
              Past 31 days
            </button>
          </div>
        </div>
        <label>
          <span>From</span>
          <input
            type="date"
            value={input.fromDate}
            disabled={input.rangeMode === "all" || busy}
            onChange={(event) => onChange({ fromDate: event.target.value })}
          />
        </label>
        <label>
          <span>Through</span>
          <input
            type="date"
            value={input.toDate}
            disabled={input.rangeMode === "all" || busy}
            onChange={(event) => onChange({ toDate: event.target.value })}
          />
        </label>
        <label>
          <span>Row anchor</span>
          <select
            value={input.dayStartHour}
            disabled={busy}
            onChange={(event) => onChange({ dayStartHour: Number(event.target.value) as 12 | 18 })}
          >
            <option value={18}>6 PM (recommended)</option>
            <option value={12}>Noon</option>
          </select>
        </label>
        <label>
          <span>IANA time zone</span>
          <input
            value={input.zoneId}
            disabled={busy}
            onChange={(event) => onChange({ zoneId: event.target.value })}
          />
        </label>
      </fieldset>

      <fieldset>
        <legend>Layers and private detail</legend>
        <label className="medication-check-row">
          <input
            type="checkbox"
            checked={input.includeMedication}
            disabled={busy}
            onChange={(event) =>
              onChange({
                includeMedication: event.target.checked,
                ...(!event.target.checked
                  ? { includeMedicationLabels: false, includeMedicationNotes: false }
                  : {}),
              })
            }
          />
          <span>Include medication records with anonymous labels</span>
        </label>
        <label className="medication-check-row medication-report-private-option">
          <input
            type="checkbox"
            checked={input.includeMedicationLabels}
            disabled={!input.includeMedication || busy}
            onChange={(event) => onChange({ includeMedicationLabels: event.target.checked })}
          />
          <span>Include private medication labels and strength text</span>
        </label>
        <label className="medication-check-row medication-report-private-option">
          <input
            type="checkbox"
            checked={input.includeMedicationNotes}
            disabled={!input.includeMedication || busy}
            onChange={(event) => onChange({ includeMedicationNotes: event.target.checked })}
          />
          <span>Include private medication notes</span>
        </label>
        <label className="medication-check-row">
          <input
            type="checkbox"
            checked={input.includeRhythmContext}
            disabled={busy}
            onChange={(event) =>
              onChange({
                includeRhythmContext: event.target.checked,
                ...(!event.target.checked ? { includeRhythmContextNotes: false } : {}),
              })
            }
          />
          <span>Include travel, illness, disruption, and forced-schedule markers</span>
        </label>
        <label className="medication-check-row medication-report-private-option">
          <input
            type="checkbox"
            checked={input.includeRhythmContextNotes}
            disabled={!input.includeRhythmContext || busy}
            onChange={(event) => onChange({ includeRhythmContextNotes: event.target.checked })}
          />
          <span>Include private rhythm-context notes</span>
        </label>
        <label className="medication-check-row">
          <input
            type="checkbox"
            checked={input.includeForecast}
            disabled={busy}
            onChange={(event) => onChange({ includeForecast: event.target.checked })}
          />
          <span>Include model forecast (off by default)</span>
        </label>
      </fieldset>

      <div className="medication-report-control-actions">
        <p>
          Diagnosis, location, and clinician-entered guidance are always omitted. Nothing is
          uploaded.
        </p>
        <button className="button primary compact" type="submit" disabled={!canGenerate}>
          {loading ? "Generating..." : hasReport ? "Regenerate preview" : "Generate preview"}
        </button>
      </div>
    </form>
  );
}

export function MedicationReportPreviewBody({
  report,
  stale,
  loading,
  exporting,
  exportOpen,
  confirmation,
  onOpenExport,
  onCancelExport,
  onConfirmationChange,
  onExport,
}: {
  report: MedicationClinicalReport;
  stale: boolean;
  loading: boolean;
  exporting: boolean;
  exportOpen: boolean;
  confirmation: string;
  onOpenExport: () => void;
  onCancelExport: () => void;
  onConfirmationChange: (value: string) => void;
  onExport: () => void;
}) {
  return (
    <div className="medication-report-preview" aria-busy={loading || undefined}>
      <MedicationReportSummary report={report} stale={stale} />
      <div className="medication-report-visuals">
        <MedicationReportActogram
          key={`${report.generatedAt}-${report.range.fromDate}-${report.range.toDate}-${report.summary.calendarRows}-${report.summary.observedSleepSegments}-${report.summary.medicationEvents}`}
          report={report}
        />
        <MedicationReportDrift report={report} />
      </div>
      <MedicationReportTables report={report} />
      <MedicationReportAssociations report={report} />

      <section className="medication-report-provenance" aria-labelledby="provenance-title">
        <h3 id="provenance-title">Provenance and limits</h3>
        <ul>
          {report.provenance.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
        <p>{report.notice}</p>
      </section>

      <section className="medication-report-export" aria-labelledby="report-export-title">
        <div>
          <p className="section-kicker">Local file</p>
          <h3 id="report-export-title">Printable HTML export</h3>
          <p>The standalone file uses the preview settings and can be printed to PDF.</p>
        </div>
        {!exportOpen ? (
          <button
            className="button secondary compact"
            type="button"
            disabled={stale || loading}
            onClick={onOpenExport}
          >
            Prepare HTML export
          </button>
        ) : (
          <div className="medication-report-export-confirmation">
            <label>
              <span>Type {medicationReportExportConfirmation} to create the file</span>
              <input
                value={confirmation}
                autoComplete="off"
                disabled={exporting}
                onChange={(event) => onConfirmationChange(event.target.value)}
              />
            </label>
            <div>
              <button
                className="button ghost compact"
                type="button"
                disabled={exporting}
                onClick={onCancelExport}
              >
                Cancel
              </button>
              <button
                className="button primary compact"
                type="button"
                disabled={exporting || stale || confirmation !== medicationReportExportConfirmation}
                onClick={onExport}
              >
                {exporting ? "Preparing..." : "Create HTML report"}
              </button>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}

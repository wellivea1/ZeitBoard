import { useState } from "react";

import {
  downloadTranscriptionTemplate,
  importSleepData,
  previewSleepImport,
  type SleepImportInput,
  type SleepImportReport,
  type SleepImportRow,
} from "../data/sleepImport";
import { notifySleepDataChanged } from "../data/sleepDataEvents";

const maxImportBytes = 8 * 1024 * 1024;
const importRowsPerPage = 100;

function ImportCounts({ report }: { report: SleepImportReport }) {
  return (
    <dl className="sleep-import-counts" aria-label="Import row counts">
      <div>
        <dt>Total</dt>
        <dd>{report.totalRows}</dd>
      </div>
      <div>
        <dt>Ready</dt>
        <dd>{report.readyRows}</dd>
      </div>
      <div>
        <dt>Duplicates</dt>
        <dd>{report.duplicateRows}</dd>
      </div>
      <div>
        <dt>Invalid</dt>
        <dd>{report.invalidRows}</dd>
      </div>
      {report.importedRows > 0 && (
        <div>
          <dt>Imported</dt>
          <dd>{report.importedRows}</dd>
        </div>
      )}
    </dl>
  );
}

function RowResult({ row }: { row: SleepImportRow }) {
  return (
    <tr data-status={row.status}>
      <td>{row.rowNumber}</td>
      <td>
        <strong>{row.sourceRecordId || "Missing source record id"}</strong>
        {row.observationId && <small>{row.observationId}</small>}
      </td>
      <td>
        {row.startLabel && row.endLabel
          ? `${row.startLabel} to ${row.endLabel}`
          : "No valid interval"}
        {row.zoneId && <small>{row.zoneId}</small>}
      </td>
      <td>{row.classification || "-"}</td>
      <td>
        <span className={`import-row-status ${row.status}`}>{row.status}</span>
        {row.statusDetail && <small>{row.statusDetail}</small>}
        {row.errors.length > 0 && (
          <ul>
            {row.errors.map((error) => (
              <li key={error}>{error}</li>
            ))}
          </ul>
        )}
      </td>
    </tr>
  );
}

function ImportResults({ report }: { report: SleepImportReport }) {
  const [page, setPage] = useState(0);
  const pageCount = Math.max(1, Math.ceil(report.rows.length / importRowsPerPage));
  const currentPage = Math.min(page, pageCount - 1);
  const firstRow = currentPage * importRowsPerPage;
  const visibleRows = report.rows.slice(firstRow, firstRow + importRowsPerPage);

  return (
    <div className="sleep-import-results">
      <ImportCounts report={report} />
      {report.errors.length > 0 && (
        <div className="sleep-import-document-errors" role="alert">
          <strong>File errors</strong>
          <ul>
            {report.errors.map((error) => (
              <li key={error}>{error}</li>
            ))}
          </ul>
        </div>
      )}
      {report.rows.length > 0 && (
        <details open={report.invalidRows > 0} className="sleep-import-row-details">
          <summary>Review all {report.totalRows} row results</summary>
          {report.rows.length > importRowsPerPage && (
            <div className="sleep-import-pagination" aria-label="Import result pages">
              <span>
                Rows {firstRow + 1}-{Math.min(firstRow + importRowsPerPage, report.rows.length)} of{" "}
                {report.rows.length}
              </span>
              <button
                className="button secondary"
                type="button"
                disabled={currentPage === 0}
                onClick={() => setPage((value) => Math.max(0, value - 1))}
              >
                Previous rows
              </button>
              <button
                className="button secondary"
                type="button"
                disabled={currentPage >= pageCount - 1}
                onClick={() => setPage((value) => Math.min(pageCount - 1, value + 1))}
              >
                Next rows
              </button>
            </div>
          )}
          <div className="sleep-import-table-scroll">
            <table className="sleep-import-table">
              <thead>
                <tr>
                  <th scope="col">Row</th>
                  <th scope="col">Source record</th>
                  <th scope="col">Sleep interval</th>
                  <th scope="col">Class</th>
                  <th scope="col">Result</th>
                </tr>
              </thead>
              <tbody>
                {visibleRows.map((row) => (
                  <RowResult
                    key={`${row.rowNumber}-${row.sourceRecordId || "missing"}`}
                    row={row}
                  />
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}
    </div>
  );
}

export function SleepImportPanel({ onImported }: { onImported: () => Promise<void> }) {
  const [input, setInput] = useState<SleepImportInput | null>(null);
  const [report, setReport] = useState<SleepImportReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const chooseFile = async (file: File | undefined) => {
    setInput(null);
    setReport(null);
    setError("");
    if (!file) return;
    if (file.size > maxImportBytes) {
      setError("The selected file exceeds the 8 MiB import limit.");
      return;
    }
    setBusy(true);
    try {
      const selected = { fileName: file.name, contents: await file.text() };
      setInput(selected);
      setReport(await previewSleepImport(selected));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not preview the import file.");
    } finally {
      setBusy(false);
    }
  };

  const commit = async () => {
    if (!input || !report?.canImport) return;
    setBusy(true);
    setError("");
    try {
      const committed = await importSleepData(input);
      setReport(committed);
      if (committed.importedRows > 0) {
        notifySleepDataChanged();
        await onImported();
      }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not import sleep data.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="panel sleep-import-panel" aria-labelledby="sleep-import-title">
      <div className="panel-heading sleep-import-heading">
        <div>
          <p className="section-kicker">Local file import</p>
          <h2 id="sleep-import-title">Import a v1 observation set</h2>
        </div>
        <span className="source-status connected">Local only</span>
      </div>
      <p className="sleep-import-intro">
        Choose contract-shaped JSON or canonical CSV. Preview is read-only; import reruns every
        check in one append-only transaction. Invalid rows block the whole file, and exact source
        record duplicates remain visible in the report.
      </p>
      <div className="sleep-import-controls">
        <label className="sleep-import-picker">
          Observation file
          <input
            type="file"
            accept=".json,.csv,application/json,text/csv"
            disabled={busy}
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.target.value = "";
              void chooseFile(file);
            }}
          />
        </label>
        <button
          className="button secondary"
          type="button"
          disabled={busy}
          onClick={downloadTranscriptionTemplate}
        >
          Download transcription template
        </button>
        <button
          className="button primary"
          type="button"
          disabled={busy || !report?.canImport}
          onClick={() => void commit()}
        >
          {busy ? "Checking..." : `Import ${report?.readyRows ?? 0} ready rows`}
        </button>
      </div>
      <p className="sleep-import-note">
        Handwritten charts require owner-reviewed CSV transcription and the local converter. Each
        dated row stays needs_review until marked confirmed_sleep or confirmed_no_observation.
        ZeitBoard does not claim to recognize handwriting or silently infer missing times.
      </p>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {report && (
        <>
          <p className="form-status" role="status" aria-live="polite">
            {report.message}
          </p>
          <ImportResults
            key={`${report.fileName}-${String(report.dryRun)}-${report.importedRows}`}
            report={report}
          />
        </>
      )}
    </section>
  );
}

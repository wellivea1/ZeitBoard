import { deleteConfirmationToken, type SleepDataExportSummary } from "../../data/sleepDataControl";
import { StorageProtectionPanel } from "./StorageProtectionPanel";

interface SleepDataSettingsProps {
  exported: SleepDataExportSummary | null;
  confirmation: string;
  busy: boolean;
  error: string;
  message: string;
  onConfirmationChange: (value: string) => void;
  onExport: () => void;
  onErase: () => void;
}

export function SleepDataSettings({
  exported,
  confirmation,
  busy,
  error,
  message,
  onConfirmationChange,
  onExport,
  onErase,
}: SleepDataSettingsProps) {
  return (
    <section className="settings-section data-controls-panel">
      <div className="data-control-intro">
        <h2>Your sleep data</h2>
        <p className="settings-copy">
          Keep a copy, or erase everything. Erasing cannot be undone. With sync on, the next sync
          also deletes the server&apos;s copy and tells your other devices to erase theirs.
        </p>
      </div>
      <div className="data-control-grid">
        <section className="data-control-card" aria-labelledby="sleep-export-title">
          <div>
            <h3 id="sleep-export-title">Export sleep data</h3>
            <p>A JSON file with every night and correction, to keep as a backup or import later.</p>
          </div>
          <button className="button secondary" type="button" onClick={onExport} disabled={busy}>
            Export sleep data
          </button>
          {exported && (
            <div className="export-summary">
              <dl>
                <div>
                  <dt>File</dt>
                  <dd>{exported.fileName}</dd>
                </div>
                <div>
                  <dt>Generated</dt>
                  <dd>{exported.generatedLabel}</dd>
                </div>
                <div>
                  <dt>Contents</dt>
                  <dd>
                    {exported.observationCount}{" "}
                    {exported.observationCount === 1 ? "observation" : "observations"},{" "}
                    {exported.correctionCount}{" "}
                    {exported.correctionCount === 1 ? "correction" : "corrections"}
                  </dd>
                </div>
              </dl>
              <pre className="export-preview" aria-label="Sleep data export JSON preview">
                {exported.preview}
              </pre>
              {exported.previewTruncated && <small>Preview truncated after 512 characters.</small>}
            </div>
          )}
        </section>
        <section className="data-control-card danger-zone" aria-labelledby="sleep-delete-title">
          <div>
            <h3 id="sleep-delete-title">Erase local sleep data</h3>
            <p>
              Deletes every sleep record and correction on this computer. To leave a single night
              out of estimates, use Exclude in Log instead.
            </p>
          </div>
          <label htmlFor="delete-all-sleep-data">
            Type DELETE to erase all local sleep data
            <input
              id="delete-all-sleep-data"
              type="text"
              value={confirmation}
              disabled={busy}
              onChange={(event) => onConfirmationChange(event.target.value)}
            />
          </label>
          <button
            className="button danger"
            type="button"
            onClick={onErase}
            disabled={busy || confirmation !== deleteConfirmationToken}
          >
            Erase all sleep data
          </button>
        </section>
        <StorageProtectionPanel />
      </div>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <p className="form-status" role="status" aria-live="polite">
        {message}
      </p>
    </section>
  );
}

import { deleteConfirmationToken } from "../../data/sleepDataControl";
import type { SleepDataExport } from "../../data/sleepEntries";

interface SleepDataSettingsProps {
  exported: SleepDataExport | null;
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
        <p className="section-kicker">Local data</p>
        <h2>Storage</h2>
        <p className="settings-copy">
          Export produces v1 JSON with observation-set and correction-set sections. Suppress appends
          a correction; erase permanently removes local observations and their correction history.
          If backend sync is on, erasure also propagates on the next sync: the server hard-deletes
          its copy and a tombstone tells your other devices to erase theirs. ZeitBoard cannot
          restore an erased record.
        </p>
      </div>
      <div className="data-control-grid">
        <section className="data-control-card" aria-labelledby="sleep-export-title">
          <div>
            <h3 id="sleep-export-title">Export sleep data</h3>
            <p>Download contract-shaped JSON for backup, review, or later import tooling.</p>
          </div>
          <button className="button secondary" type="button" onClick={onExport} disabled={busy}>
            Export sleep data
          </button>
          {exported && (
            <textarea
              className="export-preview"
              aria-label="Sleep data export JSON"
              readOnly
              rows={8}
              value={exported.json}
            />
          )}
        </section>
        <section className="data-control-card danger-zone" aria-labelledby="sleep-delete-title">
          <div>
            <h3 id="sleep-delete-title">Erase local sleep data</h3>
            <p>
              This hard-deletes local sleep observations and sleep correction history. It is not the
              append-only suppress action.
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

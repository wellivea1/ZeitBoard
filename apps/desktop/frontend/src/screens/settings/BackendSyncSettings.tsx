import type { BackendSyncInput, BackendSyncStatus } from "../../data/backendSync";

interface BackendSyncSettingsProps {
  status: BackendSyncStatus;
  form: BackendSyncInput;
  busy: boolean;
  error: string;
  message: string;
  onFormChange: (changes: Partial<BackendSyncInput>) => void;
  onConfigure: () => void;
  onDisable: () => void;
  onSyncNow: () => void;
}

function statusLabel(status: BackendSyncStatus) {
  if (!status.enabled) return "Off";
  if (status.status === "error") return "Needs attention";
  return "Connected";
}

export function BackendSyncSettings({
  status,
  form,
  busy,
  error,
  message,
  onFormChange,
  onConfigure,
  onDisable,
  onSyncNow,
}: BackendSyncSettingsProps) {
  return (
    <section className="settings-section backend-sync-panel">
      <div className="data-control-intro">
        <p className="section-kicker">Backend sync</p>
        <h2>Self-hosted server</h2>
        <p className="settings-copy">
          Sync is off by default. When enabled, only v1 sleep observations, corrections, and your
          task list are sent to your enrolled self-hosted backend. Deleting a task or erasing sleep
          data propagates the deletion to the server and every synced device. Overview and Rhythm
          clearly label synced server estimates.
        </p>
      </div>
      <div className="data-control-grid">
        <section
          className="data-control-card backend-sync-card"
          aria-labelledby="backend-sync-connect-title"
        >
          <div>
            <h3 id="backend-sync-connect-title">Connect backend</h3>
            <p>
              Use an HTTPS URL and enrollment secret from your own server. The device token is
              stored outside the editable config and is never shown here.
            </p>
          </div>
          <form
            className="backend-sync-form"
            onSubmit={(event) => {
              event.preventDefault();
              onConfigure();
            }}
          >
            <label htmlFor="backend-sync-url">
              Backend URL
              <input
                id="backend-sync-url"
                type="url"
                placeholder="https://zeitboard.example.com"
                value={form.backendUrl}
                disabled={busy}
                onChange={(event) => onFormChange({ backendUrl: event.target.value })}
              />
            </label>
            <label htmlFor="backend-sync-secret">
              Enrollment secret
              <input
                id="backend-sync-secret"
                type="password"
                value={form.enrollmentSecret}
                disabled={busy}
                onChange={(event) => onFormChange({ enrollmentSecret: event.target.value })}
              />
            </label>
            <label htmlFor="backend-sync-label">
              Device label
              <input
                id="backend-sync-label"
                type="text"
                value={form.deviceLabel}
                disabled={busy}
                onChange={(event) => onFormChange({ deviceLabel: event.target.value })}
              />
            </label>
            <label className="toggle-row backend-sync-dev-toggle" htmlFor="backend-sync-insecure">
              <span>
                <strong>Allow self-signed localhost TLS</strong>
                <small>Development only. Production sync verifies HTTPS certificates.</small>
              </span>
              <input
                id="backend-sync-insecure"
                type="checkbox"
                checked={form.insecureSkipVerify}
                disabled={busy}
                onChange={(event) => onFormChange({ insecureSkipVerify: event.target.checked })}
              />
            </label>
            <div className="backend-sync-actions">
              <button className="button primary" type="submit" disabled={busy || !form.backendUrl}>
                Enable backend sync
              </button>
              <button
                className="button secondary"
                type="button"
                onClick={onDisable}
                disabled={busy || !status.enabled}
              >
                Disable sync
              </button>
            </div>
          </form>
        </section>
        <section
          className="data-control-card backend-sync-card"
          aria-labelledby="backend-sync-status-title"
        >
          <div>
            <h3 id="backend-sync-status-title">Sync status</h3>
            <p>
              Backend unavailable falls back to local estimates. Conflicts are reported here and do
              not crash the app.
            </p>
          </div>
          <dl className="sync-status-list">
            <div>
              <dt>Status</dt>
              <dd>{statusLabel(status)}</dd>
            </div>
            <div>
              <dt>Backend</dt>
              <dd>{status.backendUrl || "Not configured"}</dd>
            </div>
            <div>
              <dt>Device</dt>
              <dd>{status.deviceId || "Not enrolled"}</dd>
            </div>
            <div>
              <dt>Pending push</dt>
              <dd>{status.pendingPushCount}</dd>
            </div>
            <div>
              <dt>Last sync</dt>
              <dd>{status.lastSyncLabel}</dd>
            </div>
          </dl>
          <button
            className="button secondary"
            type="button"
            onClick={onSyncNow}
            disabled={busy || !status.enabled}
          >
            Sync now
          </button>
          {status.lastError && <p className="form-error">Last error: {status.lastError}</p>}
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

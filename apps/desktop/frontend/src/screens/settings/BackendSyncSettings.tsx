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
        <h2>Self-hosted server</h2>
        <p className="settings-copy">
          Off unless you turn it on. Then your sleep records, corrections and tasks go to your own
          server, and a deletion reaches every synced device. Activity and medication records stay
          on this computer.
        </p>
        {/* The detail matters for trust, but not on every visit. */}
        <details className="settings-more fold">
          <summary>How sync behaves</summary>
          <p className="settings-copy">
            Sync runs after enrollment, when ZeitBoard starts, and about every minute while it is
            running, including when its window is hidden. Interrupted exchanges retry from saved
            records; Quit stops it. Home and Rhythm label estimates that come from the server.
          </p>
          <p className="settings-copy">
            Re-enrolling downloads the server history before replaying missing saved records. A
            failed connection attempt keeps your previous enrollment. Retained deletion markers stop
            erased records from returning after a restore. Changing servers sends this profile's
            sleep records, tasks and deletion markers to the new server; it does not delete the old
            server's copies.
          </p>
        </details>
      </div>
      <div className="data-control-grid">
        <section
          className="data-control-card backend-sync-card"
          aria-labelledby="backend-sync-connect-title"
        >
          <div>
            <h3 id="backend-sync-connect-title">Connect backend</h3>
            <p>From your own server. The device token it issues is never shown.</p>
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
              <dt>Pending deletions</dt>
              <dd>{status.pendingErasureCount}</dd>
            </div>
            <div>
              <dt>Corrections waiting for sources</dt>
              <dd>{status.waitingCorrectionCount}</dd>
            </div>
            <div>
              <dt>Tasks needing review</dt>
              <dd>
                {status.taskConflictCount}
                {status.taskConflictCount > 0 && (
                  <>
                    {" "}
                    · <a href="#/plan/tasks">Review conflicting edits</a>
                  </>
                )}
              </dd>
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

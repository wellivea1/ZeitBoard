import type {
  BackendSyncInput,
  BackendSyncStatus,
  ServerCertificate,
} from "../../data/backendSync";
import { momentInSentence } from "../../utils/relativeTime";

interface BackendSyncSettingsProps {
  status: BackendSyncStatus;
  form: BackendSyncInput;
  /** The certificate trusted for the server, chosen or kept; none when absent. */
  certificate?: ServerCertificate;
  busy: boolean;
  error: string;
  message: string;
  onFormChange: (changes: Partial<BackendSyncInput>) => void;
  onChooseCertificate: () => void;
  onRemoveCertificate: () => void;
  onConfigure: () => void;
  onDisable: () => void;
  onSyncNow: () => void;
}

function fingerprintLines(fingerprint: string) {
  const bytes = fingerprint.split(":");
  return [bytes.slice(0, 16).join(":"), bytes.slice(16).join(":")].filter(Boolean);
}

// A server with its own certificate or authority is trusted by that
// certificate, with verification on (ADR-0055). The fingerprint is shown in
// full: it is how the owner checks it against what the server prints.
function CertificateRow({
  certificate,
  busy,
  onChoose,
  onRemove,
}: {
  certificate?: ServerCertificate;
  busy: boolean;
  onChoose: () => void;
  onRemove: () => void;
}) {
  const until = certificate
    ? new Date(certificate.expiresAt).toLocaleDateString(undefined, {
        month: "short",
        day: "numeric",
        year: "numeric",
      })
    : "";
  return (
    <div className="backend-sync-certificate">
      <span>
        <strong>Server certificate</strong>
        {certificate ? (
          <>
            <small>
              {certificate.subject || "Unnamed"}, until {until}
            </small>
            {/* Two lines of sixteen bytes, so a line never breaks inside one. */}
            <code aria-label={`SHA-256 fingerprint ${certificate.fingerprint}`}>
              {fingerprintLines(certificate.fingerprint).map((line) => (
                <span key={line}>{line}</span>
              ))}
            </code>
          </>
        ) : (
          <small>
            Only for a server with its own certificate or authority. Verification stays on.
          </small>
        )}
      </span>
      {certificate ? (
        <button className="button ghost compact" type="button" disabled={busy} onClick={onRemove}>
          Remove
        </button>
      ) : (
        <button
          className="button secondary compact"
          type="button"
          disabled={busy}
          onClick={onChoose}
        >
          Choose certificate
        </button>
      )}
    </div>
  );
}

// The value beside "Last sync": "Today at 2:51 PM", or "Not yet".
function lastSyncWording(lastSyncAt: string | undefined) {
  if (!lastSyncAt) return "Not yet";
  const moment = momentInSentence(new Date(lastSyncAt), new Date());
  return moment.charAt(0).toUpperCase() + moment.slice(1);
}

function statusLabel(status: BackendSyncStatus) {
  if (!status.enabled) return "Off";
  if (status.status === "error") return "Needs attention";
  return "Connected";
}

export function BackendSyncSettings({
  status,
  form,
  certificate,
  busy,
  error,
  message,
  onFormChange,
  onChooseCertificate,
  onRemoveCertificate,
  onConfigure,
  onDisable,
  onSyncNow,
}: BackendSyncSettingsProps) {
  return (
    <section className="settings-section backend-sync-panel">
      <div className="data-control-intro">
        <h2>Self-hosted server</h2>
        <p className="settings-copy">
          Off unless you turn it on. Then your sleep records, tasks and accepted times, medications
          and doses, and context markers go to your own server, and a deletion reaches every synced
          device. Activity records stay on this computer.
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
            <CertificateRow
              certificate={certificate}
              busy={busy}
              onChoose={onChooseCertificate}
              onRemove={onRemoveCertificate}
            />
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
              <dt>Downloads waiting for related records</dt>
              <dd>{status.waitingRecordCount}</dd>
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
              <dd>{lastSyncWording(status.lastSyncAt)}</dd>
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

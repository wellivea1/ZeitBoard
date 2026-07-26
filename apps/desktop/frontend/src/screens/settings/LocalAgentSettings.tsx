import type { LocalAgentStatus } from "../../data/localAgent";

interface LocalAgentSettingsProps {
  status: LocalAgentStatus | null;
  error: string;
}

function capabilityStatus(status: LocalAgentStatus | null, available: boolean) {
  if (!status) return "Checking";
  return available ? "Available" : "Unavailable";
}

function endpointStatus(status: LocalAgentStatus | null) {
  if (!status) return "Checking";
  return status.running ? "Ready" : "Stopped";
}

export function LocalAgentSettings({ status, error }: LocalAgentSettingsProps) {
  return (
    <section className="settings-section local-agent-panel">
      <div className="data-control-intro">
        <p className="section-kicker">Local assistant</p>
        <h2>Desktop-local agent</h2>
        <p className="settings-copy">
          ZeitBoard serves an authenticated MCP endpoint on this computer. Local reads and
          reversible appearance changes work with backend sync off. Voice and speech recognition
          come from the MCP client; ZeitBoard does not record microphone audio.
        </p>
      </div>

      <div className="data-control-grid">
        <section className="data-control-card" aria-labelledby="local-agent-status-title">
          <div>
            <h3 id="local-agent-status-title">Endpoint status</h3>
            <p>The endpoint listens only on loopback and uses a per-launch bearer credential.</p>
          </div>
          <dl className="sync-status-list">
            <div>
              <dt>Status</dt>
              <dd>{endpointStatus(status)}</dd>
            </div>
            <div>
              <dt>Address</dt>
              <dd>{!status ? "Checking" : status.endpoint || "Not available"}</dd>
            </div>
            <div>
              <dt>Local store</dt>
              <dd>{capabilityStatus(status, status?.localStoreAvailable ?? false)}</dd>
            </div>
            <div>
              <dt>Appearance</dt>
              <dd>
                {!status
                  ? "Checking"
                  : status.appearanceStatus === "error"
                    ? "Needs repair"
                    : "Ready"}
              </dd>
            </div>
            <div>
              <dt>Proposals</dt>
              <dd>
                {!status
                  ? "Checking"
                  : status.backendProposalsAvailable
                    ? "Backend connected; approval still required"
                    : "Unavailable while backend sync is off"}
              </dd>
            </div>
          </dl>
          {status?.message && (
            <p className="settings-copy" role="status">
              {status.message}
            </p>
          )}
        </section>

        <section className="data-control-card" aria-labelledby="local-agent-boundaries-title">
          <div>
            <h3 id="local-agent-boundaries-title">Data and action boundaries</h3>
            <p>
              A connected MCP client receives only reviewed projections. A cloud-backed voice client
              may process those projections off this computer under that client&apos;s privacy
              terms.
            </p>
          </div>
          <ul className="settings-boundary-list">
            <li>Raw sleep records and observed actogram rows are excluded.</li>
            <li>Task titles and notes are excluded.</li>
            <li>
              Medication labels, notes, strength, clinician text, event rows, and exact logged
              timestamps are excluded.
            </li>
            <li>Marker notes and exact record timestamps are excluded.</li>
            <li>Medical decisions are refused; medication and marker output is factual only.</li>
            <li>
              Schedule tools create pending proposals only. They cannot approve or apply a change.
            </li>
          </ul>
        </section>
      </div>

      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}

import { Icon } from "../components/Icon";
import { PageHeader } from "../components/AppShell";

const relationshipTemplates = [
  {
    relationship: "Family",
    fields: "Availability and best-contact window",
    expiry: "Required",
  },
  {
    relationship: "Friend",
    fields: "Availability only",
    expiry: "Required",
  },
  {
    relationship: "Clinician",
    fields: "Predicted sleep and waking windows, plus confidence",
    expiry: "Required",
  },
  {
    relationship: "Collaborator",
    fields: "Availability only",
    expiry: "Short default",
  },
] as const;

export function SharingScreen() {
  return (
    <>
      <PageHeader
        title="Sharing"
        description="Choose the minimum fields, require an expiry, and preview the exact recipient view before creating a link."
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode="fixture" aria-hidden="true" />
            <span>Design preview</span>
          </div>
        }
      />

      <section className="sharing-workspace" aria-label="Sharing capability preview">
        <header className="sharing-state">
          <div className="sharing-state-copy">
            <Icon name="shield" />
            <div>
              <p className="section-kicker">Current state</p>
              <h2>No trusted view is being shared</h2>
              <p>
                Link creation and recipient access are not connected in this build. The examples
                below document minimum-access defaults; they are not people, profiles, or active
                links.
              </p>
            </div>
          </div>
          <dl className="sharing-state-facts">
            <div>
              <dt>Policy</dt>
              <dd>Default deny</dd>
            </div>
            <div>
              <dt>Active links</dt>
              <dd>None</dd>
            </div>
            <div>
              <dt>Transport</dt>
              <dd>Not connected</dd>
            </div>
          </dl>
        </header>

        <section className="sharing-template-section" aria-labelledby="sharing-template-title">
          <div className="sharing-section-heading">
            <div>
              <p className="section-kicker">Relationship templates</p>
              <h2 id="sharing-template-title">Minimum-access examples</h2>
            </div>
            <p>Every permission starts off and must be explicitly granted by the owner.</p>
          </div>

          <div className="sharing-template-table" role="table" aria-label="Sharing templates">
            <div className="sharing-template-head" role="row">
              <span role="columnheader">Relationship</span>
              <span role="columnheader">Recipient could see</span>
              <span role="columnheader">Expiry</span>
              <span role="columnheader">State</span>
            </div>
            {relationshipTemplates.map((template) => (
              <div className="sharing-template-row" role="row" key={template.relationship}>
                <strong role="cell" data-label="Relationship">
                  {template.relationship}
                </strong>
                <span role="cell" data-label="Recipient could see">
                  {template.fields}
                </span>
                <span role="cell" data-label="Expiry">
                  {template.expiry}
                </span>
                <span className="sharing-template-state" role="cell" data-label="State">
                  Example only
                </span>
              </div>
            ))}
          </div>
        </section>

        <aside className="sharing-guardrails" aria-labelledby="sharing-guardrails-title">
          <div>
            <p className="section-kicker">Before a link exists</p>
            <h2 id="sharing-guardrails-title">Required guardrails</h2>
          </div>
          <ol>
            <li>Preview the exact allowlisted recipient view.</li>
            <li>Require an expiry; permanent links are not the default.</li>
            <li>Keep revoke and access history available with every link.</li>
            <li>Render failures as a contentless unavailable page.</li>
          </ol>
          <div className="sharing-private-boundary">
            <strong>Never in a trusted link</strong>
            <span>
              Medication, diagnosis, raw activity, location, private calendar text, and rhythm
              marker notes.
            </span>
          </div>
        </aside>
      </section>
    </>
  );
}

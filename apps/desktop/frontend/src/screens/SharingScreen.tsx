import { Icon } from "../components/Icon";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";

export function SharingScreen() {
  return (
    <>
      <PageHeader
        title="Sharing"
        description="Choose a person, then allow only the minimum fields they need."
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode="fixture" aria-hidden="true" />
            <span>Sample preview</span>
          </div>
        }
      />
      <PlaceholderNotice>
        These profiles are a synthetic design preview — no trusted view is being shared. Sharing
        stays default-deny: every field must be explicitly allowlisted before anyone sees it.
      </PlaceholderNotice>
      <div className="safety-banner">
        <Icon name="shield" />
        <p>
          <strong>Default deny</strong>Medication, diagnosis, raw activity, location, and private
          calendar text are never part of a trusted view.
        </p>
      </div>
      <section className="profile-grid" aria-label="Sharing profiles">
        <article className="panel share-profile">
          <div className="avatar">HH</div>
          <div className="profile-copy">
            <span className="active-pill">Active</span>
            <h2>Household</h2>
            <p>Predicted sleep window, predicted waking window, confidence</p>
          </div>
        </article>
        <article className="panel share-profile">
          <div className="avatar soft">WC</div>
          <div className="profile-copy">
            <span className="active-pill">Active</span>
            <h2>Work coordinator</h2>
            <p>Availability windows only</p>
          </div>
        </article>
        <article className="panel share-profile muted">
          <div className="avatar neutral">EC</div>
          <div className="profile-copy">
            <span className="inactive-pill">Paused</span>
            <h2>Emergency contact</h2>
            <p>No fields currently visible</p>
          </div>
        </article>
      </section>
    </>
  );
}

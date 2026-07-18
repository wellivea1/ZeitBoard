import { Icon } from "../components/Icon";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";

export function MedicationsScreen() {
  return (
    <>
      <PageHeader
        title="Medications"
        description="Keep a private record tied to your own wake or sleep events."
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode="fixture" aria-hidden="true" />
            <span>Sample preview</span>
          </div>
        }
      />
      <PlaceholderNotice>
        These records are a synthetic design preview — medication logging is a planned feature. It
        will record clock-time doses you enter; it will never recommend one.
      </PlaceholderNotice>
      <div className="safety-banner">
        <Icon name="shield" />
        <p>
          <strong>Logging only</strong>This workspace records user-entered information. It does not
          recommend a medication, dose, or timing.
        </p>
      </div>
      <section className="panel record-list" aria-label="Synthetic medication records">
        <article>
          <div className="record-icon">
            <Icon name="clock" />
          </div>
          <div>
            <h2>Morning record</h2>
            <p>Synthetic label - relative to waking</p>
          </div>
          <span className="record-time">Within 30 min after wake</span>
        </article>
        <article>
          <div className="record-icon">
            <Icon name="moon" />
          </div>
          <div>
            <h2>Evening record</h2>
            <p>Synthetic label - manual reminder</p>
          </div>
          <span className="record-time">No active reminder</span>
        </article>
      </section>
    </>
  );
}

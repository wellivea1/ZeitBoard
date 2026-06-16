import { useEffect, useState } from "react";
import { Icon, type IconName } from "../components/Icon";
import { PageHeader } from "../components/AppShell";
import { loadOverview } from "../data/backend";
import { overviewFixture } from "../data/fixture";
import { proposalFixtures, refusalFixture, sourceConflictFixtures } from "../data/phaseTwo";
import type { ConfidenceLevel, OverviewSource } from "../data/overview";

interface MetricCardProps {
  icon: IconName;
  label: string;
  value: string;
  detail?: string;
  accent?: boolean;
}

function MetricCard({ icon, label, value, detail, accent }: MetricCardProps) {
  return (
    <article className="metric-card" data-accent={accent || undefined}>
      <div className="metric-icon">
        <Icon name={icon} />
      </div>
      <div>
        <p>{label}</p>
        <strong>{value}</strong>
        {detail && <small>{detail}</small>}
      </div>
    </article>
  );
}

function ConfidenceBadge({ value }: { value: ConfidenceLevel }) {
  return <span className={`confidence-badge confidence-${value.toLowerCase()}`}>{value}</span>;
}

const CONFIDENCE_SEGMENTS: Record<ConfidenceLevel, number> = { Low: 1, Medium: 2, High: 3 };

function ConfidenceMeter({ value }: { value: ConfidenceLevel }) {
  const filled = CONFIDENCE_SEGMENTS[value];
  return (
    <div className="confidence-meter" data-level={value.toLowerCase()} aria-hidden="true">
      {[0, 1, 2].map((index) => (
        <span key={index} data-muted={index >= filled || undefined} />
      ))}
    </div>
  );
}

type StateTone = "awake" | "asleep" | "uncertain";

function stateTone(state: string): StateTone {
  const normalized = state.toLowerCase();
  if (normalized.includes("uncertain") || normalized.includes("transition")) return "uncertain";
  if (normalized.includes("asleep") || normalized.includes("sleep")) return "asleep";
  return "awake";
}

export function OverviewScreen() {
  const [overview, setOverview] = useState(overviewFixture);
  const [mode, setMode] = useState<OverviewSource>("fixture");

  useEffect(() => {
    let current = true;
    void loadOverview().then((result) => {
      if (current) {
        setOverview(result.data);
        setMode(result.source);
      }
    });
    return () => {
      current = false;
    };
  }, []);

  return (
    <>
      <PageHeader
        eyebrow="Monday, June 15"
        title="Overview"
        description="A practical view of your estimated sleep-wake phase and the time ahead."
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode={mode} aria-hidden="true" />
            <span>{overview.fixtureMode ? "Sample data" : "Local service"}</span>
            <small>{overview.updatedLabel}</small>
          </div>
        }
      />

      <section className="phase-panel" aria-labelledby="phase-title">
        <div className="phase-copy">
          <p className="section-kicker">Estimated sleep-wake phase</p>
          <h2 id="phase-title" aria-live="polite">
            <span
              className="phase-state-dot"
              data-state={stateTone(overview.state)}
              aria-hidden="true"
            />
            {overview.state}
          </h2>
          <p>
            You have been awake for <strong>{overview.timeSinceWake}</strong>. This is an estimate
            from recent sleep-wake observations, not an exact circadian phase measurement.
          </p>
          <div className="phase-meta">
            <span>
              <Icon name="trend" />
              Drift <strong>{overview.drift.label}</strong>
            </span>
            <span>
              Confidence <ConfidenceBadge value={overview.confidence.level} />
            </span>
          </div>
        </div>
      </section>

      <section className="metric-grid" aria-label="Current planning summary">
        <MetricCard
          icon="moon"
          label="Predicted sleep window"
          value={overview.nextSleepWindow.label}
          detail={overview.nextSleepWindow.uncertainty}
        />
        <MetricCard
          icon="focus"
          label="Useful task window"
          value={overview.usefulTaskWindow.label}
          detail={overview.usefulTaskWindow.detail}
          accent
        />
        <MetricCard
          icon="sharing"
          label="Sharing"
          value={overview.sharingStatus.label}
          detail={overview.sharingStatus.detail}
        />
      </section>

      <section className="panel trust-strip" aria-labelledby="trust-strip-title">
        <div>
          <p className="section-kicker">Trust loop</p>
          <h2 id="trust-strip-title">Review before anything changes</h2>
          <p>
            {proposalFixtures.length} proposals are pending approval and{" "}
            {sourceConflictFixtures.length} source issues need review. {refusalFixture.message}
          </p>
        </div>
        <div className="trust-actions">
          <a className="button secondary" href="#/approvals">
            Review proposals
          </a>
          <a className="button secondary" href="#/rhythm">
            Review rhythm
          </a>
        </div>
      </section>

      <div className="overview-columns">
        <section className="panel schedule-panel" aria-labelledby="today-title">
          <div className="panel-heading">
            <div>
              <p className="section-kicker">Flexible plan</p>
              <h2 id="today-title">Today</h2>
            </div>
            <a href="#/calendar">
              Open calendar <Icon name="chevron" />
            </a>
          </div>
          <div className="schedule-list">
            <article className="schedule-item is-current">
              <time>Now</time>
              <span className="schedule-marker" />
              <div>
                <strong>Focused work</strong>
                <p>Good fit for 60-90 minute tasks</p>
              </div>
              <span className="task-chip">Flexible</span>
            </article>
            <article className="schedule-item">
              <time>6:30 PM</time>
              <span className="schedule-marker" />
              <div>
                <strong>Project check-in</strong>
                <p>Fixed calendar event</p>
              </div>
              <span className="task-chip fixed">Fixed</span>
            </article>
            <article className="schedule-item">
              <time>8:30 PM</time>
              <span className="schedule-marker" />
              <div>
                <strong>Lower-demand tasks</strong>
                <p>Suggested transition in task intensity</p>
              </div>
              <span className="task-chip">Flexible</span>
            </article>
          </div>
        </section>

        <aside className="panel confidence-panel" aria-labelledby="confidence-title">
          <div className="panel-heading">
            <div>
              <p className="section-kicker">Estimate quality</p>
              <h2 id="confidence-title">Confidence</h2>
            </div>
            <ConfidenceBadge value={overview.confidence.level} />
          </div>
          <ConfidenceMeter value={overview.confidence.level} />
          <p>{overview.confidence.reason}</p>
          <a href="#/rhythm">
            Review observations <Icon name="chevron" />
          </a>
        </aside>
      </div>
    </>
  );
}

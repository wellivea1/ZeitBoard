import { useEffect, useState } from "react";
import { Icon, type IconName } from "../components/Icon";
import { PageHeader } from "../components/AppShell";
import { loadOverview } from "../data/backend";
import { overviewFixture } from "../data/fixture";
import { useApprovals } from "../state/approvals";
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
  if (
    normalized.includes("uncertain") ||
    normalized.includes("transition") ||
    normalized.includes("no sleep") ||
    normalized.includes("need more") ||
    normalized.includes("unavailable")
  )
    return "uncertain";
  if (normalized.includes("asleep") || normalized.includes("sleep")) return "asleep";
  return "awake";
}

export function OverviewScreen() {
  const [overview, setOverview] = useState(overviewFixture);
  const [mode, setMode] = useState<OverviewSource>("fixture");
  const { pendingCount } = useApprovals();

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

  const hasEstimate = overview.status === "estimated";
  const todayLabel = new Date().toLocaleDateString(undefined, {
    weekday: "long",
    month: "long",
    day: "numeric",
  });

  return (
    <>
      <PageHeader
        eyebrow={todayLabel}
        title="Overview"
        description="A practical view of your estimated sleep-wake phase and the time ahead."
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode={mode} aria-hidden="true" />
            <span>
              {overview.fixtureMode
                ? "Sample data"
                : hasEstimate
                  ? "Local estimate"
                  : "Local data"}
            </span>
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
          {hasEstimate ? (
            <p>
              You have been awake for <strong>{overview.timeSinceWake}</strong>. This is an
              estimate from recent sleep-wake observations, not an exact circadian phase
              measurement.
            </p>
          ) : (
            <p>
              {overview.refusal?.message ?? overview.confidence.reason} Sleep and wake times stay
              local on this device.
            </p>
          )}
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
          <h2 id="trust-strip-title">
            {hasEstimate ? "Review before anything changes" : "Start with a manual sleep log"}
          </h2>
          <p>
            {hasEstimate
              ? `${pendingCount} pending ${
                  pendingCount === 1 ? "proposal" : "proposals"
                } waiting for explicit approval. Estimates stay uncertain and proposal-only.`
              : "Add principal sleep episodes in Data Sources. The app will refuse to estimate until there are enough usable entries."}
          </p>
        </div>
        <div className="trust-actions">
          {hasEstimate && (
            <a className="button secondary" href="#/approvals">
              Review proposals
            </a>
          )}
          <a className="button secondary" href="#/rhythm">
            Review rhythm
          </a>
          {!hasEstimate && (
            <a className="button primary" href="#/data-sources">
              Add sleep entry
            </a>
          )}
        </div>
      </section>

      <div className="overview-columns">
        <section className="panel schedule-panel" aria-labelledby="today-title">
          <div className="panel-heading">
            <div>
              <p className="section-kicker">{hasEstimate ? "Flexible plan" : "Local input"}</p>
              <h2 id="today-title">{hasEstimate ? "Current planning window" : "Manual sleep log"}</h2>
            </div>
            <a href={hasEstimate ? "#/approvals" : "#/data-sources"}>
              {hasEstimate ? "Open approvals" : "Open data entry"} <Icon name="chevron" />
            </a>
          </div>
          <p className="phase-two-copy">
            {hasEstimate
              ? `${overview.usefulTaskWindow.label}. ${overview.usefulTaskWindow.detail}`
              : "No rhythm estimate is shown until local entries meet the estimator minimum. Add civil sleep and wake times first."}
          </p>
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

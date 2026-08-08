import { useEffect, useState } from "react";
import { Icon, type IconName } from "../components/Icon";
import { PageHeader } from "../components/AppShell";
import { OutlookPanel } from "../components/OutlookPanel";
import { CycleStrip } from "../components/RhythmVisuals";
import { loadOverview } from "../data/backend";
import { loadOutlook } from "../data/outlook";
import { outlookFixture, overviewFixture } from "../data/fixture";
import { loadRhythm, rhythmFixture, type RhythmSource } from "../data/rhythm";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import { usePendingApprovalsCount } from "../state/approvals";
import type { ConfidenceLevel, OverviewSource } from "../data/overview";
import { createCoalescedRefresh } from "../utils/coalescedRefresh";

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

interface OverviewFactProps {
  icon: IconName;
  label: string;
  value: string;
  detail: string;
  tone: "sleep" | "awake" | "neutral";
}

function OverviewFact({ icon, label, value, detail, tone }: OverviewFactProps) {
  return (
    <div className="overview-fact" data-tone={tone}>
      <dt>
        <Icon name={icon} />
        {label}
      </dt>
      <dd>
        <strong>{value}</strong>
        <small>{detail}</small>
      </dd>
    </div>
  );
}

function sourceLabel(source: OverviewSource, hasEstimate: boolean) {
  if (source === "synced")
    return hasEstimate ? "Synced - server estimate" : "Synced - awaiting estimate";
  if (source === "local") return hasEstimate ? "Local estimate" : "Local data";
  return "Sample data";
}

export function OverviewScreen() {
  const [overview, setOverview] = useState(overviewFixture);
  const [mode, setMode] = useState<OverviewSource>("fixture");
  const [rhythm, setRhythm] = useState(rhythmFixture);
  const [rhythmMode, setRhythmMode] = useState<RhythmSource>("fixture");
  const [outlook, setOutlook] = useState(outlookFixture);
  const pendingCount = usePendingApprovalsCount();

  useEffect(() => {
    const refresh = createCoalescedRefresh(
      () =>
        Promise.all([
          loadOverview(),
          loadRhythm(),
          loadOutlook(globalThis as never, outlookFixture),
        ]),
      ([overviewResult, rhythmResult, outlookResult]) => {
        setOverview(overviewResult.data);
        setMode(overviewResult.source);
        setRhythm(rhythmResult.data);
        setRhythmMode(rhythmResult.source);
        setOutlook(outlookResult);
      },
    );
    const request = () => refresh.request();
    request();
    window.addEventListener(sleepDataChangedEvent, request);
    return () => {
      window.removeEventListener(sleepDataChangedEvent, request);
      refresh.dispose();
    };
  }, []);

  const hasEstimate = overview.status === "estimated";
  const hasMatchingRhythm = hasEstimate && rhythm.status === "estimated" && mode === rhythmMode;
  const todayLabel =
    mode === "fixture" && hasMatchingRhythm
      ? `Sample date · ${rhythm.actogram.now.day}`
      : new Date().toLocaleDateString(undefined, {
          weekday: "long",
          month: "long",
          day: "numeric",
        });

  return (
    <>
      <PageHeader
        eyebrow={todayLabel}
        title="Overview"
        description="What is true now, what is uncertain, and the next useful window."
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode={mode} aria-hidden="true" />
            <span>{sourceLabel(mode, hasEstimate)}</span>
            <small>{overview.updatedLabel}</small>
          </div>
        }
      />

      <section className="overview-surface" aria-labelledby="phase-title">
        <header className="overview-status" data-state={stateTone(overview.state)}>
          <div className="overview-status-primary">
            <p className="section-kicker">Estimated sleep-wake timing</p>
            <h2 id="phase-title" aria-live="polite">
              <span
                className="phase-state-dot"
                data-state={stateTone(overview.state)}
                aria-hidden="true"
              />
              {overview.state}
            </h2>
            {hasEstimate && (
              <p className="overview-elapsed">
                <strong>{overview.timeSinceWake}</strong> since wake
              </p>
            )}
            <p className="overview-freshness" data-state={overview.freshness.state}>
              {overview.freshness.ageLabel && <span>{overview.freshness.ageLabel}</span>}
              <span>{overview.freshness.explanation}</span>
            </p>
          </div>
          <div className="overview-status-context">
            <strong>{overview.stateDetail}</strong>
            <p>
              {!hasEstimate
                ? (overview.refusal?.message ?? overview.confidence.reason)
                : overview.freshness.trusted
                  ? "Estimated from recent sleep-wake observations, not an exact circadian phase measurement."
                  : "The current state is not being claimed, because the records behind it are not recent enough to support one."}
            </p>
          </div>
        </header>

        {hasEstimate ? (
          <>
            {hasMatchingRhythm ? (
              <CycleStrip
                actogram={rhythm.actogram}
                usefulWindowLabel={overview.usefulTaskWindow.label}
                sleepWindowLabel={overview.nextSleepWindow.label}
              />
            ) : (
              <div className="cycle-strip-unavailable">
                <strong>Cycle view is unavailable</strong>
                <span>Overview and Rhythm must come from the same current estimate.</span>
                <a href="#/rhythm">Review rhythm details</a>
              </div>
            )}

            <dl className="overview-facts" aria-label="Current planning facts">
              <OverviewFact
                icon="moon"
                label="Predicted sleep window"
                value={overview.nextSleepWindow.label}
                detail={overview.nextSleepWindow.uncertainty}
                tone="sleep"
              />
              <OverviewFact
                icon="trend"
                label="Recent drift"
                value={overview.drift.label}
                detail={overview.drift.direction}
                tone="neutral"
              />
            </dl>

            <OutlookPanel data={outlook} />

            <section className="overview-quality" aria-labelledby="evidence-title">
              <div>
                <span className="overview-row-label">Estimate quality</span>
                <h3 id="evidence-title">Evidence</h3>
              </div>
              <p className="overview-evidence-line" data-state={overview.freshness.state}>
                {overview.freshness.ageLabel || "No records yet"}
              </p>
              <p>{overview.freshness.explanation}</p>
              <details className="overview-model-detail">
                <summary>Model confidence</summary>
                <div className="overview-confidence-value">
                  <ConfidenceMeter value={overview.confidence.level} />
                  <ConfidenceBadge value={overview.confidence.level} />
                </div>
                <p>{overview.confidence.reason}</p>
                <p className="overview-calibration-note">
                  This label describes how well the model fits recent records. Measured against real
                  history it did not rank reliably &mdash; episodes marked High were not more
                  accurate than those marked Medium &mdash; so use the predicted range and the
                  evidence age above to decide, not this label.
                </p>
              </details>
              <a href="#/rhythm">Why this estimate?</a>
            </section>

            {overview.sharingStatus.active && (
              <aside className="overview-attention" aria-label="Active sharing notice">
                <Icon name="sharing" />
                <span>
                  <strong>{overview.sharingStatus.label}</strong>
                  <small>{overview.sharingStatus.detail}</small>
                </span>
                <a href="#/sharing">Review sharing</a>
              </aside>
            )}
          </>
        ) : (
          <section className="overview-learning" aria-labelledby="learning-title">
            <div>
              <p className="section-kicker">Local input needed</p>
              <h3 id="learning-title">Still learning your rhythm</h3>
              <p>
                {overview.refusal?.message ?? overview.confidence.reason} Add civil sleep and wake
                times before the app draws a forecast.
              </p>
            </div>
            <a className="button primary" href="#/data-sources">
              Add sleep entry
            </a>
          </section>
        )}

        <footer className="overview-trust-row">
          <div>
            <span className="overview-row-label">Trust loop</span>
            <strong>
              {hasEstimate ? "Review before anything changes" : "Your observations stay local"}
            </strong>
            <small>
              {hasEstimate
                ? `${pendingCount} pending ${pendingCount === 1 ? "proposal" : "proposals"}; every change needs explicit approval.`
                : "Suppression preserves history; permanent deletion is a separate confirmed action."}
            </small>
          </div>
          <div>
            {hasEstimate && (
              <a className="button secondary" href="#/approvals">
                Review proposals
              </a>
            )}
            <a className="button secondary" href="#/rhythm">
              Review rhythm
            </a>
          </div>
        </footer>
      </section>
    </>
  );
}

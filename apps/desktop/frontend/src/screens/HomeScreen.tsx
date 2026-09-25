import { useEffect, useState } from "react";
import { PageHeader } from "../components/AppShell";
import { Icon } from "../components/Icon";
import { OutlookPanel } from "../components/OutlookPanel";
import { QuickLogBar } from "../components/QuickLogBar";
import { loadOverview } from "../data/backend";
import { isCommitmentConflict, loadOutlook, outlookUnavailable, sleepAhead } from "../data/outlook";
import { outlookFixture, overviewFixture } from "../data/fixture";
import { sleepDataChangedEvent, notifySleepDataChanged } from "../data/sleepDataEvents";
import { hasDesktopBridge } from "../data/wailsBridge";
import { overviewUnavailable } from "../data/overview";
import { subscribeProjectionRefresh } from "../utils/projectionRefresh";
import { useApprovalQueue } from "../state/approvalQueue";
import type { ConfidenceLevel, OverviewSource, OverviewData } from "../data/overview";
import type { OutlookCommitment, OutlookData, OutlookSegment } from "../data/outlook";
import { createCoalescedRefresh } from "../utils/coalescedRefresh";
import { atOffset, relativeRange, roundToMinutes } from "../utils/relativeTime";

// Home answers three questions, in this order: am I in a waking or a sleeping
// stretch, when is the next sleep likely, and does anything need me. It used to
// answer them several times over — the predicted sleep window appeared three
// times, twice with different end times — across two timelines and nine
// labelled sections, at 2.4 screens tall. It is one screen now.

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

function sourceLabel(source: OverviewSource, hasEstimate: boolean) {
  if (source === "synced") return hasEstimate ? "Synced estimate" : "Synced, awaiting estimate";
  if (source === "local") return hasEstimate ? "Local estimate" : "Local data";
  return "Sample data";
}

function useHomeProjection() {
  const desktop = hasDesktopBridge();
  const [overview, setOverview] = useState(desktop ? overviewUnavailable : overviewFixture);
  const [mode, setMode] = useState<OverviewSource>(desktop ? "local" : "fixture");
  const [outlook, setOutlook] = useState(desktop ? outlookUnavailable : outlookFixture);
  const [loading, setLoading] = useState(desktop);

  useEffect(() => {
    const refresh = createCoalescedRefresh(
      () => Promise.all([loadOverview(), loadOutlook(globalThis as never, outlookFixture)]),
      ([overviewResult, outlookResult]) => {
        setOverview(overviewResult.data);
        setMode(overviewResult.source);
        setOutlook(outlookResult);
        setLoading(false);
      },
    );
    const request = () => refresh.request();
    const unsubscribe = subscribeProjectionRefresh(request, sleepDataChangedEvent);
    return () => {
      unsubscribe();
      refresh.dispose();
    };
  }, []);

  return { overview, mode, outlook, loading };
}

function compactRange(label: string) {
  return label.replace(" to ", " – ");
}

/**
 * "Tonight 11:30 PM – 1:35 AM" from the band's own position on the timeline.
 * Rounded to five minutes: the band itself is hours wide, and minute-precise
 * edges would suggest a precision the forecast does not have.
 */
function bandWording(segment: OutlookSegment, horizonStart: string | undefined, now: Date) {
  const start = atOffset(horizonStart, segment.offsetHours);
  const end = atOffset(horizonStart, segment.offsetHours + segment.durationHours);
  if (!start || !end) return compactRange(segment.rangeLabel);
  return relativeRange(roundToMinutes(start, 5), roundToMinutes(end, 5), now);
}

function NextSleep({ overview, outlook }: { overview: OverviewData; outlook: OutlookData }) {
  const ahead = outlook.status === "available" ? sleepAhead(outlook.segments) : {};
  const now = new Date();
  if (!ahead.onset && !ahead.wake) {
    return (
      <div className="home-next">
        <p className="home-next-label">
          <Icon name="moon" /> Next sleep
        </p>
        <strong>{overview.nextSleepWindow.label}</strong>
        <small>{overview.nextSleepWindow.uncertainty}</small>
      </div>
    );
  }
  // Inside a predicted sleep there is no onset ahead, only its end. Calling
  // that "Next sleep" read as a contradiction at 3 AM.
  if (!ahead.onset && ahead.wake) {
    return (
      <div className="home-next">
        <p className="home-next-label">
          <Icon name="moon" /> Likely waking
        </p>
        <strong>{bandWording(ahead.wake, outlook.horizonStart, now)}</strong>
        <small>the end of the sleep the forecast expects now</small>
      </div>
    );
  }
  return (
    <div className="home-next">
      <p className="home-next-label">
        <Icon name="moon" /> Next sleep
      </p>
      {ahead.onset && (
        <>
          <strong>{bandWording(ahead.onset, outlook.horizonStart, now)}</strong>
          <small>likely to begin in this window</small>
        </>
      )}
      {ahead.wake && (
        <p className="home-next-wake">
          Waking <span>{bandWording(ahead.wake, outlook.horizonStart, now)}</span>
        </p>
      )}
    </div>
  );
}

// The contract's wording is complete; a footer only needs the number.
function compactDrift(label: string) {
  return label.replace(/ minutes per observed sleep cycle$/, " min per cycle");
}

function plural(count: number, one: string, many: string) {
  return `${count} ${count === 1 ? one : many}`;
}

function NeedsYou({ overview }: { overview: OverviewData }) {
  const { breakdown, ready, incomplete } = useApprovalQueue();
  const items: { key: string; label: string; href: string }[] = [];
  if (breakdown.suggestions > 0)
    items.push({
      key: "suggestions",
      label: plural(breakdown.suggestions, "suggested time to review", "suggested times to review"),
      href: "#/plan/tasks",
    });
  if (breakdown.conflicts > 0)
    items.push({
      key: "conflicts",
      label: plural(
        breakdown.conflicts,
        "task edited on two devices",
        "tasks edited on two devices",
      ),
      href: "#/plan/tasks",
    });
  if (breakdown.assistant > 0)
    items.push({
      key: "assistant",
      label: plural(
        breakdown.assistant,
        "proposal from your assistant",
        "proposals from your assistant",
      ),
      href: "#/plan/tasks",
    });
  if (breakdown.requests > 0)
    items.push({
      key: "requests",
      label: plural(breakdown.requests, "time request", "time requests"),
      href: "#/plan/tasks",
    });

  return (
    <section className="home-panel" aria-labelledby="needs-title">
      <h3 id="needs-title">Needs you</h3>
      <ul className="home-list">
        {items.map((item) => (
          <li key={item.key}>
            <a href={item.href}>{item.label}</a>
          </li>
        ))}
        {!overview.freshness.trusted && (
          <li data-tone="warn">
            <a href="#/log/sleep">Log recent sleep — the forecast needs a newer record</a>
          </li>
        )}
      </ul>
      {items.length === 0 && overview.freshness.trusted && (
        <p className="home-empty">
          {!ready
            ? "Checking…"
            : incomplete
              ? "Some sources could not be checked."
              : "Nothing right now."}
        </p>
      )}
    </section>
  );
}

function commitmentWording(commitment: OutlookCommitment, horizonStart: string | undefined) {
  if (commitment.offsetHours === undefined || commitment.durationHours === undefined) {
    return compactRange(commitment.whenLabel);
  }
  const start = atOffset(horizonStart, commitment.offsetHours);
  const end = atOffset(horizonStart, commitment.offsetHours + commitment.durationHours);
  return start && end ? relativeRange(start, end, new Date()) : compactRange(commitment.whenLabel);
}

function ComingUp({ outlook }: { outlook: OutlookData }) {
  const commitments = outlook.status === "available" ? outlook.commitments.slice(0, 4) : [];
  return (
    <section className="home-panel" aria-labelledby="coming-title">
      <h3 id="coming-title">Coming up</h3>
      {commitments.length > 0 ? (
        <ul className="home-list">
          {commitments.map((commitment) => (
            <li
              key={`${commitment.title}-${commitment.whenLabel}`}
              data-tone={isCommitmentConflict(commitment) ? "warn" : undefined}
            >
              <span>
                <strong>{commitment.title}</strong>
                <small>{commitmentWording(commitment, outlook.horizonStart)}</small>
              </span>
              {commitment.conflictLabel && <em>{commitment.conflictLabel}</em>}
            </li>
          ))}
        </ul>
      ) : (
        <p className="home-empty">No fixed events in the next three days.</p>
      )}
    </section>
  );
}

function ModelConfidence({ level, reason }: { level: ConfidenceLevel; reason: string }) {
  return (
    <details className="home-confidence">
      <summary>Model confidence</summary>
      <p>
        <strong>{level}.</strong> {reason}
      </p>
      <p>
        This describes how well the model fits recent records. Measured against real history it did
        not rank reliably — episodes marked High were not more accurate than those marked Medium —
        so decide by the predicted range and the age of your records, not this label.
      </p>
    </details>
  );
}

export function HomeScreen() {
  const { overview, mode, outlook, loading } = useHomeProjection();
  const hasEstimate = overview.status === "estimated";
  const tone = stateTone(overview.state);
  const todayLabel = new Date().toLocaleDateString(undefined, {
    weekday: "long",
    month: "long",
    day: "numeric",
  });

  return (
    <>
      <PageHeader
        eyebrow={todayLabel}
        title="Home"
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode={mode} aria-hidden="true" />
            <span>{sourceLabel(mode, hasEstimate)}</span>
          </div>
        }
      />

      <section className="home" aria-labelledby="phase-title">
        <header className="home-now" data-state={tone}>
          <div className="home-now-state">
            <h2 id="phase-title" aria-live="polite">
              <span className="phase-state-dot" data-state={tone} aria-hidden="true" />
              {loading ? "Loading your rhythm…" : overview.state}
            </h2>
            {hasEstimate && (
              <p className="home-elapsed">
                <strong>{overview.timeSinceWake}</strong> since you woke
              </p>
            )}
            <p className="home-evidence" data-state={overview.freshness.state}>
              {overview.freshness.trusted
                ? overview.freshness.ageLabel
                : overview.freshness.explanation}
            </p>
            <QuickLogBar />
          </div>
          {hasEstimate && <NextSleep overview={overview} outlook={outlook} />}
        </header>

        {hasEstimate ? (
          <>
            <OutlookPanel data={outlook} />
            <div className="home-lower">
              <NeedsYou overview={overview} />
              <ComingUp outlook={outlook} />
            </div>
            <footer className="home-footer">
              <span>
                Drift <strong>{compactDrift(overview.drift.label)}</strong>
              </span>
              <ModelConfidence
                level={overview.confidence.level}
                reason={overview.confidence.reason}
              />
              <a href="#/rhythm">Why this estimate?</a>
              {overview.sharingStatus.active && (
                <a href="#/sharing" className="home-sharing">
                  <Icon name="sharing" />
                  {overview.sharingStatus.label}
                </a>
              )}
              <small>
                Estimated from your sleep records, not a measurement of circadian phase.
              </small>
            </footer>
          </>
        ) : (
          <HomeRecovery overview={overview} />
        )}
      </section>
    </>
  );
}

function HomeRecovery({ overview }: { overview: OverviewData }) {
  const unavailable = overview.status === "unavailable";
  return (
    <section className="home-recovery" aria-labelledby="learning-title">
      <div>
        <h3 id="learning-title">
          {unavailable ? "Your rhythm is not available yet" : "Still learning your rhythm"}
        </h3>
        <p>
          {unavailable
            ? "Your saved records have not been changed. This retries by itself, or try now."
            : `${overview.refusal?.message ?? overview.confidence.reason} Log sleep or import existing records to build a forecast.`}
        </p>
      </div>
      <div className="page-actions">
        {/* Recovery sits with the failure; the view already refreshes on
            focus, on new records and every minute. */}
        {unavailable && (
          <button className="button primary" type="button" onClick={notifySleepDataChanged}>
            Try again
          </button>
        )}
        {!unavailable && (
          <a className="button primary" href="#/log/sleep">
            Log sleep
          </a>
        )}
        <a className="button secondary" href="#/data-sources">
          {unavailable ? "Check Data Sources" : "Import sleep records"}
        </a>
      </div>
    </section>
  );
}

import { useEffect, useState } from "react";
import { Icon } from "../components/Icon";
import { PageHeader } from "../components/AppShell";
import { ScreenTabPanel, ScreenTabs, type ScreenTab } from "../components/ScreenTabs";
import { useRhythmMarkers } from "../state/rhythmMarkers";
import type { RhythmTab } from "../types";
import { ActogramPanel, DriftPanel } from "../components/RhythmVisuals";
import {
  correctionPreviewFixture,
  refusalFixture,
  sourceConflictFixtures,
  type SourceConflictFixture,
} from "../data/phaseTwo";
import { loadRhythm, rhythmFixture, type RhythmData, type RhythmSource } from "../data/rhythm";
import {
  latestCorrectedEntry,
  loadSleepEntries,
  summarizeSleepSources,
  type SleepEntriesData,
} from "../data/sleepEntries";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import { createCoalescedRefresh } from "../utils/coalescedRefresh";

function SourceConflictList({
  conflicts,
  labelledBy,
}: {
  conflicts: SourceConflictFixture[];
  labelledBy: string;
}) {
  return (
    <div className="conflict-list" aria-labelledby={labelledBy}>
      {conflicts.map((conflict) => (
        <article className="conflict-row" data-state={conflict.state} key={conflict.id}>
          <div>
            <span className="state-pill">{conflict.state}</span>
            <h3>{conflict.title}</h3>
            <p>{conflict.detail}</p>
          </div>
          <small>
            {conflict.source} - {conflict.nextAction}
          </small>
        </article>
      ))}
    </div>
  );
}

function CorrectionInspector() {
  return (
    <aside className="panel correction-inspector" aria-labelledby="correction-title">
      <div className="panel-heading">
        <div>
          <p className="section-kicker">Corrections</p>
          <h2 id="correction-title">{correctionPreviewFixture.title}</h2>
        </div>
      </div>
      <dl className="correction-diff">
        <div>
          <dt>Source interval</dt>
          <dd>{correctionPreviewFixture.sourceInterval}</dd>
        </div>
        <div>
          <dt>Effective interval</dt>
          <dd>{correctionPreviewFixture.effectiveInterval}</dd>
        </div>
      </dl>
      <p className="diff-note">{correctionPreviewFixture.diffLabel}</p>
      <small>{correctionPreviewFixture.historyLabel}</small>
    </aside>
  );
}

// Context markers moved to Log in slice U-H. Recording that you travelled or
// were ill is logging; this screen is for reading what the records imply.
const rhythmTabs: ScreenTab<RhythmTab>[] = [
  { id: "actogram", label: "Actogram" },
  { id: "drift", label: "Drift" },
  { id: "sources", label: "Sources" },
];

// Real evidence review for the Rhythm Sources tab (roadmap slice 5): the
// estimator's actual refusal, the actual correction history, and the actual
// per-source composition of the local log — no synthetic conflicts.
function LocalSourcesPanel({ rhythm }: { rhythm: RhythmData }) {
  const [entriesData, setEntriesData] = useState<SleepEntriesData>({
    status: "empty",
    empty: true,
    message: "Loading local sleep entries.",
    entries: [],
  });

  useEffect(() => {
    const refresh = createCoalescedRefresh(loadSleepEntries, setEntriesData);
    const request = () => refresh.request();
    request();
    window.addEventListener(sleepDataChangedEvent, request);
    return () => {
      window.removeEventListener(sleepDataChangedEvent, request);
      refresh.dispose();
    };
  }, []);

  const entries = entriesData.entries;
  const sources = summarizeSleepSources(entries);
  const corrected = latestCorrectedEntry(entries);
  const correctedCount = entries.filter((entry) => entry.history.length > 0).length;
  const suppressedCount = entries.filter((entry) => entry.suppressed).length;
  const latestChange = corrected?.history[corrected.history.length - 1];

  return (
    <>
      {rhythm.status !== "estimated" && rhythm.refusal && (
        <section className="panel refusal-panel" aria-labelledby="refusal-title">
          <p className="section-kicker">{rhythm.refusal.code}</p>
          <h2 id="refusal-title">The estimator is refusing, not guessing</h2>
          <p>{rhythm.refusal.message}</p>
          <div className="proposal-reasons" aria-label="Refusal actions">
            <a className="task-chip" href="#/log/sleep">
              Add sleep entries
            </a>
          </div>
        </section>
      )}

      <aside className="panel correction-inspector" aria-labelledby="correction-title">
        <div className="panel-heading">
          <div>
            <p className="section-kicker">Corrections</p>
            <h2 id="correction-title">
              {correctedCount > 0
                ? `${correctedCount} corrected ${correctedCount === 1 ? "entry" : "entries"}`
                : "No corrections yet"}
            </h2>
          </div>
          <a href="#/log/sleep">
            Edit in Data Sources <Icon name="chevron" />
          </a>
        </div>
        {corrected && latestChange ? (
          <>
            <dl className="correction-diff">
              <div>
                <dt>Source interval</dt>
                <dd>
                  {corrected.startLabel} to {corrected.endLabel}
                </dd>
              </div>
              <div>
                <dt>Effective interval</dt>
                <dd>
                  {corrected.effectiveStartLabel} to {corrected.effectiveEndLabel}
                </dd>
              </div>
            </dl>
            <p className="diff-note">{latestChange.summary}</p>
            <small>
              {latestChange.createdLabel} - {latestChange.reason}. The original observation is
              preserved; corrections are append-only and reversible with another correction.
            </small>
          </>
        ) : (
          <p className="phase-two-copy">
            Observations are immutable. Editing an entry appends a correction here instead of
            overwriting it, so the evidence trail stays reviewable.
          </p>
        )}
      </aside>

      <section className="panel source-conflicts-panel" aria-labelledby="local-sources-title">
        <div className="panel-heading">
          <div>
            <p className="section-kicker">Sources</p>
            <h2 id="local-sources-title">What the estimator sees</h2>
          </div>
        </div>
        {sources.length > 0 ? (
          <div className="conflict-list" aria-labelledby="local-sources-title">
            {sources.map((source) => (
              <article className="conflict-row" data-state="resolved" key={source.source}>
                <div>
                  <span className="state-pill">{source.provenance}</span>
                  <h3>{source.source}</h3>
                  <p>
                    {source.total} {source.total === 1 ? "entry" : "entries"}
                    {source.corrected > 0 && `, ${source.corrected} corrected`}
                    {source.suppressed > 0 && `, ${source.suppressed} suppressed from estimates`}
                  </p>
                </div>
                <small>
                  {source.suppressed > 0
                    ? "Suppressed entries stay stored but are excluded from the estimate"
                    : "All entries feed the estimate"}
                </small>
              </article>
            ))}
          </div>
        ) : (
          <p className="phase-two-copy">
            No local sleep data yet. Overlaps between future sources are resolved inside the
            estimation engine, never silently in the chart.
          </p>
        )}
        {suppressedCount + correctedCount > 0 && (
          <p className="diff-note">
            Every change is auditable: {correctedCount} correction{correctedCount === 1 ? "" : "s"}
            {suppressedCount > 0 &&
              `, ${suppressedCount} suppression${suppressedCount === 1 ? "" : "s"}`}{" "}
            recorded without altering original observations.
          </p>
        )}
      </section>
    </>
  );
}

function RhythmUnavailablePanel({ rhythm }: { rhythm: RhythmData }) {
  return (
    <section className="panel empty-state rhythm-empty-state" aria-labelledby="rhythm-empty-title">
      <p className="section-kicker">{rhythm.refusal?.code ?? rhythm.status}</p>
      <h2 id="rhythm-empty-title">
        {rhythm.status === "empty" ? "Add sleep entries to draw rhythm" : "Need more usable data"}
      </h2>
      <p>
        {rhythm.message ??
          rhythm.refusal?.message ??
          "The local estimator has no chart to show yet."}
      </p>
      <a className="button primary" href="#/log/sleep">
        Add sleep entry
      </a>
    </section>
  );
}

export function RhythmScreen() {
  const [tab, setTab] = useState<RhythmTab>("actogram");
  const [rhythm, setRhythm] = useState(rhythmFixture);
  const [mode, setMode] = useState<RhythmSource>("fixture");
  // Markers are recorded in Log and *read* here: they are the context that
  // explains a jump in the actogram, so the chart would be misleading without
  // them even though nothing on this screen edits them.
  const markers = useRhythmMarkers();
  useEffect(() => {
    const refresh = createCoalescedRefresh(loadRhythm, (result) => {
      setRhythm(result.data);
      setMode(result.source);
    });
    const request = () => refresh.request();
    request();
    window.addEventListener(sleepDataChangedEvent, request);
    return () => {
      window.removeEventListener(sleepDataChangedEvent, request);
      refresh.dispose();
    };
  }, []);

  const hasRhythm = rhythm.status === "estimated";
  const sourceLabel =
    mode === "synced"
      ? hasRhythm
        ? "Synced - server estimate"
        : "Synced - awaiting estimate"
      : mode === "local"
        ? hasRhythm
          ? "Local estimate"
          : "Local data"
        : "Sample data";

  return (
    <>
      <PageHeader
        title="Rhythm"
        description="Inspect sleep-wake observations, correction history, and estimate uncertainty."
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode={mode} aria-hidden="true" />
            <span>{sourceLabel}</span>
          </div>
        }
      />
      <p className="screen-context">
        {mode === "synced" && hasRhythm
          ? "The actogram, drift fit, and forecast below are computed by the synced server estimate."
          : mode === "synced"
            ? "The synced server estimator is waiting for enough sleep data before drawing rhythm charts."
            : mode === "local" && hasRhythm
              ? "The actogram, drift fit, and forecast below are computed by the local estimation engine."
              : mode === "local"
                ? "The local estimator is waiting for enough manually entered sleep data before drawing rhythm charts."
                : "This read-only preview distinguishes imported, estimated, corrected, and incomplete observations."}
      </p>
      <section className="rhythm-screen" aria-label="Rhythm review">
        <ScreenTabs
          name="rhythm"
          label="Rhythm views"
          tabs={rhythmTabs}
          active={tab}
          onSelect={setTab}
        />

        <ScreenTabPanel name="rhythm" id="actogram" active={tab}>
          {hasRhythm ? (
            <ActogramPanel
              actogram={rhythm.actogram}
              markers={mode === "fixture" ? [] : markers.data.markers}
            />
          ) : (
            <RhythmUnavailablePanel rhythm={rhythm} />
          )}
        </ScreenTabPanel>

        <ScreenTabPanel name="rhythm" id="drift" active={tab}>
          {hasRhythm ? (
            <DriftPanel drift={rhythm.drift} />
          ) : (
            <RhythmUnavailablePanel rhythm={rhythm} />
          )}
        </ScreenTabPanel>

        <ScreenTabPanel name="rhythm" id="sources" active={tab}>
          <>
            {mode === "fixture" ? (
              <>
                <CorrectionInspector />

                <section className="panel refusal-panel" aria-labelledby="refusal-title">
                  <p className="section-kicker">{refusalFixture.code}</p>
                  <h2 id="refusal-title">{refusalFixture.title}</h2>
                  <p>{refusalFixture.message}</p>
                  <div className="proposal-reasons" aria-label="Refusal actions">
                    {refusalFixture.actions.map((action) => (
                      <span className="task-chip" key={action}>
                        {action}
                      </span>
                    ))}
                  </div>
                </section>

                <section
                  className="panel source-conflicts-panel"
                  aria-labelledby="source-conflicts-title"
                >
                  <div className="panel-heading">
                    <div>
                      <p className="section-kicker">Sources</p>
                      <h2 id="source-conflicts-title">Source conflicts and missingness</h2>
                    </div>
                  </div>
                  <SourceConflictList
                    conflicts={sourceConflictFixtures}
                    labelledBy="source-conflicts-title"
                  />
                </section>
              </>
            ) : (
              <LocalSourcesPanel rhythm={rhythm} />
            )}
          </>
        </ScreenTabPanel>
      </section>
    </>
  );
}

import type { ReactNode } from "react";
import { Notice } from "../components/Notice";
import { Icon } from "../components/Icon";
import { Loading } from "../components/Loading";
import { PageHeader } from "../components/AppShell";
import { ScreenTabPanel, ScreenTabs, type ScreenTab } from "../components/ScreenTabs";
import { useRhythmMarkers } from "../state/rhythmMarkers";
import { useLoaded } from "../state/useLoaded";
import type { RhythmTab } from "../types";
import { ActogramPanel, DriftPanel } from "../components/RhythmVisuals";
import { estimateSourceLabel, explainRefusal, inEstimatorWords } from "../data/estimateStatus";
import { loadRhythm, type RhythmData, type RhythmSource } from "../data/rhythm";
import {
  latestCorrectedEntry,
  loadSleepEntries,
  sleepEntriesUnavailable,
  summarizeSleepSources,
} from "../data/sleepEntries";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";

// Context markers moved to Log in slice U-H. Recording that you travelled or
// were ill is logging; this screen is for reading what the records imply.
const rhythmTabs: ScreenTab<RhythmTab>[] = [
  { id: "actogram", label: "Actogram" },
  { id: "drift", label: "Drift" },
  { id: "sources", label: "Sources" },
];

// The evidence behind the charts: the actual correction history and the
// per-source composition of the log, never a synthetic example.
function SourcesPanel() {
  const { data } = useLoaded(loadSleepEntries, {
    events: [sleepDataChangedEvent],
    fallback: sleepEntriesUnavailable,
  });
  if (!data) return <Loading />;

  const entries = data.entries;
  const sources = summarizeSleepSources(entries);
  const corrected = latestCorrectedEntry(entries);
  const correctedCount = entries.filter((entry) => entry.history.length > 0).length;
  const suppressedCount = entries.filter((entry) => entry.suppressed).length;
  const latestChange = corrected?.history[corrected.history.length - 1];

  return (
    <>
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
            Edit sleep log <Icon name="chevron" />
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
              {latestChange.createdLabel} · {latestChange.reason}.
            </small>
          </>
        ) : null}
        <Notice id="rhythm.corrections">
          A recorded night is never overwritten. Editing one adds a correction on top, and another
          correction undoes it, so every change can be traced.
        </Notice>
      </aside>

      <section className="panel source-conflicts-panel" aria-labelledby="local-sources-title">
        <div className="panel-heading">
          <div>
            <p className="section-kicker">Sources</p>
            <h2 id="local-sources-title">What the estimator sees</h2>
          </div>
        </div>
        {data.status === "unavailable" ? (
          <p className="rhythm-sources-empty">{data.message}</p>
        ) : sources.length > 0 ? (
          <div className="conflict-list" aria-labelledby="local-sources-title">
            {sources.map((source) => (
              <article className="conflict-row" key={source.source}>
                <div>
                  <p className="section-kicker">{source.provenance}</p>
                  <h3>{source.source}</h3>
                  <p>
                    {source.total} {source.total === 1 ? "entry" : "entries"}
                    {source.corrected > 0 && `, ${source.corrected} corrected`}
                    {source.suppressed > 0 && `, ${source.suppressed} excluded from estimates`}
                  </p>
                </div>
                <small>
                  {source.suppressed > 0
                    ? "Excluded entries stay stored but do not feed the estimate"
                    : "All entries feed the estimate"}
                </small>
              </article>
            ))}
          </div>
        ) : (
          <p className="rhythm-sources-empty">No sleep records yet.</p>
        )}
        {suppressedCount + correctedCount > 0 && (
          <p className="diff-note">
            Every change is kept: {correctedCount} correction{correctedCount === 1 ? "" : "s"}
            {suppressedCount > 0 &&
              `, ${suppressedCount} exclusion${suppressedCount === 1 ? "" : "s"}`}
            , each recorded without altering the original.
          </p>
        )}
      </section>
    </>
  );
}

// No chart: say why in plain words and offer the one step most likely to help.
function NoChartPanel({ rhythm }: { rhythm: RhythmData }) {
  if (rhythm.status === "unavailable") {
    return (
      <section
        className="panel empty-state rhythm-empty-state"
        aria-labelledby="rhythm-empty-title"
      >
        <h2 id="rhythm-empty-title">Rhythm unavailable</h2>
        <p>
          {rhythm.message ??
            "The desktop service could not load your rhythm. It retries by itself."}
        </p>
        <a className="button primary" href="#/data-sources">
          Check Data Sources
        </a>
      </section>
    );
  }
  const explanation = rhythm.refusal
    ? explainRefusal(rhythm.refusal)
    : explainRefusal({ code: "insufficient_data", message: "" });
  return (
    <section className="panel empty-state rhythm-empty-state" aria-labelledby="rhythm-empty-title">
      <h2 id="rhythm-empty-title">{explanation.sentence}</h2>
      {rhythm.refusal && explanation.sentence !== rhythm.refusal.message && (
        <p>{inEstimatorWords(rhythm.refusal.message)}</p>
      )}
      <p>{explanation.next.body}</p>
      {explanation.next.link && (
        <a className="button primary" href={explanation.next.link.href}>
          {explanation.next.link.label}
        </a>
      )}
    </section>
  );
}

function contextLine(rhythm: RhythmData, mode: RhythmSource): string | undefined {
  const hasRhythm = rhythm.status === "estimated";
  if (rhythm.status === "unavailable") {
    return "Your records could not be read. Saved observations have not been changed; this retries by itself.";
  }
  if (mode === "synced") {
    return hasRhythm
      ? "Computed by your synced server's estimate."
      : "The synced server is waiting for enough sleep data before drawing rhythm charts.";
  }
  if (mode === "fixture") {
    return "Sample data: a read-only preview of imported, estimated and incomplete records.";
  }
  return undefined;
}

export function RhythmScreen({
  tab,
  onSelect,
}: {
  tab: RhythmTab;
  onSelect: (tab: RhythmTab) => void;
}) {
  const { data: result } = useLoaded(loadRhythm, {
    events: [sleepDataChangedEvent],
    expires: true,
  });
  // Markers are recorded in Log and *read* here: they are the context that
  // explains a jump in the actogram, so the chart would be misleading without
  // them even though nothing on this screen edits them.
  const markers = useRhythmMarkers();

  const rhythm = result?.data;
  const mode = result?.source;
  const hasRhythm = rhythm?.status === "estimated";
  const context = rhythm && mode ? contextLine(rhythm, mode) : undefined;
  const chart = (draw: (rhythm: RhythmData) => ReactNode) =>
    !rhythm ? <Loading /> : hasRhythm ? draw(rhythm) : <NoChartPanel rhythm={rhythm} />;

  return (
    <>
      <PageHeader
        title="Rhythm"
        actions={
          mode && (
            <div className="status-cluster">
              <span className="sync-dot" data-mode={mode} aria-hidden="true" />
              <span>{estimateSourceLabel(mode, hasRhythm)}</span>
            </div>
          )
        }
      />
      {/* The view refreshes itself; this line only speaks when the chart
          cannot be drawn or comes from somewhere other than this device. */}
      {context && <p className="screen-context">{context}</p>}
      <section className="rhythm-screen" aria-label="Rhythm review">
        <ScreenTabs
          name="rhythm"
          label="Rhythm views"
          tabs={rhythmTabs}
          active={tab}
          onSelect={onSelect}
        />

        <ScreenTabPanel name="rhythm" id="actogram" active={tab}>
          {chart((ready) => (
            <ActogramPanel
              actogram={ready.actogram}
              markers={mode === "fixture" ? [] : markers.data.markers}
            />
          ))}
        </ScreenTabPanel>

        <ScreenTabPanel name="rhythm" id="drift" active={tab}>
          {chart((ready) => (
            <DriftPanel drift={ready.drift} />
          ))}
        </ScreenTabPanel>

        <ScreenTabPanel name="rhythm" id="sources" active={tab}>
          <SourcesPanel />
        </ScreenTabPanel>
      </section>
    </>
  );
}

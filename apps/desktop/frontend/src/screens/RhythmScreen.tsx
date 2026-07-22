import { useEffect, useState, type KeyboardEvent } from "react";
import { Icon } from "../components/Icon";
import { PageHeader } from "../components/AppShell";
import { ActogramPanel, DriftPanel } from "../components/RhythmVisuals";
import { RhythmMarkersPanel } from "../components/RhythmMarkersPanel";
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
import {
  addRhythmMarker,
  deleteRhythmMarker,
  downloadRhythmMarkerExport,
  exportRhythmMarkers,
  loadRhythmMarkers,
  notifyRhythmMarkersChanged,
  rhythmMarkersChangedEvent,
  unavailableRhythmMarkers,
  type RhythmMarkerInput,
} from "../data/rhythmMarkers";

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

type RhythmTab = "actogram" | "drift" | "context" | "sources";

const rhythmTabs: { id: RhythmTab; label: string }[] = [
  { id: "actogram", label: "Actogram" },
  { id: "drift", label: "Drift" },
  { id: "context", label: "Context" },
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
    let current = true;
    const refresh = () =>
      void loadSleepEntries().then((loaded) => {
        if (current) setEntriesData(loaded);
      });
    refresh();
    window.addEventListener(sleepDataChangedEvent, refresh);
    return () => {
      current = false;
      window.removeEventListener(sleepDataChangedEvent, refresh);
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
            <a className="task-chip" href="#/data-sources">
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
          <a href="#/data-sources">
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
      <a className="button primary" href="#/data-sources">
        Add sleep entry
      </a>
    </section>
  );
}

export function RhythmScreen() {
  const [tab, setTab] = useState<RhythmTab>("actogram");
  const [rhythm, setRhythm] = useState(rhythmFixture);
  const [mode, setMode] = useState<RhythmSource>("fixture");
  const [markers, setMarkers] = useState(unavailableRhythmMarkers);
  const [markerBusy, setMarkerBusy] = useState(false);
  const [markerExporting, setMarkerExporting] = useState(false);
  const [markerError, setMarkerError] = useState("");
  const [markerAnnouncement, setMarkerAnnouncement] = useState("");

  useEffect(() => {
    let current = true;
    const refresh = () =>
      void loadRhythm().then((result) => {
        if (current) {
          setRhythm(result.data);
          setMode(result.source);
        }
      });
    refresh();
    window.addEventListener(sleepDataChangedEvent, refresh);
    return () => {
      current = false;
      window.removeEventListener(sleepDataChangedEvent, refresh);
    };
  }, []);

  useEffect(() => {
    let current = true;
    const refresh = () =>
      void loadRhythmMarkers().then(
        (result) => {
          if (!current) return;
          setMarkers(result);
          setMarkerError("");
        },
        (reason: unknown) => {
          if (!current) return;
          setMarkerError(
            reason instanceof Error ? reason.message : "Rhythm markers could not be loaded.",
          );
        },
      );
    refresh();
    window.addEventListener(rhythmMarkersChangedEvent, refresh);
    return () => {
      current = false;
      window.removeEventListener(rhythmMarkersChangedEvent, refresh);
    };
  }, []);

  const appendMarker = async (input: RhythmMarkerInput) => {
    if (markerBusy) return;
    setMarkerBusy(true);
    setMarkerError("");
    try {
      const result = await addRhythmMarker(input);
      setMarkers(result);
      setMarkerAnnouncement("Context marker appended.");
      notifyRhythmMarkersChanged();
    } catch (reason) {
      setMarkerError(
        reason instanceof Error ? reason.message : "Context marker could not be saved.",
      );
      throw reason;
    } finally {
      setMarkerBusy(false);
    }
  };

  const eraseMarker = async (markerId: string, confirmation: string) => {
    if (markerBusy) return;
    setMarkerBusy(true);
    setMarkerError("");
    try {
      const result = await deleteRhythmMarker(markerId, confirmation);
      setMarkers(result);
      setMarkerAnnouncement("Context marker permanently erased.");
      notifyRhythmMarkersChanged();
    } catch (reason) {
      setMarkerError(
        reason instanceof Error ? reason.message : "Context marker could not be erased.",
      );
      throw reason;
    } finally {
      setMarkerBusy(false);
    }
  };

  const exportMarkers = () => {
    if (markerExporting || markers.status === "unavailable") return;
    setMarkerExporting(true);
    setMarkerError("");
    void exportRhythmMarkers().then(
      (result) => {
        setMarkerExporting(false);
        const downloaded = downloadRhythmMarkerExport(result);
        setMarkerAnnouncement(
          `${result.markerCount} context ${result.markerCount === 1 ? "marker" : "markers"} exported${downloaded ? ` to ${result.fileName}` : "."}`,
        );
      },
      (reason: unknown) => {
        setMarkerExporting(false);
        setMarkerError(
          reason instanceof Error ? reason.message : "Context markers could not be exported.",
        );
      },
    );
  };

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

  const onTabKey = (event: KeyboardEvent<HTMLDivElement>) => {
    const index = rhythmTabs.findIndex((item) => item.id === tab);
    let nextIndex: number;
    if (event.key === "ArrowRight") nextIndex = (index + 1) % rhythmTabs.length;
    else if (event.key === "ArrowLeft")
      nextIndex = (index - 1 + rhythmTabs.length) % rhythmTabs.length;
    else if (event.key === "Home") nextIndex = 0;
    else if (event.key === "End") nextIndex = rhythmTabs.length - 1;
    else return;
    event.preventDefault();
    const next = rhythmTabs[nextIndex];
    if (next) setTab(next.id);
  };

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
        <div className="rhythm-tabs" role="tablist" aria-label="Rhythm views" onKeyDown={onTabKey}>
          {rhythmTabs.map((item) => (
            <button
              key={item.id}
              className={`filter${tab === item.id ? " active" : ""}`}
              type="button"
              role="tab"
              id={`rhythm-tab-${item.id}`}
              aria-selected={tab === item.id}
              aria-controls={`rhythm-panel-${item.id}`}
              tabIndex={tab === item.id ? 0 : -1}
              onClick={() => setTab(item.id)}
            >
              {item.label}
            </button>
          ))}
        </div>

        {tab === "actogram" && (
          <div
            className="rhythm-panel"
            role="tabpanel"
            id="rhythm-panel-actogram"
            aria-labelledby="rhythm-tab-actogram"
          >
            {hasRhythm ? (
              <ActogramPanel
                actogram={rhythm.actogram}
                markers={mode === "fixture" ? [] : markers.markers}
              />
            ) : (
              <RhythmUnavailablePanel rhythm={rhythm} />
            )}
          </div>
        )}

        {tab === "drift" && (
          <div
            className="rhythm-panel"
            role="tabpanel"
            id="rhythm-panel-drift"
            aria-labelledby="rhythm-tab-drift"
          >
            {hasRhythm ? (
              <DriftPanel drift={rhythm.drift} />
            ) : (
              <RhythmUnavailablePanel rhythm={rhythm} />
            )}
          </div>
        )}

        {tab === "context" && (
          <div
            className="rhythm-panel"
            role="tabpanel"
            id="rhythm-panel-context"
            aria-labelledby="rhythm-tab-context"
          >
            <RhythmMarkersPanel
              data={markers}
              busy={markerBusy}
              exporting={markerExporting}
              error={markerError}
              announcement={markerAnnouncement}
              onAdd={appendMarker}
              onDelete={eraseMarker}
              onExport={exportMarkers}
            />
          </div>
        )}

        {tab === "sources" && (
          <div
            className="rhythm-panel"
            role="tabpanel"
            id="rhythm-panel-sources"
            aria-labelledby="rhythm-tab-sources"
          >
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
          </div>
        )}
      </section>
    </>
  );
}

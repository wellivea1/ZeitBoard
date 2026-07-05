import { useEffect, useState, type FormEvent, type KeyboardEvent } from "react";
import { Icon } from "../components/Icon";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";
import { useAppearanceContext } from "../theme/AppearanceProvider";
import type { ThemePreference } from "../theme/theme";
import { useApprovals } from "../state/approvals";
import {
  correctionPreviewFixture,
  refusalFixture,
  sourceConflictFixtures,
  type ChangeProposalFixture,
  type ProposalOrigin,
  type RhythmDriftPointFixture,
  type RhythmSleepBandFixture,
  type SourceConflictFixture,
} from "../data/phaseTwo";
import {
  loadRhythm,
  rhythmFixture,
  type RhythmActogram,
  type RhythmData,
  type RhythmDrift,
  type RhythmSource,
} from "../data/rhythm";
import {
  decideBackendProposal,
  loadBackendProposals,
  type BackendProposal,
  type BackendProposalsData,
} from "../data/backendProposals";
import {
  addTask,
  deleteTask,
  loadTasks,
  setTaskDone,
  type Task,
  type TasksData,
} from "../data/tasks";
import {
  addSleepEntry,
  correctSleepEntry,
  deleteAllSleepData,
  deleteSleepObservation,
  exportSleepData,
  latestCorrectedEntry,
  loadSleepEntries,
  summarizeSleepSources,
  suppressSleepEntry,
  type SleepClassification,
  type SleepCorrectionInput,
  type SleepDataExport,
  type SleepEntriesData,
  type SleepEntry,
  type SleepEntryInput,
} from "../data/sleepEntries";
import {
  configureBackendSync,
  disableBackendSync,
  loadBackendSyncStatus,
  syncNow,
  type BackendSyncInput,
  type BackendSyncStatus,
} from "../data/backendSync";
import { notifySleepDataChanged, sleepDataChangedEvent } from "../data/sleepDataEvents";
import type { ConfidenceLevel } from "../data/overview";

const days = ["Mon 15", "Tue 16", "Wed 17", "Thu 18", "Fri 19"];

const confidenceSegments: Record<ConfidenceLevel, number> = { Low: 1, Medium: 2, High: 3 };
const originLabels: Record<ProposalOrigin, string> = {
  scheduler: "Scheduler",
  assistant: "Assistant",
  sync_conflict: "Sync conflict",
};

function ConfidenceDots({ value }: { value: ConfidenceLevel }) {
  const filled = confidenceSegments[value];
  return (
    <span className="proposal-confidence" aria-label={`${value} confidence`}>
      {[0, 1, 2].map((index) => (
        <span key={index} data-muted={index >= filled || undefined} />
      ))}
    </span>
  );
}

function ProposalCard({ proposal }: { proposal: ChangeProposalFixture }) {
  const { decide } = useApprovals();
  return (
    <article className="panel proposal-card" data-origin={proposal.origin}>
      <div className="proposal-header">
        <span className="proposal-kind">{proposal.kind}</span>
        <div>
          <p className="section-kicker">{originLabels[proposal.origin]} proposal</p>
          <h2>{proposal.title}</h2>
        </div>
        <ConfidenceDots value={proposal.confidence} />
      </div>
      <p className="proposal-change">
        {proposal.from && <span>From {proposal.from}</span>}
        <strong>To {proposal.to}</strong>
        <small>{proposal.rhythmContext}</small>
      </p>
      <div className="proposal-reasons" aria-label="Proposal reasons">
        {proposal.reasonLabels.map((reason) => (
          <span className="task-chip" key={reason}>
            {reason}
          </span>
        ))}
      </div>
      <p className="proposal-meta">
        {proposal.createdLabel} - {proposal.expiresLabel}
      </p>
      <div className="approval-actions">
        <button
          className="button secondary"
          type="button"
          onClick={() => decide(proposal.id, "rejected")}
        >
          Reject proposal
        </button>
        <button
          className="button primary"
          type="button"
          onClick={() => decide(proposal.id, "approved")}
        >
          Accept proposal
        </button>
      </div>
    </article>
  );
}

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
      <button className="button secondary undo-button" type="button">
        {correctionPreviewFixture.undoLabel}
      </button>
    </aside>
  );
}

const DOUBLE_PLOT_HOURS = 48;

function hourToPercent(hour: number) {
  return `${(hour / DOUBLE_PLOT_HOURS) * 100}%`;
}

function bandStyle(startHour: number, durationHours: number) {
  return {
    left: hourToPercent(startHour),
    width: `${(durationHours / DOUBLE_PLOT_HOURS) * 100}%`,
  };
}

function bandAriaLabel(band: RhythmSleepBandFixture) {
  const prefix = band.kind === "forecast" ? "Predicted sleep window" : "Sleep interval";
  return `${prefix}: ${band.day}, ${band.startLabel} to ${band.wakeLabel}, ${band.durationLabel}, ${band.source}, ${band.confidence} confidence`;
}

function ActogramBand({
  band,
  duplicate = false,
}: {
  band: RhythmSleepBandFixture;
  duplicate?: boolean;
}) {
  const startHour = duplicate ? band.startHour + 24 : band.startHour;
  return (
    <span
      className={`actogram-band is-${band.kind}${duplicate ? " is-duplicate" : ""}`}
      style={bandStyle(startHour, band.durationHours)}
      tabIndex={duplicate ? undefined : 0}
      aria-hidden={duplicate || undefined}
      aria-label={duplicate ? undefined : bandAriaLabel(band)}
      role={duplicate ? undefined : "img"}
    >
      {!duplicate && (
        <span>{band.kind === "forecast" ? "Predicted sleep window" : band.source}</span>
      )}
    </span>
  );
}

function ActogramRow({ band, now }: { band: RhythmSleepBandFixture; now: RhythmActogram["now"] }) {
  const duplicateFits = band.startHour + 24 < DOUBLE_PLOT_HOURS;
  const showNowTick = band.day === now.day;

  return (
    <div className="actogram-visual-row">
      <time>{band.day}</time>
      <div className="actogram-visual-track">
        {band.originalStartHour !== undefined && band.originalDurationHours !== undefined && (
          <span
            className="actogram-band is-original"
            style={bandStyle(band.originalStartHour, band.originalDurationHours)}
            aria-hidden="true"
            title={band.originalLabel}
          />
        )}
        <ActogramBand band={band} />
        {duplicateFits && <ActogramBand band={band} duplicate />}
        {showNowTick && (
          <span
            className="actogram-now-tick"
            style={{ left: hourToPercent(now.hour) }}
            aria-hidden="true"
          />
        )}
      </div>
      <small>{band.confidence}</small>
    </div>
  );
}

function ActogramPanel({ actogram }: { actogram: RhythmActogram }) {
  const [showForecast, setShowForecast] = useState(true);
  const forecastRows = showForecast ? actogram.forecastRows : [];
  const allBands = [...actogram.observedRows, ...forecastRows];

  return (
    <section className="panel actogram-panel" aria-labelledby="actogram-title">
      <div className="panel-heading actogram-heading">
        <div>
          <p className="section-kicker">Recent cycles</p>
          <h2 id="actogram-title">Double-plot actogram</h2>
        </div>
        <div className="actogram-controls">
          <label>
            <input
              type="checkbox"
              checked={showForecast}
              onChange={(event) => setShowForecast(event.target.checked)}
            />{" "}
            Show forecast
          </label>
        </div>
      </div>

      <div className="actogram-chart" role="img" aria-label={actogram.summary}>
        <div className="actogram-visual-axis" aria-hidden="true">
          <span>0</span>
          <span>6</span>
          <span>12</span>
          <span>18</span>
          <span>0 (24)</span>
          <span>6</span>
          <span>12</span>
          <span>18</span>
          <span>0 (48)</span>
        </div>
        <div className="actogram-visual-grid">
          {actogram.observedRows.map((band) => (
            <ActogramRow band={band} now={actogram.now} key={band.id} />
          ))}
          {showForecast && (
            <div className="actogram-now-line" aria-hidden="true">
              <span>{actogram.now.label}</span>
            </div>
          )}
          {forecastRows.map((band) => (
            <ActogramRow band={band} now={actogram.now} key={band.id} />
          ))}
        </div>
      </div>

      <div className="actogram-footer">
        <span>
          <i className="legend-observed" /> observed
        </span>
        <span>
          <i className="legend-inferred" /> inferred
        </span>
        <span>
          <i className="legend-forecast" /> predicted
        </span>
        <span>| now</span>
        <p>Approximate. Forecast widens with time and is shown as ranges, not hard lines.</p>
      </div>

      <table className="sr-table">
        <caption>Actogram sleep and forecast table</caption>
        <thead>
          <tr>
            <th>Day</th>
            <th>Sleep start</th>
            <th>Wake</th>
            <th>Duration</th>
            <th>Source</th>
            <th>Confidence</th>
          </tr>
        </thead>
        <tbody>
          {allBands.map((band) => (
            <tr key={band.id}>
              <td>{band.day}</td>
              <td>{band.startLabel}</td>
              <td>{band.wakeLabel}</td>
              <td>{band.durationLabel}</td>
              <td>{band.source}</td>
              <td>{band.confidence}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function scaleDriftX(index: number, points: RhythmDriftPointFixture[]) {
  if (points.length === 1) return 50;
  return 8 + (index / (points.length - 1)) * 84;
}

function scaleDriftY(hour: number, yMinHour: number, yMaxHour: number) {
  const normalized = (hour - yMinHour) / (yMaxHour - yMinHour);
  return 90 - normalized * 72;
}

// Format an unwrapped onset hour (which may run below 0 or above 24) back to a
// civil clock label for the dynamic y-axis ticks.
function formatClockHour(hour: number) {
  const normalized = ((hour % 24) + 24) % 24;
  let display = Math.floor(normalized);
  let minute = Math.round((normalized - display) * 60);
  if (minute === 60) {
    display = (display + 1) % 24;
    minute = 0;
  }
  const period = display >= 12 ? "PM" : "AM";
  const hour12 = display % 12 === 0 ? 12 : display % 12;
  return minute === 0
    ? `${hour12} ${period}`
    : `${hour12}:${String(minute).padStart(2, "0")} ${period}`;
}

const DRIFT_TICKS = 4;

function DriftPanel({ drift }: { drift: RhythmDrift }) {
  const points = drift.points;
  const { yMinHour, yMaxHour } = drift;
  // Ticks run top (latest onset) to bottom (earliest), derived from the data
  // range so genuinely free-running onsets are never clipped.
  const ticks = Array.from(
    { length: DRIFT_TICKS },
    (_, i) => yMaxHour - (i / (DRIFT_TICKS - 1)) * (yMaxHour - yMinHour),
  );
  const fitPoints = points
    .map(
      (point, index) =>
        `${scaleDriftX(index, points)},${scaleDriftY(point.fitHour, yMinHour, yMaxHour)}`,
    )
    .join(" ");
  const bandPoints = [
    ...points.map(
      (point, index) =>
        `${scaleDriftX(index, points)},${scaleDriftY(point.bandHighHour, yMinHour, yMaxHour)}`,
    ),
    ...[...points]
      .reverse()
      .map(
        (point, reverseIndex) =>
          `${scaleDriftX(points.length - 1 - reverseIndex, points)},${scaleDriftY(point.bandLowHour, yMinHour, yMaxHour)}`,
      ),
  ].join(" ");

  return (
    <section className="panel drift-panel" aria-labelledby="drift-title">
      <div className="panel-heading">
        <div>
          <p className="section-kicker">Phase / drift</p>
          <h2 id="drift-title">{drift.title}</h2>
        </div>
        <div className="drift-summary">
          <strong>{drift.slopeLabel}</strong>
          <span>{drift.confidence} confidence</span>
        </div>
      </div>

      <div className="drift-body">
        <div className="drift-y-axis" aria-hidden="true">
          {ticks.map((hour) => (
            <span key={hour}>{formatClockHour(hour)}</span>
          ))}
        </div>
        <div className="drift-chart" role="img" aria-label={drift.summary}>
          <svg
            className="drift-svg"
            viewBox="0 0 100 100"
            preserveAspectRatio="none"
            aria-hidden="true"
          >
            {ticks.map((hour) => (
              <line
                className="drift-gridline"
                x1="0"
                x2="100"
                y1={scaleDriftY(hour, yMinHour, yMaxHour)}
                y2={scaleDriftY(hour, yMinHour, yMaxHour)}
                vectorEffect="non-scaling-stroke"
                key={hour}
              />
            ))}
            <polygon className="drift-band" points={bandPoints} />
            <polyline className="drift-fit" points={fitPoints} vectorEffect="non-scaling-stroke" />
            {points.map((point, index) => (
              <circle
                className="drift-point"
                cx={scaleDriftX(index, points)}
                cy={scaleDriftY(point.onsetHour, yMinHour, yMaxHour)}
                r="1.7"
                vectorEffect="non-scaling-stroke"
                key={point.id}
              />
            ))}
          </svg>
          <div className="drift-x-axis" aria-hidden="true">
            {points.map((point) => (
              <span key={point.id}>{point.day}</span>
            ))}
          </div>
        </div>
      </div>

      <div className="actogram-footer">
        <span>
          <i className="legend-point" /> observed onset
        </span>
        <span>
          <i className="legend-fit" /> Theil-Sen fit
        </span>
        <span>
          <i className="legend-band" /> uncertainty band
        </span>
        <p>Y-axis is unwrapped so the free-running trend stays readable across midnight.</p>
      </div>

      <table className="sr-table">
        <caption>Drift trend table</caption>
        <thead>
          <tr>
            <th>Day</th>
            <th>Sleep onset</th>
            <th>Source</th>
            <th>Confidence</th>
            <th>Fit</th>
          </tr>
        </thead>
        <tbody>
          {points.map((point) => (
            <tr key={point.id}>
              <td>{point.day}</td>
              <td>{point.onsetLabel}</td>
              <td>{point.source}</td>
              <td>{point.confidence}</td>
              <td>{drift.slopeLabel}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

export function CalendarScreen() {
  return (
    <>
      <PageHeader
        title="Calendar"
        description="Compare fixed events with uncertain predicted sleep and waking windows."
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode="fixture" aria-hidden="true" />
            <span>Sample preview</span>
          </div>
        }
      />
      <PlaceholderNotice>
        This five-day board is a synthetic design preview. Calendar import arrives with the
        interoperability phase; fixed events will remain immutable inputs.
      </PlaceholderNotice>
      <section className="panel calendar-board" aria-label="Five day planning preview">
        <div className="calendar-hours" aria-hidden="true">
          <span>12 AM</span>
          <span>6 AM</span>
          <span>12 PM</span>
          <span>6 PM</span>
          <span>12 AM</span>
        </div>
        <div className="calendar-days">
          {days.map((day, index) => (
            <article key={day}>
              <h2>{day}</h2>
              <div className="day-track">
                <span className="sleep-band" style={{ left: `${4 + index * 6}%`, width: "25%" }}>
                  Predicted sleep
                </span>
                <span className="wake-band" style={{ left: `${34 + index * 6}%`, width: "39%" }}>
                  Useful window
                </span>
                {index === 0 && (
                  <span className="fixed-event" style={{ left: "75%" }}>
                    Check-in
                  </span>
                )}
              </div>
            </article>
          ))}
        </div>
      </section>
    </>
  );
}

function TaskRow({
  task,
  busy,
  onToggleDone,
  onDelete,
}: {
  task: Task;
  busy: boolean;
  onToggleDone: (task: Task) => void;
  onDelete: (task: Task) => void;
}) {
  const details = [task.durationLabel, task.windowLabel, task.afterWakeLabel]
    .filter(Boolean)
    .join(" · ");
  return (
    <div className="task-row" role="row" data-status={task.status}>
      <span role="cell">
        <input
          type="checkbox"
          checked={task.status === "done"}
          disabled={busy}
          onChange={() => onToggleDone(task)}
          aria-label={`Mark ${task.title} ${task.status === "done" ? "open" : "done"}`}
        />
        {task.title}
      </span>
      <span role="cell">{details}</span>
      <span role="cell">
        <span className="task-chip">{task.status === "done" ? "Done" : "Open"}</span>
      </span>
      <span role="cell">
        <button
          className="button secondary"
          type="button"
          disabled={busy}
          onClick={() => onDelete(task)}
        >
          Delete
        </button>
      </span>
    </div>
  );
}

export function TasksScreen() {
  const { pending, pendingCount, unplaced } = useApprovals();
  const firstUnplaced = unplaced[0];
  const [data, setData] = useState<TasksData>({ status: "unavailable", tasks: [] });
  const [title, setTitle] = useState("");
  const [durationMinutes, setDurationMinutes] = useState(45);
  const [latestFinishLocal, setLatestFinishLocal] = useState("");
  const [afterWakeMinutes, setAfterWakeMinutes] = useState(0);
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState("");
  const [announcement, setAnnouncement] = useState("");

  useEffect(() => {
    let current = true;
    void loadTasks().then((result) => {
      if (current) setData(result);
    });
    return () => {
      current = false;
    };
  }, []);

  const available = data.status === "ok";
  const openTasks = data.tasks.filter((task) => task.status === "open");
  const doneTasks = data.tasks.filter((task) => task.status === "done");

  const runMutation = (operation: Promise<TasksData>, successMessage: string) => {
    setBusy(true);
    setFormError("");
    operation
      .then((result) => {
        setData(result);
        setAnnouncement(successMessage);
      })
      .catch((error: unknown) => {
        setFormError(error instanceof Error ? error.message : "The task action failed.");
      })
      .finally(() => setBusy(false));
  };

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    runMutation(
      addTask({
        title,
        durationMinutes,
        ...(latestFinishLocal ? { latestFinishLocal } : {}),
        ...(afterWakeMinutes > 0 ? { preferredAfterWakeMinutes: afterWakeMinutes } : {}),
        zoneId: Intl.DateTimeFormat().resolvedOptions().timeZone,
      }),
      `Added ${title}.`,
    );
    setTitle("");
    setLatestFinishLocal("");
    setAfterWakeMinutes(0);
  };

  return (
    <>
      <PageHeader
        title="Tasks"
        description="Describe flexibility and effort; the planner returns proposals, not calendar changes."
      />
      {!available && (
        <PlaceholderNotice>
          {data.message ??
            "This browser preview is read-only. Open the ZeitBoard desktop app to manage tasks."}
        </PlaceholderNotice>
      )}
      <section className="screen-grid" aria-label="Task planning and approvals">
        <section
          className="panel phase-two-panel approval-summary"
          aria-labelledby="approval-title"
        >
          <div className="panel-heading">
            <div>
              <p className="section-kicker">Proposal review</p>
              <h2 id="approval-title">Approval queue</h2>
            </div>
            <a href="#/approvals">
              Open all <Icon name="chevron" />
            </a>
          </div>
          <p className="phase-two-copy">
            {pendingCount > 0
              ? `${pendingCount} pending ${pendingCount === 1 ? "proposal is" : "proposals are"} waiting for explicit approval.`
              : "No proposals are waiting for approval."}
          </p>
          {pending.slice(0, 1).map((proposal) => (
            <ProposalCard proposal={proposal} key={proposal.id} />
          ))}
        </section>

        {available && (
          <section className="panel table-panel task-list-panel" aria-labelledby="add-task-title">
            <div className="panel-heading">
              <div>
                <p className="section-kicker">Flexible work</p>
                <h2 id="add-task-title">Add a task</h2>
              </div>
            </div>
            <form className="sleep-entry-fields" onSubmit={onSubmit}>
              <label>
                Task
                <input
                  type="text"
                  value={title}
                  maxLength={120}
                  required
                  disabled={busy}
                  onChange={(event) => setTitle(event.target.value)}
                />
              </label>
              <label>
                Duration (minutes)
                <input
                  type="number"
                  min={5}
                  max={720}
                  value={durationMinutes}
                  required
                  disabled={busy}
                  onChange={(event) => setDurationMinutes(Number(event.target.value))}
                />
              </label>
              <label>
                Finish by (optional)
                <input
                  type="datetime-local"
                  value={latestFinishLocal}
                  disabled={busy}
                  onChange={(event) => setLatestFinishLocal(event.target.value)}
                />
              </label>
              <label>
                Minutes after waking (optional)
                <input
                  type="number"
                  min={0}
                  max={1440}
                  value={afterWakeMinutes}
                  disabled={busy}
                  onChange={(event) => setAfterWakeMinutes(Number(event.target.value))}
                />
              </label>
              <button className="button primary" type="submit" disabled={busy || !title.trim()}>
                Save task
              </button>
            </form>
            {formError && <p className="diff-note">{formError}</p>}

            <div className="panel-heading">
              <div>
                <p className="section-kicker">Your tasks</p>
                <h2 id="task-list-title">
                  {openTasks.length} open{doneTasks.length > 0 && `, ${doneTasks.length} done`}
                </h2>
              </div>
            </div>
            {data.tasks.length > 0 ? (
              <div className="task-table" role="table" aria-labelledby="task-list-title">
                <div className="task-row task-head" role="row">
                  <span role="columnheader">Task</span>
                  <span role="columnheader">Constraints</span>
                  <span role="columnheader">Status</span>
                  <span role="columnheader">Actions</span>
                </div>
                {data.tasks.map((task) => (
                  <TaskRow
                    task={task}
                    busy={busy}
                    onToggleDone={(target) =>
                      runMutation(
                        setTaskDone(target.taskId, target.status !== "done"),
                        `${target.title} marked ${target.status === "done" ? "open" : "done"}.`,
                      )
                    }
                    onDelete={(target) =>
                      runMutation(deleteTask(target.taskId), `Deleted ${target.title}.`)
                    }
                    key={task.taskId}
                  />
                ))}
              </div>
            ) : (
              <p className="phase-two-copy">
                No tasks yet. Add flexible work above and the planner will propose windows inside
                your predicted waking time — nothing is scheduled without your approval.
              </p>
            )}
            <p role="status" aria-live="polite" className="sr-only">
              {announcement}
            </p>
          </section>
        )}

        <aside className="panel unplaced-panel" aria-labelledby="unplaced-title">
          <p className="section-kicker">Not proposed</p>
          {firstUnplaced ? (
            <>
              <h2 id="unplaced-title">{firstUnplaced.title}</h2>
              <p>{firstUnplaced.reason}</p>
              <small>{firstUnplaced.nextAction}</small>
            </>
          ) : (
            <>
              <h2 id="unplaced-title">Nothing waiting on a window</h2>
              <p>Every open task either has a proposal or there are no open tasks.</p>
            </>
          )}
        </aside>
      </section>
    </>
  );
}

function SyncedProposalCard({
  proposal,
  busy,
  onDecide,
}: {
  proposal: BackendProposal;
  busy: boolean;
  onDecide: (proposal: BackendProposal, decision: "approved" | "rejected") => void;
}) {
  return (
    <article className="panel proposal-card" data-origin="assistant">
      <div className="proposal-header">
        <span className="proposal-kind">Synced</span>
        <div>
          <p className="section-kicker">Backend proposal</p>
          <h2>{proposal.title}</h2>
        </div>
        <ConfidenceDots value={proposal.confidence} />
      </div>
      <p className="proposal-change">
        <strong>{proposal.window}</strong>
        {proposal.answer && <small>{proposal.answer}</small>}
      </p>
      {proposal.reasonLabels.length > 0 && (
        <div className="proposal-reasons" aria-label="Proposal reasons">
          {proposal.reasonLabels.map((reason) => (
            <span className="task-chip" key={reason}>
              {reason}
            </span>
          ))}
        </div>
      )}
      <p className="proposal-meta">
        {proposal.createdLabel} - {proposal.expiresLabel}
      </p>
      <div className="approval-actions">
        <button
          className="button secondary"
          type="button"
          disabled={busy || !proposal.decisionToken}
          onClick={() => onDecide(proposal, "rejected")}
        >
          Reject proposal
        </button>
        <button
          className="button primary"
          type="button"
          disabled={busy || !proposal.decisionToken}
          onClick={() => onDecide(proposal, "approved")}
        >
          Accept proposal
        </button>
      </div>
    </article>
  );
}

function SyncedProposalsPanel() {
  const [data, setData] = useState<BackendProposalsData>({ status: "off", proposals: [] });
  const [busy, setBusy] = useState(false);
  const [announcement, setAnnouncement] = useState("");

  useEffect(() => {
    let current = true;
    void loadBackendProposals().then((result) => {
      if (current) setData(result);
    });
    return () => {
      current = false;
    };
  }, []);

  // Omit, don't disable: with sync off this surface simply is not there.
  if (data.status === "off") return null;

  const pending = data.proposals.filter((proposal) => proposal.status === "pending");
  const decided = data.proposals.length - pending.length;

  const onDecide = (proposal: BackendProposal, decision: "approved" | "rejected") => {
    if (!proposal.decisionToken) return;
    setBusy(true);
    void decideBackendProposal({
      proposalId: proposal.proposalId,
      decision,
      token: proposal.decisionToken,
    }).then((result) => {
      setBusy(false);
      setData(result);
      setAnnouncement(
        result.status === "ok"
          ? `${decision === "approved" ? "Approved" : "Rejected"} ${proposal.title}.`
          : (result.message ?? "The decision could not be recorded."),
      );
    });
  };

  return (
    <section className="panel synced-proposals-panel" aria-labelledby="synced-proposals-title">
      <div className="panel-heading">
        <div>
          <p className="section-kicker">Synced backend</p>
          <h2 id="synced-proposals-title">Assistant and agent proposals</h2>
        </div>
        <div className="status-cluster">
          <span className="sync-dot" data-mode="backend" aria-hidden="true" />
          <span>{pending.length} pending</span>
        </div>
      </div>
      {data.status === "error" && (
        <p className="diff-note">{data.message ?? "Could not reach the synced backend."}</p>
      )}
      {pending.length > 0 ? (
        <div className="proposal-stack">
          {pending.map((proposal) => (
            <SyncedProposalCard
              proposal={proposal}
              busy={busy}
              onDecide={onDecide}
              key={proposal.proposalId}
            />
          ))}
        </div>
      ) : (
        data.status === "ok" && (
          <p className="phase-two-copy">
            No synced proposals are waiting.{" "}
            {decided > 0 && `${decided} earlier ${decided === 1 ? "decision is" : "decisions are"} recorded on the backend.`}
          </p>
        )
      )}
      <p role="status" aria-live="polite" className="sr-only">
        {announcement}
      </p>
    </section>
  );
}

export function ApprovalsScreen() {
  const { pending, pendingCount, unplaced, source } = useApprovals();
  const byOrigin = (origin: ProposalOrigin) =>
    pending.filter((proposal) => proposal.origin === origin).length;
  return (
    <>
      <PageHeader
        title="Approvals"
        description={
          pendingCount > 0
            ? `${pendingCount} pending ${pendingCount === 1 ? "proposal" : "proposals"}. Approve or reject each change before anything moves.`
            : "Nothing is waiting for your approval right now."
        }
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode={source} aria-hidden="true" />
            <span>{source === "backend" ? "Local plan" : "Sample data"}</span>
          </div>
        }
      />
      <PlaceholderNotice>
        Approving or rejecting updates the queue with one-tap undo. Fixed and imported events remain
        immutable; nothing is written to a calendar yet.
      </PlaceholderNotice>
      <section className="screen-grid approval-screen" aria-label="Pending approval queue">
        <div className="panel approval-filter" aria-label="Pending proposals by origin">
          <span>All {pendingCount}</span>
          <span>Scheduler {byOrigin("scheduler")}</span>
          <span>Assistant {byOrigin("assistant")}</span>
          <span>Sync {byOrigin("sync_conflict")}</span>
        </div>
        {pendingCount > 0 ? (
          <div className="proposal-stack">
            {pending.map((proposal) => (
              <ProposalCard proposal={proposal} key={proposal.id} />
            ))}
          </div>
        ) : (
          <div className="panel empty-state">
            <p className="section-kicker">All clear</p>
            <h2>Nothing waiting for approval</h2>
            <p>
              Proposals from the planner and assistant appear here. Nothing changes until you
              approve it.
            </p>
          </div>
        )}

        <SyncedProposalsPanel />

        {unplaced.length > 0 && (
          <section className="panel unplaced-panel" aria-labelledby="approvals-unplaced-title">
            <p className="section-kicker">Not proposed</p>
            <h2 id="approvals-unplaced-title">
              {unplaced.length} task{unplaced.length === 1 ? "" : "s"} without a safe window
            </h2>
            <ul className="unplaced-list">
              {unplaced.map((item) => (
                <li key={item.title}>
                  <strong>{item.title}</strong> — {item.reason}. <small>{item.nextAction}</small>
                </li>
              ))}
            </ul>
          </section>
        )}
      </section>
    </>
  );
}

type RhythmTab = "actogram" | "drift" | "sources";

const rhythmTabs: { id: RhythmTab; label: string }[] = [
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

  const hasRhythm = rhythm.status === "estimated";
  const sourceLabel =
    mode === "synced"
      ? "Synced - server estimate"
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
      <PlaceholderNotice>
        {mode === "synced" && hasRhythm
          ? "The actogram, drift fit, and forecast below are computed by the synced server estimate."
          : mode === "synced"
            ? "The synced server estimator is waiting for enough sleep data before drawing rhythm charts."
            : mode === "local" && hasRhythm
              ? "The actogram, drift fit, and forecast below are computed by the local estimation engine."
              : mode === "local"
                ? "The local estimator is waiting for enough manually entered sleep data before drawing rhythm charts."
                : "This read-only preview distinguishes imported, estimated, corrected, and incomplete observations."}
      </PlaceholderNotice>
      <section className="screen-grid rhythm-screen" aria-label="Rhythm review">
        <div
          className="panel rhythm-tabs"
          role="tablist"
          aria-label="Rhythm views"
          onKeyDown={onTabKey}
        >
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
              <ActogramPanel actogram={rhythm.actogram} />
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
        These records are a synthetic design preview — medication logging is a planned feature.
        It will record clock-time doses you enter; it will never recommend one.
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
          <button className="icon-button" type="button" aria-label="Open morning record">
            <Icon name="chevron" />
          </button>
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
          <button className="icon-button" type="button" aria-label="Open evening record">
            <Icon name="chevron" />
          </button>
        </article>
      </section>
    </>
  );
}

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
          <button className="icon-button" type="button" aria-label="Open household sharing profile">
            <Icon name="chevron" />
          </button>
        </article>
        <article className="panel share-profile">
          <div className="avatar soft">WC</div>
          <div className="profile-copy">
            <span className="active-pill">Active</span>
            <h2>Work coordinator</h2>
            <p>Availability windows only</p>
          </div>
          <button
            className="icon-button"
            type="button"
            aria-label="Open work coordinator sharing profile"
          >
            <Icon name="chevron" />
          </button>
        </article>
        <article className="panel share-profile muted">
          <div className="avatar neutral">EC</div>
          <div className="profile-copy">
            <span className="inactive-pill">Paused</span>
            <h2>Emergency contact</h2>
            <p>No fields currently visible</p>
          </div>
          <button
            className="icon-button"
            type="button"
            aria-label="Open emergency contact sharing profile"
          >
            <Icon name="chevron" />
          </button>
        </article>
      </section>
    </>
  );
}

const fallbackSleepZone = "America/New_York";
const deleteConfirmationToken = "DELETE";

function browserZone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || fallbackSleepZone;
}

function dateTimeInputValue(value: Date) {
  const local = new Date(value.getTime() - value.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function initialSleepForm(): SleepEntryInput {
  const end = new Date();
  end.setSeconds(0, 0);
  const start = new Date(end.getTime() - 8 * 60 * 60 * 1000);
  return {
    startLocal: dateTimeInputValue(start),
    endLocal: dateTimeInputValue(end),
    zoneId: browserZone(),
    classification: "principal",
  };
}

function endAfterStart(input: SleepEntryInput) {
  return new Date(input.endLocal).getTime() > new Date(input.startLocal).getTime();
}

function downloadSleepDataExport(exported: SleepDataExport) {
  if (
    (typeof navigator !== "undefined" && navigator.userAgent.toLowerCase().includes("jsdom")) ||
    typeof Blob === "undefined" ||
    typeof URL === "undefined" ||
    typeof URL.createObjectURL !== "function" ||
    typeof document === "undefined"
  ) {
    return false;
  }
  const blob = new Blob([exported.json], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = exported.fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
  return true;
}

function SleepEntryForm({
  form,
  onChange,
  submitLabel,
  disabled,
}: {
  form: SleepEntryInput;
  onChange: (form: SleepEntryInput) => void;
  submitLabel: string;
  disabled: boolean;
}) {
  return (
    <div className="sleep-entry-fields">
      <label>
        Sleep start
        <input
          type="datetime-local"
          value={form.startLocal}
          onChange={(event) => onChange({ ...form, startLocal: event.target.value })}
          required
        />
      </label>
      <label>
        Wake time
        <input
          type="datetime-local"
          value={form.endLocal}
          onChange={(event) => onChange({ ...form, endLocal: event.target.value })}
          required
        />
      </label>
      <label>
        Time zone
        <input
          type="text"
          value={form.zoneId}
          onChange={(event) => onChange({ ...form, zoneId: event.target.value })}
          required
        />
      </label>
      <label>
        Classification
        <select
          value={form.classification}
          onChange={(event) =>
            onChange({ ...form, classification: event.target.value as SleepClassification })
          }
        >
          <option value="principal">Principal sleep</option>
          <option value="nap">Nap</option>
        </select>
      </label>
      <button className="button primary" type="submit" disabled={disabled}>
        {submitLabel}
      </button>
    </div>
  );
}

function SleepEntryCard({
  entry,
  editing,
  editForm,
  busy,
  deleteConfirming,
  deleteConfirmation,
  onBeginEdit,
  onCancelEdit,
  onEditChange,
  onSaveEdit,
  onSuppress,
  onBeginDelete,
  onCancelDelete,
  onDeleteConfirmationChange,
  onDelete,
}: {
  entry: SleepEntry;
  editing: boolean;
  editForm: SleepEntryInput;
  busy: boolean;
  deleteConfirming: boolean;
  deleteConfirmation: string;
  onBeginEdit: () => void;
  onCancelEdit: () => void;
  onEditChange: (form: SleepEntryInput) => void;
  onSaveEdit: () => void;
  onSuppress: () => void;
  onBeginDelete: () => void;
  onCancelDelete: () => void;
  onDeleteConfirmationChange: (value: string) => void;
  onDelete: () => void;
}) {
  const corrected =
    entry.startLocal !== entry.effectiveStartLocal ||
    entry.endLocal !== entry.effectiveEndLocal ||
    entry.classification !== entry.effectiveClassification;
  const deleteInputID = `delete-confirm-${entry.observationId}`;

  return (
    <article className="sleep-entry-card" data-suppressed={entry.suppressed || undefined}>
      <div className="sleep-entry-main">
        <div className="record-icon">
          <Icon name="moon" />
        </div>
        <div>
          <h2>
            {entry.effectiveStartLabel} to {entry.effectiveEndLabel}
          </h2>
          <p>
            {entry.durationLabel} - {entry.effectiveClassification} - {entry.provenanceLabel}
          </p>
          {corrected && (
            <p className="sleep-entry-raw">
              Raw entry: {entry.startLabel} to {entry.endLabel}
            </p>
          )}
        </div>
        <span className={`source-status ${entry.suppressed ? "" : "connected"}`}>
          {entry.suppressed ? "Suppressed" : corrected ? "Corrected" : "Active"}
        </span>
      </div>

      {editing ? (
        <form
          className="sleep-edit-form"
          onSubmit={(event) => {
            event.preventDefault();
            onSaveEdit();
          }}
        >
          <SleepEntryForm
            form={editForm}
            onChange={onEditChange}
            submitLabel="Save correction"
            disabled={busy}
          />
          <button className="button secondary" type="button" onClick={onCancelEdit}>
            Cancel
          </button>
        </form>
      ) : (
        <div className="sleep-entry-actions">
          <button className="button secondary" type="button" onClick={onBeginEdit}>
            Edit by correction
          </button>
          <button
            className="button secondary"
            type="button"
            onClick={onSuppress}
            disabled={entry.suppressed || busy}
          >
            Suppress from estimates
          </button>
          <button className="button secondary danger-outline" type="button" onClick={onBeginDelete}>
            Delete permanently
          </button>
        </div>
      )}

      {deleteConfirming && (
        <div
          className="sleep-delete-confirmation"
          role="group"
          aria-labelledby={`${deleteInputID}-title`}
        >
          <div>
            <strong id={`${deleteInputID}-title`}>Permanent erase</strong>
            <p>
              Type DELETE to remove this observation and its correction history from local sleep
              storage. Use suppress when you only want it excluded from estimates.
            </p>
          </div>
          <label htmlFor={deleteInputID}>Deletion confirmation</label>
          <input
            id={deleteInputID}
            type="text"
            value={deleteConfirmation}
            onChange={(event) => onDeleteConfirmationChange(event.target.value)}
          />
          <div className="sleep-delete-actions">
            <button
              className="button danger"
              type="button"
              onClick={onDelete}
              disabled={busy || deleteConfirmation !== deleteConfirmationToken}
            >
              Erase entry
            </button>
            <button className="button secondary" type="button" onClick={onCancelDelete}>
              Cancel erase
            </button>
          </div>
        </div>
      )}

      {entry.history.length > 0 && (
        <details className="sleep-entry-history">
          <summary>Correction history ({entry.history.length})</summary>
          <ul>
            {entry.history.map((item) => (
              <li key={item.correctionId}>
                <strong>{item.createdLabel}</strong> - {item.summary}
              </li>
            ))}
          </ul>
        </details>
      )}
    </article>
  );
}

export function DataSourcesScreen() {
  const [entriesData, setEntriesData] = useState<SleepEntriesData>({
    status: "empty",
    empty: true,
    message: "Loading local sleep entries.",
    entries: [],
  });
  const [form, setForm] = useState<SleepEntryInput>(initialSleepForm);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editForm, setEditForm] = useState<SleepEntryInput>(initialSleepForm);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState("");
  const [statusMessage, setStatusMessage] = useState("");
  const [syncStatus, setSyncStatus] = useState<BackendSyncStatus | undefined>(undefined);

  const refreshEntries = async () => {
    const loaded = await loadSleepEntries();
    setEntriesData(loaded);
  };

  useEffect(() => {
    let current = true;
    void loadSleepEntries()
      .then((loaded) => {
        if (current) setEntriesData(loaded);
      })
      .catch((error: unknown) => {
        if (!current) return;
        setEntriesData({
          status: "unavailable",
          empty: true,
          message: error instanceof Error ? error.message : "Manual sleep log is unavailable.",
          entries: [],
        });
      });
    void loadBackendSyncStatus()
      .then((loaded) => {
        if (current) setSyncStatus(loaded);
      })
      .catch(() => {
        // The sync row simply stays hidden when status is unavailable.
      });
    return () => {
      current = false;
    };
  }, []);

  const submitEntry = async () => {
    setFormError("");
    if (!endAfterStart(form)) {
      setFormError("Wake time must be after sleep start.");
      return;
    }
    setBusy(true);
    try {
      await addSleepEntry(form);
      notifySleepDataChanged();
      setStatusMessage("Sleep entry saved locally.");
      setForm(initialSleepForm());
      await refreshEntries();
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not save sleep entry.");
    } finally {
      setBusy(false);
    }
  };

  const beginEdit = (entry: SleepEntry) => {
    setEditingId(entry.observationId);
    setDeletingId(null);
    setDeleteConfirmation("");
    setEditForm({
      startLocal: entry.effectiveStartLocal,
      endLocal: entry.effectiveEndLocal,
      zoneId: entry.zoneId,
      classification: entry.effectiveClassification,
    });
    setFormError("");
  };

  const saveEdit = async () => {
    if (!editingId) return;
    setFormError("");
    if (!endAfterStart(editForm)) {
      setFormError("Wake time must be after sleep start.");
      return;
    }
    setBusy(true);
    try {
      const correction: SleepCorrectionInput = { observationId: editingId, ...editForm };
      await correctSleepEntry(correction);
      notifySleepDataChanged();
      setStatusMessage("Correction appended locally.");
      setEditingId(null);
      await refreshEntries();
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not append correction.");
    } finally {
      setBusy(false);
    }
  };

  const suppressEntry = async (entry: SleepEntry) => {
    setBusy(true);
    setFormError("");
    setDeletingId(null);
    try {
      await suppressSleepEntry(entry.observationId);
      notifySleepDataChanged();
      setStatusMessage("Sleep entry suppressed from estimates.");
      await refreshEntries();
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not suppress entry.");
    } finally {
      setBusy(false);
    }
  };

  const beginDelete = (entry: SleepEntry) => {
    setDeletingId(entry.observationId);
    setDeleteConfirmation("");
    setEditingId(null);
    setFormError("");
  };

  const deleteEntry = async (entry: SleepEntry) => {
    if (deleteConfirmation !== deleteConfirmationToken) {
      setFormError("Type DELETE to confirm permanent erasure.");
      return;
    }
    setBusy(true);
    setFormError("");
    try {
      const loaded = await deleteSleepObservation(entry.observationId, deleteConfirmation);
      setEntriesData(loaded);
      notifySleepDataChanged();
      setStatusMessage("Sleep entry erased permanently.");
      setDeletingId(null);
      setDeleteConfirmation("");
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not erase sleep entry.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <PageHeader
        title="Data Sources"
        description="Enter local sleep episodes and review immutable observations plus append-only corrections."
      />
      <section className="screen-grid data-source-screen" aria-label="Data source review">
        <section className="panel sleep-entry-panel" aria-labelledby="sleep-entry-title">
          <div className="panel-heading">
            <div>
              <p className="section-kicker">Manual input</p>
              <h2 id="sleep-entry-title">Add sleep entry</h2>
            </div>
          </div>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void submitEntry();
            }}
          >
            <SleepEntryForm
              form={form}
              onChange={setForm}
              submitLabel="Save sleep entry"
              disabled={busy || entriesData.status === "unavailable"}
            />
          </form>
          {formError && (
            <p className="form-error" role="alert">
              {formError}
            </p>
          )}
          <p className="form-status" role="status" aria-live="polite">
            {statusMessage || entriesData.message}
          </p>
        </section>

        <section className="panel sleep-entry-list-panel" aria-labelledby="sleep-log-title">
          <div className="panel-heading">
            <div>
              <p className="section-kicker">Local observations</p>
              <h2 id="sleep-log-title">Sleep log</h2>
            </div>
          </div>
          {entriesData.entries.length === 0 ? (
            <div className="empty-state sleep-log-empty">
              <p className="section-kicker">{entriesData.status}</p>
              <h2>No sleep entries yet</h2>
              <p>
                Add your first principal sleep episode above. The estimator will stay refused until
                enough usable entries exist.
              </p>
            </div>
          ) : (
            <div className="sleep-entry-list">
              {entriesData.entries.map((entry) => (
                <SleepEntryCard
                  key={entry.observationId}
                  entry={entry}
                  editing={editingId === entry.observationId}
                  editForm={editForm}
                  busy={busy}
                  deleteConfirming={deletingId === entry.observationId}
                  deleteConfirmation={deletingId === entry.observationId ? deleteConfirmation : ""}
                  onBeginEdit={() => beginEdit(entry)}
                  onCancelEdit={() => setEditingId(null)}
                  onEditChange={setEditForm}
                  onSaveEdit={saveEdit}
                  onSuppress={() => void suppressEntry(entry)}
                  onBeginDelete={() => beginDelete(entry)}
                  onCancelDelete={() => {
                    setDeletingId(null);
                    setDeleteConfirmation("");
                  }}
                  onDeleteConfirmationChange={setDeleteConfirmation}
                  onDelete={() => void deleteEntry(entry)}
                />
              ))}
            </div>
          )}
        </section>

        <section className="panel source-list source-status-panel" aria-label="Data source status">
          <article>
            <div className="source-mark">
              <Icon name="clock" />
            </div>
            <div>
              <h2>Manual sleep log</h2>
              <p>
                {entriesData.entries.length} local{" "}
                {entriesData.entries.length === 1 ? "entry" : "entries"} - observations immutable,
                corrections append-only
              </p>
            </div>
            <span className={`source-status ${entriesData.status === "ready" ? "connected" : ""}`}>
              {entriesData.status === "ready" ? "Available" : entriesData.status}
            </span>
          </article>
          {syncStatus && (
            <article>
              <div className="source-mark">
                <Icon name="sources" />
              </div>
              <div>
                <h2>Server sync</h2>
                <p>
                  {syncStatus.enabled
                    ? `Your own instance - ${syncStatus.pushedCount} pushed, ${syncStatus.pulledCount} pulled${
                        syncStatus.lastSyncLabel ? `, last sync ${syncStatus.lastSyncLabel}` : ""
                      }`
                    : "Off - data stays on this device unless you enroll in Settings"}
                </p>
              </div>
              <span
                className={`source-status ${syncStatus.status === "connected" ? "connected" : ""}`}
              >
                {syncStatus.status === "connected"
                  ? "Connected"
                  : syncStatus.status === "error"
                    ? "Error"
                    : "Off"}
              </span>
            </article>
          )}
          <article>
            <div className="source-mark">
              <Icon name="calendar" />
            </div>
            <div>
              <h2>Calendar import</h2>
              <p>Out of scope for this local sleep-data slice</p>
            </div>
            <span className="source-status">Future</span>
          </article>
          <article>
            <div className="source-mark">
              <Icon name="sources" />
            </div>
            <div>
              <h2>Device activity</h2>
              <p>Not connected - optional supporting observation later</p>
            </div>
            <span className="source-status">Off</span>
          </article>
        </section>
      </section>
    </>
  );
}

const initialBackendSyncStatus: BackendSyncStatus = {
  enabled: false,
  status: "off",
  backendUrl: "",
  deviceId: "",
  insecureSkipVerify: false,
  lastSyncLabel: "Not synced yet",
  lastError: "",
  pendingPushCount: 0,
  pushedCount: 0,
  pulledCount: 0,
  cursor: 0,
};

function initialBackendSyncForm(status = initialBackendSyncStatus): BackendSyncInput {
  return {
    enabled: true,
    backendUrl: status.backendUrl,
    enrollmentSecret: "",
    deviceLabel: "ZeitBoard desktop",
    insecureSkipVerify: status.insecureSkipVerify,
  };
}

function backendSyncStatusLabel(status: BackendSyncStatus) {
  if (!status.enabled) return "Off";
  if (status.status === "error") return "Needs attention";
  return "Connected";
}

export function SettingsScreen() {
  const { theme, reducedStimulation, setTheme, setReducedStimulation } = useAppearanceContext();
  const [exportedSleepData, setExportedSleepData] = useState<SleepDataExport | null>(null);
  const [dataControlStatus, setDataControlStatus] = useState("");
  const [dataControlError, setDataControlError] = useState("");
  const [deleteAllConfirmation, setDeleteAllConfirmation] = useState("");
  const [dataControlBusy, setDataControlBusy] = useState(false);
  const [backendSyncStatus, setBackendSyncStatus] =
    useState<BackendSyncStatus>(initialBackendSyncStatus);
  const [backendSyncForm, setBackendSyncForm] = useState<BackendSyncInput>(() =>
    initialBackendSyncForm(),
  );
  const [backendSyncMessage, setBackendSyncMessage] = useState("");
  const [backendSyncError, setBackendSyncError] = useState("");
  const [backendSyncBusy, setBackendSyncBusy] = useState(false);

  useEffect(() => {
    let current = true;
    void loadBackendSyncStatus()
      .then((status) => {
        if (!current) return;
        setBackendSyncStatus(status);
        setBackendSyncForm((form) => ({
          ...form,
          backendUrl: status.backendUrl || form.backendUrl,
          insecureSkipVerify: status.insecureSkipVerify,
        }));
      })
      .catch((error: unknown) => {
        if (!current) return;
        setBackendSyncError(
          error instanceof Error ? error.message : "Could not read backend sync status.",
        );
      });
    return () => {
      current = false;
    };
  }, []);

  const handleExportSleepData = async () => {
    setDataControlBusy(true);
    setDataControlError("");
    try {
      const exported = await exportSleepData();
      setExportedSleepData(exported);
      const downloaded = downloadSleepDataExport(exported);
      setDataControlStatus(
        `${downloaded ? "Downloaded" : "Prepared"} ${exported.observationCount} ${
          exported.observationCount === 1 ? "observation" : "observations"
        } and ${exported.correctionCount} ${
          exported.correctionCount === 1 ? "correction" : "corrections"
        } from ${exported.generatedLabel}.`,
      );
    } catch (error) {
      setDataControlError(error instanceof Error ? error.message : "Could not export sleep data.");
    } finally {
      setDataControlBusy(false);
    }
  };

  const handleDeleteAllSleepData = async () => {
    if (deleteAllConfirmation !== deleteConfirmationToken) {
      setDataControlError("Type DELETE to confirm permanent erasure.");
      return;
    }
    setDataControlBusy(true);
    setDataControlError("");
    try {
      await deleteAllSleepData(deleteAllConfirmation);
      notifySleepDataChanged();
      setExportedSleepData(null);
      setDeleteAllConfirmation("");
      setDataControlStatus("All local sleep observations and correction history were erased.");
    } catch (error) {
      setDataControlError(error instanceof Error ? error.message : "Could not erase sleep data.");
    } finally {
      setDataControlBusy(false);
    }
  };

  const handleConfigureBackendSync = async () => {
    setBackendSyncBusy(true);
    setBackendSyncError("");
    setBackendSyncMessage("");
    try {
      const status = await configureBackendSync(backendSyncForm);
      setBackendSyncStatus(status);
      setBackendSyncForm((form) => ({ ...form, enrollmentSecret: "" }));
      setBackendSyncMessage(
        "Backend sync enabled. Synced server estimates are now available when the backend is reachable.",
      );
    } catch (error) {
      setBackendSyncError(
        error instanceof Error ? error.message : "Could not enable backend sync.",
      );
      setBackendSyncForm((form) => ({ ...form, enrollmentSecret: "" }));
    } finally {
      setBackendSyncBusy(false);
    }
  };

  const handleDisableBackendSync = async () => {
    setBackendSyncBusy(true);
    setBackendSyncError("");
    setBackendSyncMessage("");
    try {
      const status = await disableBackendSync();
      setBackendSyncStatus(status);
      setBackendSyncMessage(
        "Backend sync disabled. Estimates remain local and no sync network calls will be made.",
      );
    } catch (error) {
      setBackendSyncError(
        error instanceof Error ? error.message : "Could not disable backend sync.",
      );
    } finally {
      setBackendSyncBusy(false);
    }
  };

  const handleSyncNow = async () => {
    setBackendSyncBusy(true);
    setBackendSyncError("");
    setBackendSyncMessage("");
    try {
      const status = await syncNow();
      setBackendSyncStatus(status);
      if (status.status === "error") {
        setBackendSyncError(status.lastError || "Backend sync failed.");
      } else if (!status.enabled) {
        setBackendSyncMessage("Backend sync is off.");
      } else {
        setBackendSyncMessage(
          `Sync complete: ${status.pushedCount} pushed, ${status.pulledCount} pulled.`,
        );
      }
    } catch (error) {
      setBackendSyncError(error instanceof Error ? error.message : "Could not sync now.");
    } finally {
      setBackendSyncBusy(false);
    }
  };

  return (
    <>
      <PageHeader
        title="Settings"
        description="Control display, local storage, and estimate presentation."
      />
      <section className="settings-stack">
        <div className="panel settings-section">
          <div>
            <p className="section-kicker">Display</p>
            <h2>Time and appearance</h2>
          </div>
          <label>
            Appearance
            <select
              value={theme}
              onChange={(event) => setTheme(event.target.value as ThemePreference)}
            >
              <option value="auto">Auto</option>
              <option value="light">Light</option>
              <option value="dark">Dark</option>
            </select>
          </label>
          <label className="toggle-row settings-row">
            <span>
              <strong>Reduced stimulation</strong>
              <small>Soften motion, saturation, and contrast. Works in light and dark modes.</small>
            </span>
            <input
              type="checkbox"
              checked={reducedStimulation}
              onChange={(event) => setReducedStimulation(event.target.checked)}
            />
          </label>
        </div>
        <div className="panel settings-section">
          <div>
            <p className="section-kicker">Estimates</p>
            <h2>Uncertainty display</h2>
          </div>
          <p className="settings-copy">
            Uncertainty is never hidden: estimates always show predicted ranges rather than
            single-point times, ordinal confidence, and the reasons behind it.
          </p>
        </div>
        <div className="panel settings-section backend-sync-panel">
          <div className="data-control-intro">
            <p className="section-kicker">Backend sync</p>
            <h2>Self-hosted server</h2>
            <p className="settings-copy">
              Sync is off by default. When enabled, only v1 sleep observations and corrections are
              sent to your enrolled self-hosted backend. Overview and Rhythm clearly label synced
              server estimates.
            </p>
          </div>
          <div className="data-control-grid">
            <section
              className="data-control-card backend-sync-card"
              aria-labelledby="backend-sync-connect-title"
            >
              <div>
                <h3 id="backend-sync-connect-title">Connect backend</h3>
                <p>
                  Use an HTTPS URL and enrollment secret from your own server. The device token is
                  stored outside the editable config and is never shown here.
                </p>
              </div>
              <form
                className="backend-sync-form"
                onSubmit={(event) => {
                  event.preventDefault();
                  void handleConfigureBackendSync();
                }}
              >
                <label htmlFor="backend-sync-url">
                  Backend URL
                  <input
                    id="backend-sync-url"
                    type="url"
                    placeholder="https://zeitboard.example.com"
                    value={backendSyncForm.backendUrl}
                    onChange={(event) =>
                      setBackendSyncForm((form) => ({ ...form, backendUrl: event.target.value }))
                    }
                  />
                </label>
                <label htmlFor="backend-sync-secret">
                  Enrollment secret
                  <input
                    id="backend-sync-secret"
                    type="password"
                    value={backendSyncForm.enrollmentSecret}
                    onChange={(event) =>
                      setBackendSyncForm((form) => ({
                        ...form,
                        enrollmentSecret: event.target.value,
                      }))
                    }
                  />
                </label>
                <label htmlFor="backend-sync-label">
                  Device label
                  <input
                    id="backend-sync-label"
                    type="text"
                    value={backendSyncForm.deviceLabel}
                    onChange={(event) =>
                      setBackendSyncForm((form) => ({ ...form, deviceLabel: event.target.value }))
                    }
                  />
                </label>
                <label
                  className="toggle-row backend-sync-dev-toggle"
                  htmlFor="backend-sync-insecure"
                >
                  <span>
                    <strong>Allow self-signed localhost TLS</strong>
                    <small>Development only. Production sync verifies HTTPS certificates.</small>
                  </span>
                  <input
                    id="backend-sync-insecure"
                    type="checkbox"
                    checked={backendSyncForm.insecureSkipVerify}
                    onChange={(event) =>
                      setBackendSyncForm((form) => ({
                        ...form,
                        insecureSkipVerify: event.target.checked,
                      }))
                    }
                  />
                </label>
                <div className="backend-sync-actions">
                  <button
                    className="button primary"
                    type="submit"
                    disabled={backendSyncBusy || !backendSyncForm.backendUrl}
                  >
                    Enable backend sync
                  </button>
                  <button
                    className="button secondary"
                    type="button"
                    onClick={() => void handleDisableBackendSync()}
                    disabled={backendSyncBusy || !backendSyncStatus.enabled}
                  >
                    Disable sync
                  </button>
                </div>
              </form>
            </section>
            <section
              className="data-control-card backend-sync-card"
              aria-labelledby="backend-sync-status-title"
            >
              <div>
                <h3 id="backend-sync-status-title">Sync status</h3>
                <p>
                  Backend unavailable falls back to local estimates. Conflicts are reported here and
                  do not crash the app.
                </p>
              </div>
              <dl className="sync-status-list">
                <div>
                  <dt>Status</dt>
                  <dd>{backendSyncStatusLabel(backendSyncStatus)}</dd>
                </div>
                <div>
                  <dt>Backend</dt>
                  <dd>{backendSyncStatus.backendUrl || "Not configured"}</dd>
                </div>
                <div>
                  <dt>Device</dt>
                  <dd>{backendSyncStatus.deviceId || "Not enrolled"}</dd>
                </div>
                <div>
                  <dt>Pending push</dt>
                  <dd>{backendSyncStatus.pendingPushCount}</dd>
                </div>
                <div>
                  <dt>Last sync</dt>
                  <dd>{backendSyncStatus.lastSyncLabel}</dd>
                </div>
              </dl>
              <button
                className="button secondary"
                type="button"
                onClick={() => void handleSyncNow()}
                disabled={backendSyncBusy || !backendSyncStatus.enabled}
              >
                Sync now
              </button>
              {backendSyncStatus.lastError && (
                <p className="form-error">Last error: {backendSyncStatus.lastError}</p>
              )}
            </section>
          </div>
          {backendSyncError && (
            <p className="form-error" role="alert">
              {backendSyncError}
            </p>
          )}
          <p className="form-status" role="status" aria-live="polite">
            {backendSyncMessage}
          </p>
        </div>
        <div className="panel settings-section data-controls-panel">
          <div className="data-control-intro">
            <p className="section-kicker">Local data</p>
            <h2>Storage</h2>
            <p className="settings-copy">
              Export produces v1 JSON with observation-set and correction-set sections. Suppress
              appends a correction; erase permanently removes local observations and their
              correction history. If backend sync is on, erasure also propagates on the next sync:
              the server hard-deletes its copy and a tombstone tells your other devices to erase
              theirs — nothing can restore an erased record.
            </p>
          </div>
          <div className="data-control-grid">
            <section className="data-control-card" aria-labelledby="sleep-export-title">
              <div>
                <h3 id="sleep-export-title">Export sleep data</h3>
                <p>
                  Download a contract-shaped JSON file for backup, review, or later import tooling.
                </p>
              </div>
              <button
                className="button secondary"
                type="button"
                onClick={() => void handleExportSleepData()}
                disabled={dataControlBusy}
              >
                Export sleep data
              </button>
              {exportedSleepData && (
                <textarea
                  className="export-preview"
                  aria-label="Sleep data export JSON"
                  readOnly
                  rows={8}
                  value={exportedSleepData.json}
                />
              )}
            </section>
            <section className="data-control-card danger-zone" aria-labelledby="sleep-delete-title">
              <div>
                <h3 id="sleep-delete-title">Erase local sleep data</h3>
                <p>
                  This hard-deletes local sleep observations and sleep correction history. It is not
                  the append-only suppress action.
                </p>
              </div>
              <label htmlFor="delete-all-sleep-data">
                Type DELETE to erase all local sleep data
                <input
                  id="delete-all-sleep-data"
                  type="text"
                  value={deleteAllConfirmation}
                  onChange={(event) => setDeleteAllConfirmation(event.target.value)}
                />
              </label>
              <button
                className="button danger"
                type="button"
                onClick={() => void handleDeleteAllSleepData()}
                disabled={dataControlBusy || deleteAllConfirmation !== deleteConfirmationToken}
              >
                Erase all sleep data
              </button>
            </section>
          </div>
          {dataControlError && (
            <p className="form-error" role="alert">
              {dataControlError}
            </p>
          )}
          <p className="form-status" role="status" aria-live="polite">
            {dataControlStatus}
          </p>
        </div>
      </section>
    </>
  );
}

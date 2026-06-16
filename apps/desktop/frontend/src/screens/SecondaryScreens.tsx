import { useState, type KeyboardEvent } from "react";
import { Icon } from "../components/Icon";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";
import { useAppearanceContext } from "../theme/AppearanceProvider";
import type { ThemePreference } from "../theme/theme";
import {
  correctionPreviewFixture,
  proposalFixtures,
  refusalFixture,
  rhythmActogramFixture,
  rhythmDriftFixture,
  sourceConflictFixtures,
  unplacedTaskFixture,
  type ChangeProposalFixture,
  type ProposalOrigin,
  type RhythmDriftPointFixture,
  type RhythmSleepBandFixture,
  type SourceConflictFixture,
} from "../data/phaseTwo";
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
        <button className="button secondary" type="button">
          Reject proposal
        </button>
        <button className="button secondary" type="button">
          Modify
        </button>
        <button className="button primary" type="button">
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

function ActogramRow({ band }: { band: RhythmSleepBandFixture }) {
  const duplicateFits = band.startHour + 24 < DOUBLE_PLOT_HOURS;
  const showNowTick = band.day === rhythmActogramFixture.now.day;

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
            style={{ left: hourToPercent(rhythmActogramFixture.now.hour) }}
            aria-hidden="true"
          />
        )}
      </div>
      <small>{band.confidence}</small>
    </div>
  );
}

function ActogramPanel() {
  const [showForecast, setShowForecast] = useState(true);
  const forecastRows = showForecast ? rhythmActogramFixture.forecastRows : [];
  const allBands = [...rhythmActogramFixture.observedRows, ...forecastRows];

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

      <div className="actogram-chart" role="img" aria-label={rhythmActogramFixture.summary}>
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
          {rhythmActogramFixture.observedRows.map((band) => (
            <ActogramRow band={band} key={band.id} />
          ))}
          {showForecast && (
            <div className="actogram-now-line" aria-hidden="true">
              <span>{rhythmActogramFixture.now.label}</span>
            </div>
          )}
          {forecastRows.map((band) => (
            <ActogramRow band={band} key={band.id} />
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

function scaleDriftY(hour: number) {
  const { yMinHour, yMaxHour } = rhythmDriftFixture;
  const normalized = (hour - yMinHour) / (yMaxHour - yMinHour);
  return 90 - normalized * 72;
}

function DriftPanel() {
  const points = rhythmDriftFixture.points;
  const fitPoints = points
    .map((point, index) => `${scaleDriftX(index, points)},${scaleDriftY(point.fitHour)}`)
    .join(" ");
  const bandPoints = [
    ...points.map(
      (point, index) => `${scaleDriftX(index, points)},${scaleDriftY(point.bandHighHour)}`,
    ),
    ...[...points]
      .reverse()
      .map(
        (point, reverseIndex) =>
          `${scaleDriftX(points.length - 1 - reverseIndex, points)},${scaleDriftY(point.bandLowHour)}`,
      ),
  ].join(" ");
  const gridHours = [18, 20, 22, 24];

  return (
    <section className="panel drift-panel" aria-labelledby="drift-title">
      <div className="panel-heading">
        <div>
          <p className="section-kicker">Phase / drift</p>
          <h2 id="drift-title">{rhythmDriftFixture.title}</h2>
        </div>
        <div className="drift-summary">
          <strong>{rhythmDriftFixture.slopeLabel}</strong>
          <span>{rhythmDriftFixture.confidence} confidence</span>
        </div>
      </div>

      <div className="drift-body">
        <div className="drift-y-axis" aria-hidden="true">
          <span>12 AM</span>
          <span>10 PM</span>
          <span>8 PM</span>
          <span>6 PM</span>
        </div>
        <div className="drift-chart" role="img" aria-label={rhythmDriftFixture.summary}>
          <svg
            className="drift-svg"
            viewBox="0 0 100 100"
            preserveAspectRatio="none"
            aria-hidden="true"
          >
            {gridHours.map((hour) => (
              <line
                className="drift-gridline"
                x1="0"
                x2="100"
                y1={scaleDriftY(hour)}
                y2={scaleDriftY(hour)}
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
                cy={scaleDriftY(point.onsetHour)}
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
              <td>{rhythmDriftFixture.slopeLabel}</td>
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
          <button className="button primary" type="button">
            Add fixed event
          </button>
        }
      />
      <PlaceholderNotice>
        Placeholder schedule proposals use synthetic events. Fixed events remain unchanged.
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

const taskRows = [
  ["Draft project outline", "Deep focus", "60-90 min", "Today"],
  ["Review household paperwork", "Light focus", "30 min", "This cycle"],
  ["Call service provider", "Fixed hours", "20 min", "Tomorrow"],
  ["Organize reference notes", "Low demand", "45 min", "Flexible"],
];

export function TasksScreen() {
  return (
    <>
      <PageHeader
        title="Tasks"
        description="Describe flexibility and effort; the planner returns proposals, not calendar changes."
        actions={
          <button className="button primary" type="button">
            New task
          </button>
        }
      />
      <PlaceholderNotice>
        Task suggestions are synthetic. They create proposals only; no calendar or task change is
        applied automatically.
      </PlaceholderNotice>
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
            {proposalFixtures.length} pending proposals are waiting for explicit approval.
          </p>
          {proposalFixtures.slice(0, 1).map((proposal) => (
            <ProposalCard proposal={proposal} key={proposal.id} />
          ))}
        </section>

        <section className="panel table-panel task-list-panel">
          <div className="filter-row" aria-label="Task filters">
            <button className="filter active" type="button">
              Open 4
            </button>
            <button className="filter" type="button">
              Scheduled 2
            </button>
            <button className="filter" type="button">
              Done
            </button>
          </div>
          <div className="task-table" role="table" aria-label="Open tasks">
            <div className="task-row task-head" role="row">
              <span role="columnheader">Task</span>
              <span role="columnheader">Demand</span>
              <span role="columnheader">Duration</span>
              <span role="columnheader">Timing</span>
            </div>
            {taskRows.map(([task, demand, duration, timing]) => (
              <div className="task-row" role="row" key={task}>
                <span role="cell">
                  <span className="empty-check" aria-hidden="true" />
                  {task}
                </span>
                <span role="cell">{demand}</span>
                <span role="cell">{duration}</span>
                <span role="cell">
                  <span className="task-chip">{timing}</span>
                </span>
              </div>
            ))}
          </div>
        </section>

        <aside className="panel unplaced-panel" aria-labelledby="unplaced-title">
          <p className="section-kicker">Not proposed</p>
          <h2 id="unplaced-title">{unplacedTaskFixture.title}</h2>
          <p>{unplacedTaskFixture.reason}</p>
          <small>{unplacedTaskFixture.nextAction}</small>
        </aside>
      </section>
    </>
  );
}

export function ApprovalsScreen() {
  return (
    <>
      <PageHeader
        title="Approvals"
        description={`${proposalFixtures.length} pending proposals. Review, modify, approve, or reject each change before anything moves.`}
        actions={
          <button className="button secondary" type="button">
            Approve low-risk
          </button>
        }
      />
      <PlaceholderNotice>
        Approving here would create an audited change with undo. Fixed and imported events remain
        immutable.
      </PlaceholderNotice>
      <section className="screen-grid approval-screen" aria-label="Pending approval queue">
        <div className="panel approval-filter" aria-label="Approval filters">
          <button className="filter active" type="button">
            All {proposalFixtures.length}
          </button>
          <button className="filter" type="button">
            Scheduler 1
          </button>
          <button className="filter" type="button">
            Assistant 1
          </button>
          <button className="filter" type="button">
            Sync 0
          </button>
          <button className="filter sort-filter" type="button">
            Sort by expiry
          </button>
        </div>
        <div className="proposal-stack">
          {proposalFixtures.map((proposal) => (
            <ProposalCard proposal={proposal} key={proposal.id} />
          ))}
        </div>
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

export function RhythmScreen() {
  const [tab, setTab] = useState<RhythmTab>("actogram");

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
        description="Inspect synthetic sleep-wake observations, correction history, and estimate uncertainty."
      />
      <PlaceholderNotice>
        This read-only preview distinguishes imported, estimated, corrected, and incomplete
        observations.
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
            <ActogramPanel />
          </div>
        )}

        {tab === "drift" && (
          <div
            className="rhythm-panel"
            role="tabpanel"
            id="rhythm-panel-drift"
            aria-labelledby="rhythm-tab-drift"
          >
            <DriftPanel />
          </div>
        )}

        {tab === "sources" && (
          <div
            className="rhythm-panel"
            role="tabpanel"
            id="rhythm-panel-sources"
            aria-labelledby="rhythm-tab-sources"
          >
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
          <button className="button primary" type="button">
            Add record
          </button>
        }
      />
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
          <button className="button primary" type="button">
            New profile
          </button>
        }
      />
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

export function DataSourcesScreen() {
  return (
    <>
      <PageHeader
        title="Data Sources"
        description="Review local inputs and the boundary around each permission."
      />
      <section className="screen-grid data-source-screen" aria-label="Data source review">
        <section className="panel source-conflicts-panel" aria-labelledby="data-conflicts-title">
          <div className="panel-heading">
            <div>
              <p className="section-kicker">Estimate inputs</p>
              <h2 id="data-conflicts-title">Source conflicts and missingness</h2>
            </div>
          </div>
          <SourceConflictList
            conflicts={sourceConflictFixtures}
            labelledBy="data-conflicts-title"
          />
        </section>
        <section className="panel source-list source-status-panel" aria-label="Data source status">
          <article>
            <div className="source-mark">
              <Icon name="clock" />
            </div>
            <div>
              <h2>Manual sleep log</h2>
              <p>3 entries this week - last entry today</p>
            </div>
            <span className="source-status connected">Available</span>
            <button className="button secondary" type="button">
              Manage
            </button>
          </article>
          <article>
            <div className="source-mark">
              <Icon name="calendar" />
            </div>
            <div>
              <h2>Local calendar</h2>
              <p>Fixed event times only - private titles stay local</p>
            </div>
            <span className="source-status connected">Connected</span>
            <button className="button secondary" type="button">
              Manage
            </button>
          </article>
          <article>
            <div className="source-mark">
              <Icon name="sources" />
            </div>
            <div>
              <h2>Device activity</h2>
              <p>Not connected - optional supporting observation</p>
            </div>
            <span className="source-status">Off</span>
            <button className="button secondary" type="button">
              Set up
            </button>
          </article>
        </section>
      </section>
    </>
  );
}

export function SettingsScreen() {
  const { theme, reducedStimulation, setTheme, setReducedStimulation } = useAppearanceContext();

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
            Time format
            <select defaultValue="12-hour">
              <option value="12-hour">12-hour</option>
              <option value="24-hour">24-hour</option>
            </select>
          </label>
          <label>
            Week starts
            <select defaultValue="monday">
              <option value="monday">Monday</option>
              <option value="sunday">Sunday</option>
            </select>
          </label>
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
          <label className="toggle-row">
            <span>
              <strong>Show confidence reasons</strong>
              <small>Explain why an estimate is high, moderate, or low.</small>
            </span>
            <input type="checkbox" defaultChecked />
          </label>
          <label className="toggle-row">
            <span>
              <strong>Show predicted ranges</strong>
              <small>Use windows rather than single-point times.</small>
            </span>
            <input type="checkbox" defaultChecked />
          </label>
        </div>
        <div className="panel settings-section">
          <div>
            <p className="section-kicker">Local data</p>
            <h2>Storage</h2>
          </div>
          <p className="settings-copy">
            All application data is stored on this device. Export and deletion controls will be
            connected to the desktop service.
          </p>
          <button className="button secondary" type="button">
            Open data controls
          </button>
        </div>
      </section>
    </>
  );
}

import { Icon } from "../components/Icon";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";

const days = ["Mon 15", "Tue 16", "Wed 17", "Thu 18", "Fri 19"];

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
        Task suggestions are synthetic and demonstrate scheduling constraints only.
      </PlaceholderNotice>
      <section className="panel table-panel">
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
    </>
  );
}

const timelineRows = [
  { day: "Jun 15", start: 21, width: 31, quality: "Complete" },
  { day: "Jun 14", start: 18, width: 32, quality: "Complete" },
  { day: "Jun 13", start: 15, width: 29, quality: "Estimated" },
  { day: "Jun 12", start: 12, width: 34, quality: "Complete" },
  { day: "Jun 11", start: 9, width: 31, quality: "Incomplete" },
  { day: "Jun 10", start: 6, width: 33, quality: "Complete" },
];

export function TimelineScreen() {
  return (
    <>
      <PageHeader
        title="Timeline"
        description="Inspect synthetic sleep-wake observations, corrections, and estimate uncertainty."
      />
      <PlaceholderNotice>
        This preview distinguishes imported, estimated, and incomplete observations.
      </PlaceholderNotice>
      <section className="panel timeline-panel" aria-labelledby="observation-title">
        <div className="panel-heading">
          <div>
            <p className="section-kicker">Last six cycles</p>
            <h2 id="observation-title">Sleep observations</h2>
          </div>
          <span className="legend">
            <i /> Observed sleep
          </span>
        </div>
        <div className="timeline-axis" aria-hidden="true">
          <span>12 AM</span>
          <span>6 AM</span>
          <span>12 PM</span>
          <span>6 PM</span>
          <span>12 AM</span>
        </div>
        <div className="actogram">
          {timelineRows.map((row) => (
            <div className="actogram-row" key={row.day}>
              <time>{row.day}</time>
              <div className="actogram-track">
                <span
                  style={{ left: `${row.start}%`, width: `${row.width}%` }}
                  data-quality={row.quality}
                />
              </div>
              <small>{row.quality}</small>
            </div>
          ))}
        </div>
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
      <section className="panel source-list" aria-label="Data source status">
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
    </>
  );
}

export function SettingsScreen() {
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

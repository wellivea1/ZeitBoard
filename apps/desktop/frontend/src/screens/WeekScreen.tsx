import { useEffect, useMemo, useState } from "react";
import { PlaceholderNotice } from "../components/AppShell";
import { Icon } from "../components/Icon";
import { WeekBoard } from "../components/WeekBoard";
import { EventInspector, EventTable, SuggestionInspector } from "../components/WeekDetails";
import { weekColumns, weekTitle } from "../components/weekLayout";
import { loadOverview } from "../data/backend";
import {
  addCivilDays,
  calendarDataChangedEvent,
  downloadCalendarExport,
  exportOwnedCalendar,
  loadCalendar,
  todayCivilDate,
  type CalendarData,
  type CalendarEventSegment,
} from "../data/calendar";
import {
  loadMedications,
  medicationDataChangedEvent,
  type MedicationsData,
} from "../data/medications";
import type { OverviewData } from "../data/overview";
import { loadSleepEntries, type SleepEntry } from "../data/sleepEntries";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import { useApprovals } from "../state/approvals";
import { localZone } from "../utils/civilTime";

// Plan › Week: the calendar-native view, for arranging things rather than
// glancing at them. Home says when sleep is likely in a sentence; here the
// same forecast is painted into the days, with every event, accepted time,
// suggestion and dose on one clock, so a clash is something you see. Adding
// and removing calendars lives in Data Sources with the other inputs.

type DaysShown = 1 | 4 | 7;

const ranges: { days: DaysShown; label: string }[] = [
  { days: 1, label: "Day" },
  { days: 4, label: "4 days" },
  { days: 7, label: "Week" },
];

const hourHeight = 40;
const storageKey = "zeitboard.week.days";

function storedDays(): DaysShown {
  try {
    const value = Number(window.localStorage.getItem(storageKey));
    return value === 1 || value === 7 ? value : 4;
  } catch {
    return 4;
  }
}

function useDaysShown() {
  const [days, setDays] = useState<DaysShown>(storedDays);
  const choose = (value: DaysShown) => {
    setDays(value);
    try {
      window.localStorage.setItem(storageKey, String(value));
    } catch {
      // A per-viewer convenience; losing it only resets the range.
    }
  };
  return [days, choose] as const;
}

function useNow() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 60_000);
    return () => window.clearInterval(timer);
  }, []);
  return now;
}

/** Reloads `load` now and whenever one of `events` fires. */
function useReloading<T>(load: () => Promise<T>, events: string[], initial: T) {
  const [value, setValue] = useState<T>(initial);
  const key = events.join(" ");
  useEffect(() => {
    let current = true;
    const run = () =>
      void load().then(
        (result) => {
          if (current) setValue(result);
        },
        () => undefined,
      );
    run();
    for (const name of key.split(" ")) window.addEventListener(name, run);
    return () => {
      current = false;
      for (const name of key.split(" ")) window.removeEventListener(name, run);
    };
  }, [load, key]);
  return value;
}

const loadSleep = () => loadSleepEntries().then((data) => data.entries);
const loadDoses = () => loadMedications();
const loadFreshness = () => loadOverview().then((result) => result.data);

function useCalendarRange(start: string, days: number, zoneId: string) {
  const [state, setState] = useState<{
    data: CalendarData | null;
    source: "local" | "fixture";
    error: string;
    loading: boolean;
  }>({ data: null, source: "local", error: "", loading: true });
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    const onChanged = () => setRevision((value) => value + 1);
    window.addEventListener(calendarDataChangedEvent, onChanged);
    window.addEventListener(sleepDataChangedEvent, onChanged);
    return () => {
      window.removeEventListener(calendarDataChangedEvent, onChanged);
      window.removeEventListener(sleepDataChangedEvent, onChanged);
    };
  }, []);

  useEffect(() => {
    let current = true;
    void loadCalendar({ startCivilDate: start, days, zoneId }).then(
      (result) => {
        if (current)
          setState({ data: result.data, source: result.source, error: "", loading: false });
      },
      (reason: unknown) => {
        if (current)
          setState((previous) => ({
            ...previous,
            loading: false,
            error: reason instanceof Error ? reason.message : "The calendar could not be read.",
          }));
      },
    );
    return () => {
      current = false;
    };
  }, [start, days, zoneId, revision]);

  return state;
}

function sourceSummary(data: CalendarData) {
  const events = new Set(data.days.flatMap((day) => day.events.map((event) => event.eventId)));
  const count = `${events.size} ${events.size === 1 ? "event" : "events"}`;
  if (data.sources.length === 0) return `${count}; no calendars added yet`;
  return `${count} from ${data.sources.map((source) => source.label).join(", ")}`;
}

function shortWeekday(civilDate: string) {
  return new Date(`${civilDate}T12:00:00`).toLocaleDateString(undefined, { weekday: "short" });
}

function WeekToolbar({
  start,
  days,
  today,
  onStart,
  onDays,
}: {
  start: string;
  days: DaysShown;
  today: string;
  onStart: (value: string) => void;
  onDays: (value: DaysShown) => void;
}) {
  const last = addCivilDays(start, days - 1);
  const span = days === 1 ? "day" : `${days} days`;
  return (
    <header className="week-toolbar">
      <div className="week-title">
        <h2 id="week-title">{weekTitle(start, last)}</h2>
        {days > 1 && (
          <span>
            {shortWeekday(start)} – {shortWeekday(last)}
          </span>
        )}
      </div>
      <div className="week-nav" role="group" aria-label="Dates">
        <button
          className="icon-button week-step"
          data-direction="back"
          type="button"
          aria-label={`The ${span} before`}
          onClick={() => onStart(addCivilDays(start, -days))}
        >
          <Icon name="chevron" />
        </button>
        <button
          className="button compact"
          type="button"
          disabled={start === today}
          onClick={() => onStart(today)}
        >
          Today
        </button>
        <button
          className="icon-button week-step"
          type="button"
          aria-label={`The ${span} after`}
          onClick={() => onStart(addCivilDays(start, days))}
        >
          <Icon name="chevron" />
        </button>
        <input
          type="date"
          aria-label="First day shown"
          value={start}
          onChange={(event) => {
            if (event.currentTarget.value) onStart(event.currentTarget.value);
          }}
        />
      </div>
      <div className="week-range" role="group" aria-label="Days shown">
        {ranges.map((range) => (
          <button
            className={`filter${days === range.days ? " active" : ""}`}
            type="button"
            aria-pressed={days === range.days}
            onClick={() => onDays(range.days)}
            key={range.days}
          >
            {range.label}
          </button>
        ))}
      </div>
      <a className="button primary compact week-add" href="#/plan/tasks">
        Add a task
      </a>
    </header>
  );
}

/** What the board cannot show by itself: errors, gaps, and a stale estimate. */
function WeekNotes({
  data,
  error,
  overview,
}: {
  data: CalendarData | null;
  error: string;
  overview: OverviewData | null;
}) {
  // Home withholds its figure when the records are too old to anchor a
  // forecast. The calendar still draws the last estimate, so it says so.
  const stale =
    data?.status === "estimated" && overview?.status === "estimated" && !overview.freshness.trusted;
  return (
    <>
      {error && (
        <p className="week-error" role="alert">
          {error}
        </p>
      )}
      {data?.warnings.map((warning) => (
        <p className="week-warning" key={warning}>
          {warning}
        </p>
      ))}
      {stale && (
        <p className="week-warning">
          {overview.freshness.explanation.replace(/\.?$/, ".")} The bands below come from the last
          estimate and may have drifted since.
        </p>
      )}
      {data && data.status !== "estimated" && (
        <p className="week-warning">
          No sleep forecast to draw yet. <a href="#/log/sleep">Log sleep</a> and it appears here.
        </p>
      )}
    </>
  );
}

function WeekFoot({
  data,
  canExport,
  unplaced,
}: {
  data: CalendarData | null;
  canExport: boolean;
  unplaced: number;
}) {
  const [exporting, setExporting] = useState(false);
  const [notice, setNotice] = useState("");
  const exportAccepted = () => {
    if (!canExport || exporting) return;
    setExporting(true);
    void exportOwnedCalendar().then(
      (result) => {
        setExporting(false);
        const saved = downloadCalendarExport(result);
        const times = result.eventCount === 1 ? "time" : "times";
        setNotice(
          `${result.eventCount} accepted ${times} exported${saved ? ` to ${result.fileName}` : "."}`,
        );
      },
      (reason: unknown) => {
        setExporting(false);
        setNotice(reason instanceof Error ? reason.message : "The export failed.");
      },
    );
  };
  return (
    <footer className="week-foot">
      <p className="week-legend" aria-label="Legend">
        <span data-key="asleep">Likely asleep</span>
        <span data-key="uncertain">The model cannot say</span>
        <span data-key="recorded">Recorded sleep</span>
        <span data-key="event">Event</span>
        <span data-key="accepted">Accepted time</span>
        <span data-key="suggested">Suggested, not yet accepted</span>
      </p>
      <p className="week-sources">
        {data && <span>{sourceSummary(data)}.</span>}
        <a href="#/data-sources">Manage calendars</a>
        <button
          className="text-link"
          type="button"
          disabled={!canExport || exporting}
          title="Save the times you accepted as an .ics file for another calendar app"
          onClick={exportAccepted}
        >
          {exporting ? "Exporting…" : "Export accepted times"}
        </button>
      </p>
      {unplaced > 0 && (
        <p className="week-unplaced">
          {unplaced === 1
            ? "One suggestion has no exact time yet"
            : `${unplaced} suggestions have no exact time yet`}
          ; <a href="#/plan/tasks">decide {unplaced === 1 ? "it" : "them"} in Tasks</a>.
        </p>
      )}
      <p className="week-notice" role="status" aria-live="polite">
        {notice}
      </p>
    </footer>
  );
}

export function WeekScreen() {
  const zoneId = localZone() || "America/New_York";
  const today = todayCivilDate(zoneId);
  const [days, setDays] = useDaysShown();
  const [start, setStart] = useState(today);
  const calendar = useCalendarRange(start, days, zoneId);
  const sleep = useReloading(loadSleep, [sleepDataChangedEvent], [] as SleepEntry[]);
  const medications = useReloading<MedicationsData | null>(
    loadDoses,
    [medicationDataChangedEvent],
    null,
  );
  const overview = useReloading<OverviewData | null>(loadFreshness, [sleepDataChangedEvent], null);
  const approvals = useApprovals();
  const now = useNow();
  const [selected, setSelected] = useState<CalendarEventSegment | null>(null);
  const [inspecting, setInspecting] = useState<string | null>(null);

  const data = calendar.data;
  const columns = useMemo(
    () =>
      data
        ? weekColumns({ calendar: data, suggestions: approvals.pending, sleep, medications, now })
        : [],
    [data, approvals.pending, sleep, medications, now],
  );
  const unplaced = approvals.pending.filter((proposal) => !proposal.startAt || !proposal.endAt);
  const suggestion = approvals.pending.find((proposal) => proposal.id === inspecting);
  const event = selected
    ? data?.days
        .flatMap((day) => day.events)
        .find((candidate) => candidate.segmentId === selected.segmentId)
    : undefined;

  return (
    <section className="week" aria-labelledby="week-title">
      {calendar.source === "fixture" && (
        <PlaceholderNotice>
          Sample calendar. Nothing is read from your files or written anywhere.
        </PlaceholderNotice>
      )}
      <WeekToolbar start={start} days={days} today={today} onStart={setStart} onDays={setDays} />
      <WeekNotes data={data} error={calendar.error} overview={overview} />
      {data ? (
        <WeekBoard
          columns={columns}
          hour={hourHeight}
          deciding={approvals.busyProposalId !== null}
          onSelect={(next) => {
            setInspecting(null);
            setSelected(next);
          }}
          onInspect={(id) => {
            setSelected(null);
            setInspecting(id);
          }}
          onDecide={approvals.decide}
        />
      ) : (
        <p className="week-loading" role="status">
          {calendar.loading ? "Reading the calendar…" : "The calendar is not available."}
        </p>
      )}
      <WeekFoot data={data} canExport={calendar.source === "local"} unplaced={unplaced.length} />
      {event && <EventInspector event={event} onClose={() => setSelected(null)} />}
      {suggestion && (
        <SuggestionInspector
          proposal={suggestion}
          deciding={approvals.busyProposalId !== null}
          onDecide={(decision) => approvals.decide(suggestion.id, decision)}
          onClose={() => setInspecting(null)}
        />
      )}
      {data && <EventTable data={data} />}
    </section>
  );
}

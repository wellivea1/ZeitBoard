import { Fragment, useEffect, useState } from "react";
import { Diary, Doses, NeedsYou } from "../components/HomeColumns";
import { OutlookPanel } from "../components/OutlookPanel";
import { QuickLogBar } from "../components/QuickLogBar";
import { loadOverview } from "../data/backend";
import {
  calendarDataChangedEvent,
  loadCalendar,
  todayCivilDate,
  type CalendarDay,
} from "../data/calendar";
import { outlookFixture, overviewFixture } from "../data/fixture";
import { diaryDays, leadParts, stateTone } from "../data/homeLead";
import {
  hasLocalMedicationService,
  loadMedications,
  logMedicationEvent,
  medicationDataChangedEvent,
  notifyMedicationDataChanged,
  type MedicationEventInput,
  type MedicationsData,
} from "../data/medications";
import { loadOutlook, outlookUnavailable } from "../data/outlook";
import { overviewUnavailable } from "../data/overview";
import { sleepDataChangedEvent, notifySleepDataChanged } from "../data/sleepDataEvents";
import { hasDesktopBridge } from "../data/wailsBridge";
import { localZone } from "../utils/civilTime";
import { createCoalescedRefresh } from "../utils/coalescedRefresh";
import { subscribeProjectionRefresh } from "../utils/projectionRefresh";
import type { ConfidenceLevel, OverviewSource, OverviewData } from "../data/overview";

// Home, set as an almanac page (ui-refactor-plan.md §14). It leads with a
// sentence, not a status tile: how long you have been awake, when sleep is
// likely to begin, when you will probably wake. Under it the next three days
// as a figure, then three columns: what is waiting on you, what is in the
// diary, and the doses to record. The state tile, the next-sleep panel and the
// "Coming up" list it replaces each restated part of that sentence.

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

// The diary reads the calendar itself: events stay listed while the forecast
// is withheld, and the times accepted in Plan are calendar events too.
function useHomeCalendar() {
  const [days, setDays] = useState<CalendarDay[]>([]);
  useEffect(() => {
    let current = true;
    const load = () => {
      const zoneId = localZone() || "America/New_York";
      void loadCalendar({ startCivilDate: todayCivilDate(zoneId), days: 4, zoneId }).then(
        (result) => {
          if (current) setDays(result.data.days);
        },
        () => undefined,
      );
    };
    load();
    window.addEventListener(sleepDataChangedEvent, load);
    window.addEventListener(calendarDataChangedEvent, load);
    return () => {
      current = false;
      window.removeEventListener(sleepDataChangedEvent, load);
      window.removeEventListener(calendarDataChangedEvent, load);
    };
  }, []);
  return days;
}

function useHomeMedications() {
  const [data, setData] = useState<MedicationsData | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let current = true;
    const load = () =>
      void loadMedications().then(
        (loaded) => {
          if (current) setData(loaded);
        },
        () => {
          if (current) setData(null);
        },
      );
    load();
    window.addEventListener(medicationDataChangedEvent, load);
    return () => {
      current = false;
      window.removeEventListener(medicationDataChangedEvent, load);
    };
  }, []);
  const log = async (input: MedicationEventInput) => {
    setBusy(true);
    try {
      setData(await logMedicationEvent(input));
      notifyMedicationDataChanged();
    } finally {
      setBusy(false);
    }
  };
  const available = hasLocalMedicationService() && data !== null && data.status !== "unavailable";
  return { data, busy, available, log };
}

// The contract's wording is complete; a footer only needs the number.
function compactDrift(label: string) {
  return label.replace(/ minutes per observed sleep cycle$/, " min per cycle");
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
  const calendar = useHomeCalendar();
  const medications = useHomeMedications();
  const hasEstimate = overview.status === "estimated";
  const tone = stateTone(overview.state);
  const parts = loading ? [{ text: "Reading your records…" }] : leadParts(overview, outlook);

  return (
    <div className="almanac">
      <h1 className="sr-only">Home</h1>
      <section className="home-now" data-state={tone} aria-label="Now">
        <div className="home-lead">
          <p className="lead" aria-live="polite">
            {parts.map((part, index) =>
              "strong" in part && part.strong ? (
                <strong key={index}>{part.text}</strong>
              ) : (
                <Fragment key={index}>{part.text}</Fragment>
              ),
            )}
          </p>
          <p className="lead-meta">
            <span className="sync-dot" data-mode={mode} aria-hidden="true" />
            <span>{sourceLabel(mode, hasEstimate)}</span>
            {hasEstimate && overview.freshness.trusted && (
              <span>{overview.freshness.ageLabel}</span>
            )}
            {hasEstimate && <span>Drift {compactDrift(overview.drift.label)}</span>}
            {hasEstimate && <a href="#/rhythm">How this is worked out</a>}
          </p>
        </div>
        <aside className="home-record" aria-label="Record">
          <span className="section-kicker">Record</span>
          <QuickLogBar />
        </aside>
      </section>

      {hasEstimate ? (
        <>
          <OutlookPanel data={outlook} />
          <section className="home-columns" aria-label="Today and the days ahead">
            <NeedsYou />
            <Diary days={diaryDays(calendar, outlook, medications.data)} />
            <Doses
              data={medications.data}
              busy={medications.busy}
              available={medications.available}
              onLog={medications.log}
            />
          </section>
          <footer className="home-footer">
            <span>Estimated from your sleep records, not a measurement of circadian phase.</span>
            <ModelConfidence
              level={overview.confidence.level}
              reason={overview.confidence.reason}
            />
            {overview.sharingStatus.active && (
              <a href="#/sharing">{overview.sharingStatus.label}</a>
            )}
            <span>Kept on this computer; synced only to your own server if you turn sync on.</span>
          </footer>
        </>
      ) : (
        !loading && <HomeRecovery overview={overview} />
      )}
    </div>
  );
}

function HomeRecovery({ overview }: { overview: OverviewData }) {
  const unavailable = overview.status === "unavailable";
  return (
    <section className="home-recovery" aria-labelledby="learning-title">
      <h2 id="learning-title" className="section-title">
        {unavailable ? "Your rhythm is not available yet" : "Still learning your rhythm"}
      </h2>
      <p>
        {unavailable
          ? "Your saved records have not been changed. This retries by itself, or try now."
          : `${overview.refusal?.message ?? overview.confidence.reason} Log sleep or import existing records to build a forecast.`}
      </p>
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

import { useEffect, useRef, useState } from "react";
import {
  activityState,
  activityStateLabels,
  loadActivityCollection,
  setActivityCollection,
  setPowerLogReading,
  deleteActivityData,
  saveActivityDataExport,
  type ActivityCollection,
} from "../../data/activityCollection";
import { timeZones } from "../../data/reachingHours";
import { createCoalescedRefresh, type CoalescedRefresh } from "../../utils/coalescedRefresh";

// The zone records are kept in, and the switch, with Retry once collection has
// stopped by itself. Shown only where the system can record.
function CollectionControls({
  status,
  zone,
  zones,
  busy,
  onZone,
  onSet,
}: {
  status: ActivityCollection | null;
  zone: string;
  zones: string[];
  busy: boolean;
  onZone: (zone: string) => void;
  onSet: (enabled: boolean, zoneId: string) => void;
}) {
  const canToggle = status !== null && (status.enabled || zone.length > 0);
  return (
    <>
      <label className="reaching-field">
        <span>Activity time zone</span>
        <input
          list="activity-zones"
          value={zone}
          disabled={busy || !status || status.enabled}
          onChange={(event) => onZone(event.target.value)}
        />
        <datalist id="activity-zones">
          {zones.map((item) => (
            <option key={item} value={item} />
          ))}
        </datalist>
      </label>
      <button
        className="button primary"
        type="button"
        disabled={busy || !canToggle}
        onClick={() => onSet(!status?.enabled, status?.enabled ? status.zoneId : zone)}
      >
        {status?.enabled ? "Turn off activity collection" : "Enable local activity collection"}
      </button>
      {status?.enabled && !status.running && (
        <button
          type="button"
          className="button secondary"
          disabled={busy}
          onClick={() => onSet(true, status.zoneId)}
        >
          Retry collection
        </button>
      )}
    </>
  );
}

// The records kept so far: exported to a private file, or deleted with the
// word typed back, which also turns collection off.
function ActivityRecordsCard({
  status,
  busy,
  confirmation,
  onConfirmation,
  onExport,
  onDelete,
}: {
  status: ActivityCollection | null;
  busy: boolean;
  confirmation: string;
  onConfirmation: (value: string) => void;
  onExport: () => void;
  onDelete: () => void;
}) {
  return (
    <section className="data-control-card" aria-label="Activity data controls">
      <h3>Saved activity records</h3>
      <p>
        Turning collection off keeps previous records. Deleting also turns collection off. Exported
        files contain exact timestamps; keep them private.
      </p>
      <button
        type="button"
        className="button secondary"
        disabled={busy || !status || status.recordCount === 0}
        onClick={onExport}
      >
        Export activity records
      </button>
      <label className="reaching-field">
        <span>Type DELETE to delete activity records</span>
        <input
          value={confirmation}
          disabled={busy || !status}
          autoComplete="off"
          onChange={(event) => onConfirmation(event.target.value)}
        />
      </label>
      <button
        type="button"
        className="button secondary"
        disabled={busy || !status || confirmation !== "DELETE"}
        onClick={onDelete}
      >
        Delete activity records
      </button>
    </section>
  );
}

// Windows' own log of sleep, wake, shutdown and start (ADR-0060). What it
// reads is said before it is chosen, and it reads only while desktop activity
// is on.
function PowerLogCard({
  status,
  busy,
  onToggle,
}: {
  status: ActivityCollection;
  busy: boolean;
  onToggle: (enabled: boolean) => void;
}) {
  const { powerLog } = status;
  return (
    <section className="data-control-card" aria-label="Sleep and wake from Windows">
      <h3>Sleep and wake from Windows</h3>
      <p>
        Windows notes when this computer goes to sleep, wakes, shuts down and starts, and keeps
        those notes for months. ZeitBoard can read those times and nothing else Windows logs,
        including for the hours it was closed and the months before it was installed.
      </p>
      <dl className="sync-status-list">
        <div>
          <dt>Reading</dt>
          <dd>{powerLog.enabled ? (status.enabled ? "On" : "Paused") : "Off"}</dd>
        </div>
        <div>
          <dt>Read from Windows</dt>
          <dd>
            {powerLog.entries > 0
              ? `${powerLog.entries} ${powerLog.entries === 1 ? "time" : "times"}, from ${powerLog.sinceLabel}`
              : "None yet"}
          </dd>
        </div>
      </dl>
      <button
        type="button"
        className="button secondary"
        disabled={busy || (!powerLog.enabled && !status.enabled)}
        onClick={() => onToggle(!powerLog.enabled)}
      >
        {powerLog.enabled ? "Stop reading from Windows" : "Read sleep and wake from Windows"}
      </button>
      {!status.enabled && (
        <p>
          {powerLog.enabled
            ? "Paused while desktop activity is off."
            : "Turn on desktop activity first."}
        </p>
      )}
      {powerLog.lastError && (
        <p className="form-error" role="alert">
          {powerLog.lastError}
        </p>
      )}
    </section>
  );
}

export function ActivityCollectionSettings() {
  const [status, setStatus] = useState<ActivityCollection | null>(null);
  const [zone, setZone] = useState("");
  const [zones] = useState(timeZones);
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const busyRef = useRef(false);
  const refreshRef = useRef<CoalescedRefresh | null>(null);

  useEffect(() => {
    const refresh = createCoalescedRefresh(
      loadActivityCollection,
      (value) => {
        setStatus(value);
        setZone((previous) => previous || value.zoneId);
        setError("");
      },
      (reason) => {
        setStatus(null);
        setError(reason instanceof Error ? reason.message : "Activity status could not be read.");
      },
    );
    refreshRef.current = refresh;
    const request = () => {
      if (!busyRef.current && document.visibilityState !== "hidden") refresh.request();
    };
    refresh.request();
    const interval = window.setInterval(request, 15_000);
    window.addEventListener("focus", request);
    document.addEventListener("visibilitychange", request);
    return () => {
      refresh.dispose();
      window.clearInterval(interval);
      window.removeEventListener("focus", request);
      document.removeEventListener("visibilitychange", request);
    };
  }, []);

  const run = async (action: () => Promise<ActivityCollection | string>) => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    refreshRef.current?.supersede();
    setMessage("");
    setError("");
    try {
      const result = await action();
      if (typeof result === "string") setMessage(result);
      else {
        setStatus(result);
        setZone(result.zoneId);
        setConfirmation("");
      }
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Activity settings could not be changed.",
      );
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
  };

  return (
    <section className="settings-section" aria-labelledby="activity-heading">
      <div className="data-control-intro">
        <h2 id="activity-heading">Desktop activity</h2>
        <p className="settings-copy">
          {status?.supported !== false && "Off unless you turn it on. "}Records only when this
          computer is active, idle or locked: never keystrokes, window titles, screenshots or
          browsing. It stays on this computer and does not change estimates or planning.
        </p>
        <details className="settings-more fold">
          <summary>When it runs</summary>
          <p className="settings-copy">
            Once on, it continues while ZeitBoard runs, with its window open or hidden, and resumes
            when ZeitBoard reopens; quitting stops it. It cannot collect while the computer is
            asleep or the app is closed, and gaps between polls are kept as possible suspend and
            resume evidence.
            {status?.powerLog.supported &&
              " With sleep and wake read from Windows, the times Windows noted while ZeitBoard was closed are read when it runs again."}
          </p>
        </details>
      </div>
      <div className="data-control-grid">
        <section className="data-control-card" aria-label="Activity consent">
          <dl className="sync-status-list">
            <div>
              <dt>Collection</dt>
              <dd>{activityStateLabels[activityState(status)]}</dd>
            </div>
            <div>
              <dt>Saved records</dt>
              <dd>{status?.recordCount ?? "Unavailable"}</dd>
            </div>
          </dl>
          {status && activityState(status) === "unavailable" ? (
            // Nothing here could turn it on, so nothing is offered but turning
            // off collection left on, as in a data folder brought from Windows.
            <>
              <p>ZeitBoard cannot record desktop activity on this system.</p>
              {status.enabled && (
                <button
                  className="button primary"
                  type="button"
                  disabled={busy}
                  onClick={() => void run(() => setActivityCollection(false, status.zoneId))}
                >
                  Turn off activity collection
                </button>
              )}
            </>
          ) : (
            <CollectionControls
              status={status}
              zone={zone}
              zones={zones}
              busy={busy}
              onZone={setZone}
              onSet={(enabled, zoneId) => void run(() => setActivityCollection(enabled, zoneId))}
            />
          )}
        </section>
        {status?.powerLog.supported && (
          <PowerLogCard
            status={status}
            busy={busy}
            onToggle={(enabled) => void run(() => setPowerLogReading(enabled))}
          />
        )}
        <ActivityRecordsCard
          status={status}
          busy={busy}
          confirmation={confirmation}
          onConfirmation={setConfirmation}
          onExport={() => void run(saveActivityDataExport)}
          onDelete={() => void run(() => deleteActivityData(confirmation))}
        />
      </div>
      {(error || status?.lastError) && (
        <p className="form-error" role="alert">
          {error || status?.lastError}
        </p>
      )}
      {message && <p role="status">{message}</p>}
      <button
        type="button"
        className="button secondary"
        disabled={busy}
        onClick={() => refreshRef.current?.request()}
      >
        Refresh activity status
      </button>
    </section>
  );
}

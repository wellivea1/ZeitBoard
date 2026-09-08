import { useEffect, useRef, useState } from "react";
import {
  loadActivityCollection,
  setActivityCollection,
  deleteActivityData,
  saveActivityDataExport,
  type ActivityCollection,
} from "../../data/activityCollection";
import { timeZones } from "../../data/reachingHours";
import { createCoalescedRefresh, type CoalescedRefresh } from "../../utils/coalescedRefresh";

function collectionLabel(status: ActivityCollection | null) {
  if (!status) return "Unavailable";
  if (status.running) return "Running";
  return status.enabled ? "Stopped" : "Off";
}

function canToggleCollection(status: ActivityCollection | null, zone: string) {
  return status !== null && (status.enabled || (status.supported && zone.length > 0));
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
        <p className="section-kicker">Optional local evidence</p>
        <h2 id="activity-heading">Desktop activity</h2>
        <p className="settings-copy">
          Save coarse active, idle, lock and app lifecycle transitions on this computer. No
          keystrokes, window titles, screenshots or browsing content are recorded. Activity stays
          local and does not currently change sleep estimates or planning.
        </p>
        <p className="settings-copy">
          Collection is off until you enable it. Once enabled, it continues when you hide or close
          the window and resumes when ZeitBoard reopens. Quit from the tray to stop it. ZeitBoard
          must be running; it cannot collect while the computer is asleep or the app is closed. Gaps
          between polls are recorded as possible suspend/resume evidence.
        </p>
      </div>
      <div className="data-control-grid">
        <section className="data-control-card" aria-label="Activity consent">
          <dl className="sync-status-list">
            <div>
              <dt>Collection</dt>
              <dd>{collectionLabel(status)}</dd>
            </div>
            <div>
              <dt>Saved records</dt>
              <dd>{status?.recordCount ?? "Unavailable"}</dd>
            </div>
          </dl>
          {status && !status.supported && (
            <p>This platform does not support activity collection.</p>
          )}
          <label className="reaching-field">
            <span>Activity time zone</span>
            <input
              list="activity-zones"
              value={zone}
              disabled={busy || !status || status.enabled}
              onChange={(event) => setZone(event.target.value)}
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
            disabled={busy || !canToggleCollection(status, zone)}
            onClick={() =>
              void run(() =>
                setActivityCollection(!status?.enabled, status?.enabled ? status.zoneId : zone),
              )
            }
          >
            {status?.enabled ? "Turn off activity collection" : "Enable local activity collection"}
          </button>
          {status?.enabled && !status.running && (
            <button
              type="button"
              className="button secondary"
              disabled={busy}
              onClick={() => void run(() => setActivityCollection(true, status.zoneId))}
            >
              Retry collection
            </button>
          )}
        </section>
        <section className="data-control-card" aria-label="Activity data controls">
          <h3>Saved activity records</h3>
          <p>
            Turning collection off keeps previous records. Erasing also turns collection off.
            Exported files contain exact timestamps; keep them private.
          </p>
          <button
            type="button"
            className="button secondary"
            disabled={busy || !status || status.recordCount === 0}
            onClick={() => void run(saveActivityDataExport)}
          >
            Export activity records
          </button>
          <label className="reaching-field">
            <span>Type DELETE to erase activity records</span>
            <input
              value={confirmation}
              disabled={busy || !status}
              autoComplete="off"
              onChange={(event) => setConfirmation(event.target.value)}
            />
          </label>
          <button
            type="button"
            className="button secondary"
            disabled={busy || !status || confirmation !== "DELETE"}
            onClick={() => void run(() => deleteActivityData(confirmation))}
          >
            Erase activity records
          </button>
        </section>
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

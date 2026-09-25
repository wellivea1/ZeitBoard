import { useEffect, useRef, useState } from "react";
import {
  loadStartupSettings,
  saveStartupSettings,
  hideDesktopWindow,
  quitDesktop,
  type StartupSettings as StartupStatus,
} from "../../data/startup";
import { createCoalescedRefresh, type CoalescedRefresh } from "../../utils/coalescedRefresh";

function registrationLabel(status: StartupStatus | null) {
  if (!status) return "Unavailable";
  if (!status.registered) return "Off";
  return status.matchesCurrent ? "Registered with Windows" : "Needs updating";
}

export function StartupSettings() {
  const [status, setStatus] = useState<StartupStatus | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [hidden, setHidden] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const busyRef = useRef(false);
  const dirty = useRef(false);
  const refreshRef = useRef<CoalescedRefresh | null>(null);

  const accept = (value: StartupStatus) => {
    setStatus(value);
    if (!dirty.current) {
      setEnabled(value.registered);
      setHidden(value.registered && value.matchesCurrent ? value.startHidden : true);
    }
  };

  useEffect(() => {
    const refresh = createCoalescedRefresh(
      loadStartupSettings,
      (value) => {
        accept(value);
        setError("");
      },
      (reason) => {
        setStatus(null);
        setError(reason instanceof Error ? reason.message : "Startup settings could not be read.");
      },
    );
    refreshRef.current = refresh;
    const request = () => {
      if (!busyRef.current && document.visibilityState !== "hidden") refresh.request();
    };
    refresh.request();
    const timer = window.setInterval(request, 15_000);
    window.addEventListener("focus", request);
    document.addEventListener("visibilitychange", request);
    return () => {
      refresh.dispose();
      window.clearInterval(timer);
      window.removeEventListener("focus", request);
      document.removeEventListener("visibilitychange", request);
    };
  }, []);

  const run = async (action: () => Promise<void>) => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setMessage("");
    refreshRef.current?.supersede();
    try {
      await action();
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "The desktop action could not be completed.",
      );
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
  };

  const save = async () => {
    const value = await saveStartupSettings(enabled, hidden);
    dirty.current = false;
    accept(value);
    setMessage(
      value.registered
        ? "Login startup registered. Your activity and sync consent settings are unchanged."
        : "Login startup removed. This session keeps running until you quit.",
    );
  };

  return (
    <section className="settings-section" aria-labelledby="startup-title">
      <div className="data-control-intro">
        <p className="section-kicker">App lifecycle</p>
        <h2 id="startup-title">Startup and background work</h2>
        <p className="settings-copy">
          Keep ZeitBoard available after signing in to Windows. Startup does not enable activity
          collection, sync or notifications; each keeps its own consent setting.
        </p>
      </div>
      <div className="data-control-grid">
        <section className="data-control-card" aria-label="Login startup">
          <h3>When you sign in</h3>
          <p>{registrationLabel(status)}</p>
          <label className="startup-toggle">
            <input
              type="checkbox"
              checked={enabled}
              disabled={busy || !status?.available}
              onChange={(event) => {
                dirty.current = true;
                setEnabled(event.target.checked);
              }}
            />
            <span>Start ZeitBoard when I sign in</span>
          </label>
          <label className="startup-toggle">
            <input
              type="checkbox"
              checked={hidden}
              disabled={busy || !status?.available || !enabled}
              onChange={(event) => {
                dirty.current = true;
                setHidden(event.target.checked);
              }}
            />
            <span>Start in the tray</span>
          </label>
          <p>
            Windows can delay or disable startup apps. If ZeitBoard does not open, check Windows
            Settings → Apps → Startup. Moving the executable requires saving this setting again.
          </p>
          <button
            type="button"
            className="button primary"
            disabled={busy || !status?.available}
            onClick={() => void run(save)}
          >
            Save startup settings
          </button>
        </section>
        <section className="data-control-card" aria-label="Current session">
          <h3>While ZeitBoard is running</h3>
          <p>
            Closing the window hides it when the tray is available. Enabled collection and sync
            continue. Reopen from the tray or launch ZeitBoard again. Quit stops background work
            until the next launch.
          </p>
          {status && !status.trayAvailable && (
            <p>The tray is unavailable. Closing the window will quit; hiding is disabled.</p>
          )}
          <button
            type="button"
            className="button secondary"
            disabled={busy || !status?.trayAvailable}
            onClick={() => void run(hideDesktopWindow)}
          >
            Hide to tray
          </button>
          <button
            type="button"
            className="button secondary"
            disabled={busy || !status}
            onClick={() => void run(quitDesktop)}
          >
            Quit ZeitBoard
          </button>
        </section>
      </div>
      {status?.message && <p className="settings-copy">{status.message}</p>}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {message && <p role="status">{message}</p>}
      <button
        type="button"
        className="button secondary"
        disabled={busy}
        onClick={() => refreshRef.current?.request()}
      >
        Refresh startup status
      </button>
    </section>
  );
}

import { useState } from "react";
import {
  loadReportedStatus,
  saveStatusSettings,
  statusChangedEvent,
  statusUnavailable,
  type ReportedStatus,
  type StatusSettings,
} from "../data/status";
import { useLoaded } from "../state/useLoaded";

// The owner's standing choices about a reported status (ADR-0070), set beside
// the links they govern. Both start off: a report stays on the owner's own
// devices, and the sleep buttons say nothing to anyone. What one link shows is
// chosen on that link; each report can still be kept back or shared. Both
// choices travel to the owner's other devices through their server, and the
// newest one made anywhere stands (ADR-0074).

export function StatusChoices() {
  const { data: loaded, set } = useLoaded<ReportedStatus>(loadReportedStatus, {
    events: [statusChangedEvent],
    fallback: () => statusUnavailable,
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  if (!loaded || loaded.status !== "ok") return null;

  const save = (settings: StatusSettings) => {
    setBusy(true);
    setError("");
    void saveStatusSettings(settings, loaded.settingsRevision).then(
      (result) => {
        set(result);
        setBusy(false);
      },
      (reason: unknown) => {
        setBusy(false);
        setError(reason instanceof Error ? reason.message : "Your choice could not be stored.");
      },
    );
  };

  return (
    <section className="status-choices sheet" aria-labelledby="status-choices-title">
      <h2 id="status-choices-title" className="section-title">
        The status you report
      </h2>
      <p className="status-choices-lead">
        On Home you can say you are awake, trying to sleep, or not to be disturbed. A link shows it
        only if you chose that for the link, and only a report you let links show.
      </p>
      <label className="status-choice">
        <input
          type="checkbox"
          checked={loaded.settings.shareByDefault}
          disabled={busy}
          onChange={(event) => save({ ...loaded.settings, shareByDefault: event.target.checked })}
        />
        <span>
          Let links show what I report, unless I untick it
          <small>
            Off, each report stays on your own devices unless you tick it at the time. Your phone
            follows this choice through your own server, and a change made there is followed here.
          </small>
        </span>
      </label>
      <label className="status-choice">
        <input
          type="checkbox"
          checked={loaded.settings.followSleepTaps}
          disabled={busy}
          onChange={(event) => save({ ...loaded.settings, followSleepTaps: event.target.checked })}
        />
        <span>
          Report it from the sleep buttons
          <small>
            “I am going to sleep” also reports “trying to sleep”, and “I woke up” reports “awake”.
            The night is recorded the same either way.
          </small>
        </span>
      </label>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}

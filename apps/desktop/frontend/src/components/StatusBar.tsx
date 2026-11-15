import { useState } from "react";
import {
  clearReportedStatus,
  deliveryWords,
  loadReportedStatus,
  reportableStatuses,
  reportStatus,
  statusChangedEvent,
  statusLifetimes,
  statusUnavailable,
  type ReportableStatus,
  type ReportedStatus,
} from "../data/status";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import { useLoaded } from "../state/useLoaded";

// What the owner says about now, beside the buttons that record sleep
// (ADR-0070). A report is the owner's word: it stands until the time shown or
// until it is cleared, and it moves no forecast. Whether a share link may show
// it is asked here each time, starting from the owner's usual choice.

export function StatusBar() {
  const { data: loaded, set } = useLoaded<ReportedStatus>(loadReportedStatus, {
    // The sleep buttons can report a status too, when the owner asked for it.
    events: [statusChangedEvent, sleepDataChangedEvent],
    // A status lapses by itself, so what stands is read again as time passes.
    expires: true,
    fallback: () => statusUnavailable,
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  // The owner's usual choice, until they tick or untick it for this report.
  const [shareThis, setShareThis] = useState<boolean | undefined>(undefined);
  // Zero is the status's own usual length.
  const [minutes, setMinutes] = useState(0);

  if (!loaded || loaded.status !== "ok") return null;
  const share = shareThis ?? loaded.settings.shareByDefault;

  const run = (action: () => Promise<ReportedStatus>) => {
    setBusy(true);
    setError("");
    void action().then(
      (result) => {
        set(result);
        setBusy(false);
        setShareThis(undefined);
        setMinutes(0);
      },
      (reason: unknown) => {
        setBusy(false);
        setError(reason instanceof Error ? reason.message : "That did not work.");
      },
    );
  };
  const report = (status: ReportableStatus) =>
    run(() => reportStatus({ status, shared: share, minutes }));

  return (
    <section className="status-bar" aria-label="Your status">
      <span className="section-kicker">Status</span>
      {loaded.reported ? (
        <p className="status-standing" role="status">
          <strong>{loaded.reportedLabel}</strong>
          <small>
            Said {loaded.saidLabel}, until {loaded.untilLabel}.{" "}
            {loaded.shared ? "Share links you allowed may show it." : "Kept to your own devices."}{" "}
            {deliveryWords(loaded)}
          </small>
        </p>
      ) : (
        <p className="status-standing">
          <small>Nothing reported. The forecast speaks for itself.</small>
        </p>
      )}
      <div className="status-actions">
        {reportableStatuses.map((item) => (
          <button
            className="button ghost"
            type="button"
            key={item.status}
            disabled={busy}
            aria-pressed={loaded.reported === item.status}
            onClick={() => report(item.status)}
          >
            {item.label}
          </button>
        ))}
        {loaded.reported && (
          <button
            className="button ghost"
            data-quiet
            type="button"
            disabled={busy}
            onClick={() => run(clearReportedStatus)}
          >
            Clear
          </button>
        )}
      </div>
      <label className="status-lasting">
        <span>The next one lasts</span>
        <select
          value={minutes}
          disabled={busy}
          onChange={(event) => setMinutes(Number(event.target.value))}
        >
          {statusLifetimes.map((item) => (
            <option key={item.minutes} value={item.minutes}>
              {item.label}
            </option>
          ))}
        </select>
      </label>
      <label className="status-share">
        <input
          type="checkbox"
          checked={share}
          disabled={busy}
          onChange={(event) => setShareThis(event.target.checked)}
        />
        <span>Let my share links show the next one</span>
      </label>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <a className="status-choices-link" href="#/sharing">
        Who sees it
      </a>
    </section>
  );
}

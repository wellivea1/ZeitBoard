import { useState } from "react";
import { removeCalendarSource, type CalendarSource } from "../data/calendar";

const kindLabels: Record<CalendarSource["kind"], string> = {
  ics: "ICS snapshot",
  caldav: "CalDAV snapshot",
  zeitboard: "App-owned",
};

export function CalendarSourcesPanel({
  sources,
  available,
  onChanged,
}: {
  sources: CalendarSource[];
  available: boolean;
  onChanged: () => void;
}) {
  const [removing, setRemoving] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const sourceBeingRemoved = sources.find((source) => source.sourceId === removing);

  const remove = (sourceId: string) => {
    if (confirmation !== "REMOVE" || busy) return;
    setBusy(true);
    setError("");
    void removeCalendarSource(sourceId).then(
      () => {
        setBusy(false);
        setRemoving(null);
        setConfirmation("");
        onChanged();
      },
      (reason: unknown) => {
        setBusy(false);
        setError(reason instanceof Error ? reason.message : "Calendar source removal failed.");
      },
    );
  };

  return (
    <section className="calendar-sources-panel" aria-labelledby="calendar-sources-title">
      <header>
        <p className="section-kicker">Local ownership</p>
        <h2 id="calendar-sources-title">Calendar sources</h2>
      </header>
      {sources.length === 0 ? (
        <p className="calendar-source-empty">No local calendar sources. Importing is optional.</p>
      ) : (
        <div className="calendar-source-list">
          {sources.map((source) => (
            <div className="calendar-source-row" data-kind={source.kind} key={source.sourceId}>
              <span className="calendar-source-mark" aria-hidden="true" />
              <div>
                <strong>{source.label}</strong>
                <span>
                  {kindLabels[source.kind]} - {source.visibleEvents} visible
                </span>
                <small>{source.coverageLabel}</small>
                {source.endpoint && (
                  <small className="calendar-source-endpoint">{source.endpoint}</small>
                )}
              </div>
              {source.readOnly && available && (
                <button
                  className="text-button danger"
                  type="button"
                  onClick={() => {
                    setRemoving(source.sourceId);
                    setConfirmation("");
                    setError("");
                  }}
                >
                  Remove
                </button>
              )}
            </div>
          ))}
        </div>
      )}
      {sourceBeingRemoved && (
        <div className="calendar-remove-confirmation">
          <label>
            <span>Type REMOVE to erase this imported snapshot</span>
            <input
              value={confirmation}
              autoComplete="off"
              disabled={busy}
              onChange={(event) => setConfirmation(event.currentTarget.value)}
            />
          </label>
          <div>
            <button
              className="button ghost compact"
              type="button"
              disabled={busy}
              onClick={() => {
                setRemoving(null);
                setConfirmation("");
              }}
            >
              Cancel
            </button>
            <button
              className="button danger compact"
              type="button"
              disabled={busy || confirmation !== "REMOVE"}
              onClick={() => remove(sourceBeingRemoved.sourceId)}
            >
              {busy ? "Erasing..." : "Erase source"}
            </button>
          </div>
        </div>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <p className="calendar-ownership-note">
        Imported events are immutable. Approvals write only to ZeitBoard placements.
      </p>
    </section>
  );
}

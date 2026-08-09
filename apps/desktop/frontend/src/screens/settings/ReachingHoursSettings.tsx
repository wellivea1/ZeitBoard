import { useEffect, useState } from "react";
import {
  WEEKDAY_LABELS,
  loadReachingHours,
  reachingHoursUnavailable,
  saveReachingHours,
  timeZones,
  type ReachingHours,
  type ReachingHoursEnvelope,
} from "../../data/reachingHours";

// The hours belong to whoever the person needs to reach, not to them. Everything
// on this panel is worded that way, because the difference is the whole point:
// a drifting rhythm makes "when is the clinic open, and will I be awake" a
// different question every day.

const RUN_PRESETS: { label: string; days: number[] }[] = [
  { label: "Weekdays", days: [1, 2, 3, 4, 5] },
  { label: "Every day", days: [0, 1, 2, 3, 4, 5, 6] },
  { label: "Weekends", days: [0, 6] },
];

function sameDays(a: number[], b: number[]) {
  return a.length === b.length && a.every((day, index) => day === b[index]);
}

export function ReachingHoursSettings() {
  const [envelope, setEnvelope] = useState<ReachingHoursEnvelope>(reachingHoursUnavailable);
  const [draft, setDraft] = useState<ReachingHours>(reachingHoursUnavailable.state);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [zones] = useState(timeZones);

  useEffect(() => {
    let current = true;
    void loadReachingHours().then((loaded) => {
      if (!current) return;
      setEnvelope(loaded);
      setDraft(loaded.state);
    });
    return () => {
      current = false;
    };
  }, []);

  const change = (changes: Partial<ReachingHours>) => {
    setDraft((state) => ({ ...state, ...changes }));
    setMessage("");
  };

  const toggleDay = (day: number) =>
    change({
      days: draft.days.includes(day)
        ? draft.days.filter((value) => value !== day)
        : [...draft.days, day].sort((a, b) => a - b),
    });

  const submit = async () => {
    setBusy(true);
    setError("");
    try {
      const saved = await saveReachingHours(draft, envelope.revision);
      setEnvelope(saved);
      setDraft(saved.state);
      setMessage(
        saved.conflict
          ? "These hours were changed elsewhere, so that edit was kept and yours was not applied."
          : "Saved. The next three days now use these hours.",
      );
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The schedule could not be saved.");
    } finally {
      setBusy(false);
    }
  };

  const overnight = draft.endLocal <= draft.startLocal;

  return (
    <section className="settings-section reaching-panel">
      <div className="data-control-intro">
        <p className="section-kicker">Reaching people</p>
        <h2>When the people you need are available</h2>
        <p className="settings-copy">
          These are someone else&apos;s hours, not yours &mdash; a clinic, a pharmacy, an employer,
          family in another country. The next three days use them to work out how much of your
          predicted waking time overlaps. Nothing here is shared or sent anywhere.
        </p>
      </div>

      <div className="reaching-form">
        <label className="reaching-toggle">
          <input
            type="checkbox"
            checked={draft.enabled}
            onChange={(event) => change({ enabled: event.target.checked })}
          />
          <span>
            Show reaching hours
            <small>
              Turn this off if there is nobody you need to reach on a schedule. The outlook then
              says nothing rather than assuming a working week.
            </small>
          </span>
        </label>

        <label className="reaching-field">
          <span>Whose hours are these?</span>
          <input
            type="text"
            value={draft.label}
            maxLength={60}
            placeholder="The clinic"
            disabled={!draft.enabled}
            onChange={(event) => change({ label: event.target.value })}
          />
        </label>

        <div className="reaching-clocks">
          <label className="reaching-field">
            <span>Opens</span>
            <input
              type="time"
              value={draft.startLocal}
              disabled={!draft.enabled}
              onChange={(event) => change({ startLocal: event.target.value })}
            />
          </label>
          <label className="reaching-field">
            <span>Closes</span>
            <input
              type="time"
              value={draft.endLocal}
              disabled={!draft.enabled}
              onChange={(event) => change({ endLocal: event.target.value })}
            />
          </label>
        </div>
        {draft.enabled && overnight && (
          <p className="reaching-note">
            {draft.startLocal === draft.endLocal
              ? "Open all day, every day you choose below."
              : "Closes the next morning, so this counts as one overnight stretch."}
          </p>
        )}

        <fieldset className="reaching-days" disabled={!draft.enabled}>
          <legend>Open on</legend>
          <div className="reaching-day-buttons">
            {WEEKDAY_LABELS.map((name, day) => (
              <label key={name}>
                <input
                  type="checkbox"
                  checked={draft.days.includes(day)}
                  onChange={() => toggleDay(day)}
                />
                <span>{name.slice(0, 3)}</span>
              </label>
            ))}
          </div>
          <div className="reaching-presets">
            {RUN_PRESETS.map((preset) => (
              <button
                key={preset.label}
                type="button"
                className="button secondary compact"
                aria-pressed={sameDays(draft.days, preset.days)}
                onClick={() => change({ days: preset.days })}
              >
                {preset.label}
              </button>
            ))}
          </div>
        </fieldset>

        <label className="reaching-field">
          <span>
            Their time zone
            <small>Set this to theirs, not yours, if they are somewhere else.</small>
          </span>
          <select
            value={draft.zoneId}
            disabled={!draft.enabled}
            onChange={(event) => change({ zoneId: event.target.value })}
          >
            {!zones.includes(draft.zoneId) && draft.zoneId && (
              <option value={draft.zoneId}>{draft.zoneId}</option>
            )}
            {zones.map((zone) => (
              <option key={zone} value={zone}>
                {zone}
              </option>
            ))}
          </select>
        </label>

        <p className="reaching-summary">{envelope.summary}</p>

        <div className="reaching-actions">
          <button className="button primary" type="button" disabled={busy} onClick={() => submit()}>
            Save reaching hours
          </button>
          <button
            className="button secondary"
            type="button"
            disabled={busy}
            onClick={() => {
              setDraft(envelope.state);
              setMessage("");
              setError("");
            }}
          >
            Discard changes
          </button>
        </div>

        {message && (
          <p className="settings-copy" role="status">
            {message}
          </p>
        )}
        {(error || envelope.message) && (
          <p className="form-error" role="alert">
            {error || envelope.message}
          </p>
        )}
      </div>
    </section>
  );
}

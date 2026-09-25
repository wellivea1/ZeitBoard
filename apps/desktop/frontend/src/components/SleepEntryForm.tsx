import type { SleepEntryInput, SleepClassification } from "../data/sleepEntries";

export function SleepEntryForm({
  form,
  onChange,
  submitLabel,
  disabled,
  editing = false,
  excluded = false,
  onExcludedChange,
  onCancel,
}: {
  form: SleepEntryInput;
  onChange: (form: SleepEntryInput) => void;
  submitLabel: string;
  disabled: boolean;
  editing?: boolean;
  excluded?: boolean;
  onExcludedChange?: (value: boolean) => void;
  onCancel?: () => void;
}) {
  return (
    <div className="sleep-entry-fields">
      <label>
        Sleep start
        <input
          type={editing ? "text" : "datetime-local"}
          value={form.startLocal}
          disabled={disabled}
          onChange={(event) => onChange({ ...form, startLocal: event.target.value })}
          required
        />
      </label>
      <label>
        Wake time
        <input
          type={editing ? "text" : "datetime-local"}
          value={form.endLocal}
          disabled={disabled}
          onChange={(event) => onChange({ ...form, endLocal: event.target.value })}
          required
        />
      </label>
      <label>
        Time zone
        <input
          type="text"
          value={form.zoneId}
          disabled={disabled || editing}
          onChange={(event) => onChange({ ...form, zoneId: event.target.value })}
          required
        />
      </label>
      <label>
        Kind
        <select
          value={form.classification}
          disabled={disabled}
          onChange={(event) =>
            onChange({ ...form, classification: event.target.value as SleepClassification })
          }
        >
          <option value="principal">Main sleep</option>
          <option value="nap">Nap</option>
          <option value="unknown">Not sure</option>
        </select>
      </label>
      {editing && (
        <>
          <p className="sleep-entry-hint">
            Times include their UTC offset, for example 2026-11-01T01:30:00-04:00. Keep the offset
            valid for the selected zone.
          </p>
          <label className="sleep-entry-check">
            <input
              type="checkbox"
              checked={excluded}
              disabled={disabled}
              onChange={(event) => onExcludedChange?.(event.target.checked)}
            />
            Exclude from estimates
          </label>
        </>
      )}
      <div className="sleep-entry-submit">
        <button className="button primary" type="submit" disabled={disabled}>
          {submitLabel}
        </button>
        {onCancel && (
          <button className="button ghost" type="button" onClick={onCancel} disabled={disabled}>
            Cancel
          </button>
        )}
      </div>
    </div>
  );
}

import type { SleepClassification } from "../data/sleepEntries";
import { localZone, zoneWords } from "../utils/civilTime";
import {
  occurrences,
  skippedTime,
  wallClock,
  type SleepForm,
  type SleepTimeField,
} from "../utils/sleepForm";

// One form for a night added and a night corrected: the same date-and-time
// pickers, in the night's own zone. A correction once opened its times as
// RFC 3339 text, "2026-10-25T14:00:00-04:00", to tell apart the hour that
// happens twice when the clocks go back. Now only that hour asks.

function TimeField({
  label,
  field,
  zone,
  disabled,
  onChange,
}: {
  label: string;
  field: SleepTimeField;
  zone: string;
  disabled: boolean;
  onChange: (field: SleepTimeField) => void;
}) {
  const candidates = occurrences(field, zone);
  return (
    <div className="sleep-time-field">
      <label>
        {label}
        <input
          type="datetime-local"
          value={field.wall}
          disabled={disabled}
          onChange={(event) => onChange({ wall: event.target.value })}
          required
        />
      </label>
      {candidates && candidates.length > 1 && (
        <label>
          Which {wallClock(field.wall)}?
          <select
            value={field.at ?? ""}
            disabled={disabled}
            onChange={(event) => onChange({ ...field, at: event.target.value || undefined })}
          >
            <option value="">Choose one</option>
            {candidates.map((candidate, index) => (
              <option key={candidate.instant} value={candidate.instant}>
                {index === 0 ? "The first" : "The second"}, {candidate.label}
              </option>
            ))}
          </select>
        </label>
      )}
      {candidates && candidates.length === 0 && (
        <small className="sleep-time-note" role="alert">
          {skippedTime(field.wall, zone)}
        </small>
      )}
    </div>
  );
}

export function SleepEntryForm({
  form,
  onChange,
  submitLabel,
  disabled,
  editing = false,
  excluded = false,
  onExcludedChange,
  onCancel,
  error = "",
}: {
  form: SleepForm;
  onChange: (form: SleepForm) => void;
  submitLabel: string;
  disabled: boolean;
  editing?: boolean;
  excluded?: boolean;
  onExcludedChange?: (value: boolean) => void;
  onCancel?: () => void;
  /** Why the form was not saved, said beside it. */
  error?: string;
}) {
  return (
    <div className="sleep-entry-fields">
      <TimeField
        label="Sleep start"
        field={form.start}
        zone={form.zoneId}
        disabled={disabled}
        onChange={(start) => onChange({ ...form, start })}
      />
      <TimeField
        label="Wake time"
        field={form.end}
        zone={form.zoneId}
        disabled={disabled}
        onChange={(end) => onChange({ ...form, end })}
      />
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
          {form.zoneId !== localZone() && (
            <p className="sleep-entry-hint">
              In {zoneWords(form.zoneId)}, where the night was recorded.
            </p>
          )}
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
      {error && (
        <p className="form-error sleep-entry-error" role="alert">
          {error}
        </p>
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

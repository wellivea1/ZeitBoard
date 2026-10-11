import {
  WEEKDAY_LABELS,
  WEEK_ORDER,
  dayHoursNote,
  daySpans,
  withDayHours,
  withDayOpen,
  type ReachingContact,
} from "../../data/reachingHours";
import { axisHour } from "../../utils/relativeTime";

// One person on the reaching list: who they are, their zone, and their hours on
// each day of the week. Each day carries a small 24-hour bar, so a week of
// hours reads as a shape at a glance, and a closing time typed as 05:00 instead
// of 17:00 shows as the overnight stretch it would be before anything is saved.

interface ReachingContactEditorProps {
  contact: ReachingContact;
  /** Its place in the list, from 1: the name of someone not yet named. */
  position: number;
  zones: string[];
  canMoveUp: boolean;
  onChange: (contact: ReachingContact) => void;
  onMoveUp: () => void;
  onRemove: () => void;
}

const AXIS_HOURS = [0, 6, 12, 18, 24];

export function ReachingContactEditor({
  contact,
  position,
  zones,
  canMoveUp,
  onChange,
  onMoveUp,
  onRemove,
}: ReachingContactEditorProps) {
  const name = contact.label.trim() || `Person ${position}`;
  return (
    <fieldset className="reaching-person" data-off={contact.off || undefined}>
      <legend className="sr-only">{name}</legend>
      <div className="reaching-person-head">
        <label className="reaching-field reaching-name">
          <span>Name</span>
          <input
            type="text"
            value={contact.label}
            maxLength={60}
            placeholder="The clinic"
            onChange={(event) => onChange({ ...contact, label: event.target.value })}
          />
        </label>
        <label className="reaching-field">
          <span>Their time zone</span>
          <select
            value={contact.zoneId}
            onChange={(event) => onChange({ ...contact, zoneId: event.target.value })}
          >
            {!zones.includes(contact.zoneId) && contact.zoneId && (
              <option value={contact.zoneId}>{contact.zoneId}</option>
            )}
            {zones.map((zone) => (
              <option key={zone} value={zone}>
                {zone}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className="reaching-week" role="group" aria-label={`When ${name} is available`}>
        {WEEK_ORDER.map((day) => (
          <ReachingDay key={day} contact={contact} day={day} onChange={onChange} />
        ))}
        <div className="reaching-week-axis" aria-hidden="true">
          <span className="reaching-week-scale">
            {AXIS_HOURS.map((hour) => (
              <small key={hour} style={{ left: `${(hour / 24) * 100}%` }}>
                {axisHour(hour)}
              </small>
            ))}
          </span>
        </div>
      </div>

      <div className="reaching-person-foot">
        <label className="reaching-toggle">
          <input
            type="checkbox"
            checked={!contact.off}
            onChange={(event) => onChange({ ...contact, off: !event.target.checked })}
          />
          <span>
            Show on Home
            <small>Off keeps these hours on the list without drawing a row for them.</small>
          </span>
        </label>
        <span className="reaching-person-actions">
          {canMoveUp && (
            <button type="button" className="button ghost compact" onClick={onMoveUp}>
              Move up
            </button>
          )}
          <button type="button" className="button ghost compact" onClick={onRemove}>
            Remove
          </button>
        </span>
      </div>
    </fieldset>
  );
}

function ReachingDay({
  contact,
  day,
  onChange,
}: {
  contact: ReachingContact;
  day: number;
  onChange: (contact: ReachingContact) => void;
}) {
  const name = WEEKDAY_LABELS[day];
  const hours = contact.hours.find((entry) => entry.day === day);
  const note = hours ? dayHoursNote(hours) : undefined;
  return (
    <div className="reaching-day" data-open={hours ? true : undefined}>
      <label className="reaching-day-name">
        <input
          type="checkbox"
          checked={hours !== undefined}
          onChange={(event) => onChange(withDayOpen(contact, day, event.target.checked))}
        />
        <span>{name}</span>
      </label>
      {hours ? (
        <span className="reaching-day-clocks">
          <input
            type="time"
            aria-label={`${name} opens`}
            value={hours.startLocal}
            onChange={(event) =>
              onChange(withDayHours(contact, day, { startLocal: event.target.value }))
            }
          />
          <span aria-hidden="true">to</span>
          <input
            type="time"
            aria-label={`${name} closes`}
            value={hours.endLocal}
            onChange={(event) =>
              onChange(withDayHours(contact, day, { endLocal: event.target.value }))
            }
          />
          {note && <small>{note}</small>}
        </span>
      ) : (
        <small className="reaching-day-closed">Closed</small>
      )}
      <span className="reaching-day-track" aria-hidden="true">
        {daySpans(contact.hours, day).map((span) => (
          <span
            key={span.carried ? "carried" : "own"}
            data-carried={span.carried || undefined}
            style={{ left: `${span.start * 100}%`, width: `${(span.end - span.start) * 100}%` }}
          />
        ))}
      </span>
    </div>
  );
}

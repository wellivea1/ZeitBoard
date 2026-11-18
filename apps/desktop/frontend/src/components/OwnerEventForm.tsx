import { useEffect, useRef, useState, type FormEvent } from "react";
import { TimeField } from "./SleepEntryForm";
import {
  addOwnerEvent,
  changeOwnerEvent,
  ownerEventTitleLimit,
  type OwnerEvent,
} from "../data/ownerEvents";
import { civilMinute, localZone, zonedWall, zoneWords } from "../utils/civilTime";
import {
  ownerEventInput,
  ownerEventTimes,
  sleepWindowOver,
  type SleepWindow,
} from "../utils/ownerEventForm";
import { relativeRange } from "../utils/relativeTime";
import type { SleepTimeField } from "../utils/sleepForm";

// An event the owner adds (ADR-0068): a title, a start and an end, with the
// same date-and-time pickers as a night. It is a fixed commitment, so the form
// says at once when it falls in a predicted sleep window: that is the
// appointment a person with a moving rhythm most needs to see.

interface EventForm {
  title: string;
  start: SleepTimeField;
  end: SleepTimeField;
}

const HALF_HOUR = 30 * 60_000;

/** The next half hour from now, for an hour: where a new event starts out. */
function startingForm(startWall: string | undefined, zone: string): EventForm {
  const start = startWall ?? civilMinute(Math.ceil(Date.now() / HALF_HOUR) * HALF_HOUR, zone);
  return { title: "", start: { wall: start }, end: { wall: shiftWall(start, 60) } };
}

function storedForm(event: OwnerEvent): EventForm {
  const field = (value: string): SleepTimeField => {
    const { wall, at } = zonedWall(value, event.zoneId);
    return at ? { wall, at, exact: value } : { wall };
  };
  return { title: event.title, start: field(event.startLocal), end: field(event.endLocal) };
}

/** A wall-clock time moved by whole minutes, read as plain clock arithmetic. */
function shiftWall(wall: string, minutes: number): string {
  const naive = Date.parse(`${wall}:00Z`);
  return Number.isFinite(naive)
    ? new Date(naive + minutes * 60_000).toISOString().slice(0, 16)
    : "";
}

function wallMinutes(from: string, to: string): number {
  return (Date.parse(`${to}:00Z`) - Date.parse(`${from}:00Z`)) / 60_000;
}

export function OwnerEventForm({
  event,
  startWall,
  sleepWindows,
  onSaved,
  onCancel,
}: {
  /** The event being changed; absent for a new one. */
  event?: OwnerEvent;
  /** Where a new event starts out, as a wall-clock time in this zone. */
  startWall?: string;
  /** The predicted sleep windows in view, to say when the event falls in one. */
  sleepWindows: SleepWindow[];
  onSaved: (event: OwnerEvent, message: string) => void;
  onCancel: () => void;
}) {
  const zone = event?.zoneId ?? localZone();
  const [form, setForm] = useState<EventForm>(() =>
    event ? storedForm(event) : startingForm(startWall, zone),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const section = useRef<HTMLElement>(null);

  // Opened from a button that may be far up or down the page.
  useEffect(() => {
    section.current?.scrollIntoView?.({ block: "nearest" });
  }, []);

  // Moving the start keeps the event's length, as a calendar does.
  const changeStart = (start: SleepTimeField) => {
    const length = wallMinutes(form.start.wall, form.end.wall);
    const end =
      start.wall !== form.start.wall && Number.isFinite(length) && length > 0 && start.wall
        ? { wall: shiftWall(start.wall, length) }
        : form.end;
    setForm({ ...form, start, end });
  };

  // Said as soon as the times are in, before the event has a title.
  const times = ownerEventTimes(form, zone);
  const inSleep =
    "problem" in times
      ? undefined
      : sleepWindowOver(sleepWindows, times.startInstant, times.endInstant);

  const submit = (submitted: FormEvent) => {
    submitted.preventDefault();
    const ready = ownerEventInput(form, zone);
    if ("problem" in ready) {
      setError(ready.problem);
      return;
    }
    setBusy(true);
    setError("");
    const save = event
      ? changeOwnerEvent({ ...ready.input, eventId: event.eventId, revision: event.revision })
      : addOwnerEvent(ready.input);
    void save.then(
      (saved) => onSaved(saved, event ? `Changed ${saved.title}.` : `Added ${saved.title}.`),
      (reason: unknown) => {
        setBusy(false);
        setError(reason instanceof Error ? reason.message : "The event could not be saved.");
      },
    );
  };

  return (
    <section className="event-form" aria-labelledby="event-form-title" ref={section}>
      <h2 id="event-form-title" className="section-title">
        {event ? "Change the event" : "Add an event"}
      </h2>
      <form className="sleep-entry-fields sheet" onSubmit={submit}>
        <label className="event-form-title">
          What it is
          <input
            name="title"
            type="text"
            value={form.title}
            maxLength={ownerEventTitleLimit}
            autoFocus
            required
            disabled={busy}
            onChange={(change) => setForm({ ...form, title: change.target.value })}
          />
        </label>
        <TimeField
          label="Starts"
          field={form.start}
          zone={zone}
          disabled={busy}
          onChange={changeStart}
        />
        <TimeField
          label="Ends"
          field={form.end}
          zone={zone}
          disabled={busy}
          onChange={(end) => setForm({ ...form, end })}
        />
        {zone !== localZone() && (
          <p className="sleep-entry-hint">In {zoneWords(zone)}, where the event was entered.</p>
        )}
        {inSleep && (
          <p className="sleep-entry-hint event-form-sleep" role="status">
            This falls in a predicted sleep window,{" "}
            {relativeRange(new Date(inSleep.startAt), new Date(inSleep.endAt), new Date())}. The
            event is kept as entered.
          </p>
        )}
        {error && (
          <p className="form-error sleep-entry-error" role="alert">
            {error}
          </p>
        )}
        <div className="sleep-entry-submit">
          <button className="button primary" type="submit" disabled={busy}>
            {busy ? "Saving…" : event ? "Save changes" : "Add event"}
          </button>
          <button className="button ghost" type="button" disabled={busy} onClick={onCancel}>
            Cancel
          </button>
        </div>
      </form>
    </section>
  );
}

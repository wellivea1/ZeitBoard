# ADR 0068: Events the owner adds, mirrored everywhere

- Status: accepted
- Date: 2026-10-28
- Owner direction: "Events should be mirrored everywhere."
- Builds on [ADR-0017](0017-server-erasure-tombstones.md),
  [ADR-0020](0020-task-sync-revisions.md),
  [ADR-0023](0023-local-calendar-ownership-and-adapters.md),
  [ADR-0047](0047-accepted-times-sync-as-placements.md),
  [ADR-0053](0053-accepted-times-written-to-a-caldav-calendar.md) and
  [ADR-0065](0065-accepted-times-written-to-google-and-microsoft-calendars.md).

## Context

The ease-of-use review of 2026-10-28 found that no event can be entered in ZeitBoard. A dentist
on Thursday at 3 PM is what a person with Non-24 most needs to see against predicted sleep. The
app can show it, mark it when it falls in predicted sleep, and plan around it, but only if it
arrives from another calendar. Giving a task a fixed window does not stand in for one: the planner
never places a task in predicted sleep, so the appointment that most needs seeing is the one it
refuses.

Asked whether events stay on the computer or go further, the owner chose: everywhere.

## Decision

1. **The owner adds, changes and deletes events on the computer.**
   - An event has a title of 1 to 120 characters, a start and an end within one day of each
     other, and the zone it was entered in.
   - It is added from the week board and from Home's "In the diary", beside "Add a task", and
     changed or deleted from the event itself.
2. **An event is a fixed commitment, like a calendar's.**
   - ZeitBoard keeps the events in `local_owner_events` and mirrors each into
     `local_calendar_events` under its own source, "Your events" (`calendar_source_owner_events`,
     kind `zeitboard`).
   - Every reader treats it as busy time it may not move: the planner, the week board, Home's
     diary and its predicted-sleep warning, and the assistant's snapshot.
   - No existing table changes. An edit replaces the mirrored row, as imported rows are never
     changed in place.
3. **It syncs to the owner's server as revisions (ADR-0020's model).**
   - Each change is a new immutable record of kind `event`, with id `<event_id>_r<revision>`,
     carrying the whole event.
   - Every device keeps the highest revision. Two different revisions of the same number are
     settled the same way everywhere, by the later `updated_at` and then the record's bytes, so
     all devices agree.
   - Deleting erases every pushed revision through the erase endpoint, and the server and every
     device refuse the event's return. Titles are private text, as task titles are.
4. **It reaches the owner's calendar.** When writing is on (ADR-0053, ADR-0065), an event is
   written there as accepted times are. A change replaces it, and deletion removes it.
5. **It reaches the phone.** The companion receives events, shows them with the day's plans, and
   erases them when they are erased.

## Consequences

- An appointment entered once is seen on every surface the owner uses, and planned around.
- The server learns the titles and times of events, encrypted at rest on the owner's own
  instance. Trusted links never show them, and models never read their titles.
- Events are added on the computer. Adding them on the phone can come later through the same
  record kind.
- The work lands in three parts: the computer and server, then the owner's calendar, then the
  phone.

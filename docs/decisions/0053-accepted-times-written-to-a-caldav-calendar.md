# ADR 0053: Accepted times can be written to the owner's CalDAV calendar

- Status: accepted
- Date: 2026-09-27
- Implements C2's "an explicitly enabled CalDAV write-back path handles only approved app-owned
  blocks, with revision/conflict checks, idempotent retry and safe undo; imported fixed events
  cannot be silently moved".
- Builds on [ADR-0047](0047-accepted-times-sync-as-placements.md) (accepted times as
  placements) and [ADR-0052](0052-reviewed-batch-decisions.md) (decisions made together).

## Context

A time the owner accepts becomes an app-owned block in ZeitBoard's own placements calendar. The
owner could export those blocks as an ICS file, but the calendar they actually live by (on their
phone, shared with others) never saw them unless they imported that file by hand, and an undo
never reached it.

Writing to someone's calendar is the most trusted thing ZeitBoard could do there. Two failures
matter most. ZeitBoard must never change or delete an event the owner made. And it must never
remove an event of its own that the owner has since changed, for instance by moving it.

## Decision

1. **Off until turned on, for one CalDAV calendar.** Data Sources › Calendars offers "Write
   accepted times here" on an imported CalDAV calendar. It says what will happen and asks for a
   sign-in, which ZeitBoard checks before keeping. A PROPFIND on the collection must show a
   calendar that holds events and a sign-in that may add them (`write`, `write-content`, `bind`
   or `all`), where the server reports those properties. The sign-in is kept in the
   owner-protected local database, the same place and protection as the backend credential. It
   is never exported or synced, and stopping erases it. Every other calendar is only read.
2. **Triggers queue, a worker writes.** The store queues a write whenever an app-owned block
   appears (a create) or goes (a delete, if it was written; a create not yet written is simply
   dropped). Triggers do this, so every path is covered without code of its own: accepting,
   deciding together, undoing. A worker on the desktop carries the queue to the calendar
   every minute, and at once after a decision or undo.
3. **ZeitBoard's own events only, named by their placement.** An accepted time is written as a
   calendar object whose UID is `<event id>@zeitboard.local`, the UID the ICS export already
   used, at `<collection>/<UID>.ics`. Only a resource directly in the collection, at a name
   ZeitBoard made, is ever removed.
4. **Preconditions instead of trust.**
   - A create is `PUT` with `If-None-Match: *`: it cannot overwrite anything. If something is
     already there and carries this time's UID, it is an earlier attempt whose answer was lost,
     and ZeitBoard keeps it. If it carries another UID, the owner is asked.
   - A removal is `DELETE` with `If-Match` on the ETag ZeitBoard was given. If the owner changed
     the event, the removal fails, and the event stays until the owner chooses "Remove it
     anyway" or "Keep it in the calendar".
   - A weak ETag is not kept, because `If-Match` never matches one.
   - A write that a redirect turned into a read is a failure, not a success.
5. **Failures retry, conflicts wait.** A failed write is retried after 1 minute, then 5 and 30
   minutes, then every 2 hours, and "Try again now" retries at once. A conflict is never retried
   on its own. Each problem is listed with the time it concerns, what happened, and when the next
   attempt is.
6. **Reading back is not double counting.** Importing or refreshing a calendar leaves out
   ZeitBoard's own events: those it holds, and those it is still removing. Each is already on the
   board, and kept as an imported busy event it would clash with the very placement it copies.
   An empty CalDAV calendar, such as one made for ZeitBoard, can now be imported.
7. **Stopping forgets and leaves.** Stopping writing, or removing the calendar from ZeitBoard,
   turns writing off and erases the sign-in. What was written stays in the
   owner's calendar, and ZeitBoard no longer removes it on undo. The confirmation says so.

## Consequences

- Accepted times appear wherever the owner's calendar does, and undoing one removes it. Imported
  events are never changed. That was enforced in the database before, and now also on the
  server, by name and by precondition.
- An accepted time written while its answer was lost is not written twice. A time undone while
  it was being written is removed once the write lands.
- A server that rewrites what it stores gives no ETag on `PUT`. ZeitBoard then reads the ETag
  back, so the owner's later changes are still protected.
- The sign-in is a credential kept unencrypted, restricted to the owner's account (see the
  threat model). ZeitBoard asks for an app password where the provider offers one.
- One calendar at a time. Writing to another one means stopping first, which keeps "where do my
  accepted times go?" a one-word answer.

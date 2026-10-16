# ADR 0065: Accepted times written to a Google or Microsoft calendar

- Status: accepted
- Date: 2026-10-12
- Completes the planned "OAuth calendar providers" with writing. Extends
  [ADR-0053](0053-accepted-times-written-to-a-caldav-calendar.md)'s write-back from CalDAV to a calendar
  brought in from an account through [ADR-0064](0064-calendar-accounts-through-the-owners-registration.md),
  with the same guarantees.

## Context

ADR-0053 writes the times the owner accepts to one CalDAV calendar of theirs, and removes them on
undo. Google Calendar and Outlook speak CalDAV poorly or not at all to third parties, so their
owners could not have their accepted times there. ADR-0064 signs in to those accounts to read
them, asking only to read.

Writing is the most trusted thing ZeitBoard does in someone's calendar. ADR-0053's two rules
carry over: never change or delete an event the owner made, and never remove one of ZeitBoard's
events that the owner has since changed. Each provider offers what those rules need:
- **Google:** an event ID the client chooses, refused if taken, and `If-Match` on delete, which
  fails with 412 once the event changed.
- **Microsoft:** a client transaction ID that stops a retried create from making a second event,
  and a change key that versions each event.

## Decision

1. **The same offer, on the account calendar's row.** "Write accepted times here" appears on a
   calendar brought in from an account, as on a CalDAV calendar. One calendar takes accepted
   times at a time, either kind. It says what will happen before anything is asked for.
2. **Asked for once, in the browser, and only what writing needs.**
   - Reading never asked to add events. Turning writing on signs the account in again to ask,
     with the registration it was kept with.
   - Google's scope is `calendar.events.owned`: events on calendars the owner owns, nothing else.
   - Microsoft's is `Calendars.ReadWrite`, which has no narrower form.
   - A permission left unticked, or a sign-in to a different account, turns nothing on and says
     why.
   - What a sign-in states is what is kept. Signing in again asks for what is in use: adding
     events only while accepted times are written to the account. Google's
     `include_granted_scopes` keeps an earlier grant on its side.
   - A sign-in from the form that would take adding events from the account written to is
     refused, and its kept sign-in stays.
   - Turning writing on can be cancelled at any point, whether or not the browser was asked.
     Waiting for the browser stops after five minutes.
3. **Only a calendar ZeitBoard may add to.** In Google, a calendar the owner owns. In Microsoft,
   one the owner can edit. A calendar shared with them is refused, by name.
4. **ZeitBoard's own events only, recognisable as such.**
   - Each acceptance of a time is written as an event of its own, named by the time and the
     approval that accepted it. Retrying a write keeps that name, as does writing to the same
     calendar again after stopping; accepting the time again after undoing it makes a new one.
   - In Google, the event's ID is derived from that name (base32hex, as Google requires). It
     carries a private mark naming the time, and another naming the time of writing, from turning
     writing on to stopping, that wrote it: an opaque value, compared only for equality.
   - In Microsoft, it carries ZeitBoard's UID for the time, with the approval, as its transaction
     ID.
   - Events are written without reminders, as CalDAV writes are.
   - A removal checks the event carries this time's mark or UID, else leaves it.
   - Reading the calendar back maps both to ZeitBoard's UID, so an import or refresh recognises
     them, as ADR-0053 does for CalDAV.
5. **Preconditions instead of trust.**
   - A Google create that finds its ID taken keeps the event only if it carries this time's mark;
     anything else is a conflict for the owner. Google keeps a removed event under its ID.
     ZeitBoard's own event written in an earlier time of writing, and removed while writing was
     off, is put back when writing is turned on again, as turning writing on writes every
     accepted time. One written in this time of writing and removed since was removed by the
     owner, and stays removed, as it would had the create's answer come back.
   - A Microsoft create first looks, over every page of the time's events, for what an earlier
     attempt wrote, its answer lost or the app quitting first. Graph also drops a create that
     repeats a transaction ID, which alone guards a time holding more events than ZeitBoard
     reads.
   - A time with no approval on record cannot be named, so it waits for the owner as a
     conflict.
   - A Google removal first checks the event still carries this time's mark, then sends
     `If-Match` with the version ZeitBoard was given. "Remove it anyway" sends none.
   - A Microsoft removal compares the change key first.
   - A changed event waits for the owner, as in ADR-0053. IDs are asked for as immutable, so an
     event the owner moves between calendars is still found.
6. **The writer and its queue are ADR-0053's.** The same triggers queue writes, and the same
   retries, conflicts, "Try again now", status and problem list apply. When the account cannot
   be signed in to, every due write waits and says why, such as an ended sign-in.
   - A time accepted again while its earlier acceptance is still being removed is written once
     that removal is done. If the owner keeps the changed event instead, that event stands for the
     time, at the version ZeitBoard wrote, so removing it later still asks first.
   - A create that lands after its acceptance was undone is removed, and a time accepted again
     meanwhile is written after. Whatever becomes of a create, written, refused or failed, is
     recorded only for the acceptance it carried.
   - A pass ends where writing stops or moves to another calendar, and what it carried by then
     is recorded only for the time of writing it began in.
   - Undoing a time settles a create of it that met a conflict, so the time accepted again is
     written afresh.
7. **Stopping and removing.**
   - Stopping writing forgets what was written, as ADR-0053 does. The account stays signed in
     for reading. Removing the account withdraws its access: Google revokes the grant, and
     Microsoft's is withdrawn in the account's settings.
   - Removing the calendar or the account also stops writing.
   - Every confirmation says what happens to the events already written.

## Consequences

- Accepted times appear in the owner's Google or Outlook calendar, and undoing one removes it,
  with the same protection ADR-0053 gives the owner's own events and changes.
- The refresh token now also allows adding and removing events, on Google calendars the owner
  owns or any Microsoft calendar. It stays where ADR-0064 keeps it, and ZeitBoard writes only
  its own events with it. The threat model records the wider grant.
- The account sees the accepted times' task names, as any calendar host does (ADR-0053's
  limitation 3).
- One calendar at a time still, whether CalDAV or an account's.
- Residual: Graph documents its de-duplication by transaction ID for retries, and moves a deleted
  event out of the calendar, where ZeitBoard does not look. So an event the owner deleted after a
  create whose answer was lost may be written again by the retry. Writing to the same calendar
  again after stopping, ZeitBoard assumes a create is not answered with an event the owner
  deleted meanwhile. Both need checking against a real account.
- Update, 2026-10-15: a code review of this change brought in the per-acceptance names, putting a
  removed Google event back, writing without reminders, looking first only on a retry and over
  every page, and what a sign-in again asks for (completion plan, "Writing to accounts after
  review").
- Update, 2026-10-15: a second review named each acceptance by its approval instead of its
  moment, which two acceptances could share. It also limited putting a Google event back to one
  removed before writing began, made every Microsoft create look first, and checked the mark
  before every Google removal. No release carried the earlier names, so nothing written under
  them needs carrying over (completion plan, "Writing to accounts after a second review").
- Update, 2026-10-16: a third review took clocks out of the choice to put a Google event back,
  kept an event the owner chose to keep, removed a create that landed for an undone acceptance,
  and let turning writing on be cancelled at any point (completion plan, "Writing to accounts
  after a third review"). A fourth review settled the choice on the time of writing each event
  carries, and kept a rotated refresh token through a cancel (completion plan, "Writing to
  accounts after a fourth review"). A fifth recorded every outcome of a create only for the
  acceptance it carried (completion plan, "Writing to accounts after a fifth review"). A sixth
  kept a pass to its own time of writing, and let an undo settle a create's conflict (completion
  plan, "Writing to accounts after a sixth review").

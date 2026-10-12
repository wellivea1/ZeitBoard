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
   - Signing in again later keeps what was allowed: Google with `include_granted_scopes`, and
     Microsoft because consent is kept per app.
3. **Only a calendar ZeitBoard may add to.** In Google, a calendar the owner owns. In Microsoft,
   one the owner can edit. A calendar shared with them is refused, by name.
4. **ZeitBoard's own events only, recognisable as such.**
   - In Google, an accepted time is written under an ID derived from ZeitBoard's own (base32hex,
     as Google requires) and carries a private mark naming it.
   - In Microsoft, it carries ZeitBoard's UID as its transaction ID.
   - A removal checks the event carries this time's mark or UID, else leaves it.
   - Reading the calendar back maps both to ZeitBoard's UID, so an import or refresh recognises
     them, as ADR-0053 does for CalDAV.
5. **Preconditions instead of trust.**
   - A Google create that finds its ID taken keeps the event only if it carries this time's
     mark; anything else is a conflict for the owner.
   - A Microsoft create first looks for an event with its transaction ID over the time it takes,
     and keeps one it finds. Graph drops a repeated transaction ID too.
   - A Google removal sends `If-Match` with the version ZeitBoard was given.
   - A Microsoft removal compares the change key first.
   - A changed event waits for the owner, as in ADR-0053. IDs are asked for as immutable, so an
     event the owner moves between calendars is still found.
6. **The writer and its queue are ADR-0053's.** The same triggers queue writes, and the same
   retries, conflicts, "Try again now", status and problem list apply. When the account cannot
   be signed in to, every due write waits and says why, such as an ended sign-in.
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

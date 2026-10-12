# ADR 0058: A share link can show the doses logged as skipped

- Status: accepted
- Date: 2026-10-11
- Delivers slice M-F of the medication plan ("Missed-dose sharing"), and widens the portal's
  allowlist (`portal-design.md` section 5) by one optional field.

## Context

Medisafe's "Medfriend" tells a chosen person when a dose is missed, so that they can check in. The
medication plan deferred an equivalent to the sharing transport, with two rules. It is a
permission of a share link, off unless the owner turns it on. And it never shows a dose that was
not logged as missed.

The second rule matters because ZeitBoard cannot know that an unlogged dose was missed. It may
have been taken and not recorded. The only way to reach another person is the share link on the
owner's server, and the portal was designed to carry no medication facts at all.

## Decision

1. **A grant on the link, off by default.** "Doses logged as skipped" can be turned on when a link
   is made, for that link only. Before it is turned on, the Sharing screen says exactly what it
   reveals: that the owner takes a medication on a schedule, and when they logged a dose as
   skipped.
2. **Times only.** The link's snapshot carries the time and zone of each dose logged as skipped in
   the last 72 hours, and nothing else. It holds no medication name, form, strength, schedule,
   note or reason, and no count of doses taken. The portal store holds those times and no other
   medication fact.
3. **Only what is logged as skipped, as it now stands.** A dose counts only if its record, after
   corrections, is "skipped" and it is not excluded. A dose corrected to "taken" disappears at the
   next publication. An expected dose with no record is never shown.
4. **Seen, not sent.** The visitor sees the times on the page, which updates when the snapshot
   does. Nothing is pushed to them, because ZeitBoard has no way to reach another person that does
   not go through an outside service.
5. **Published on its own terms.** Skipped doses are shown whatever the state of the forecast,
   and drop off 72 hours after the dose. The materializer runs after a dose is synced, as it does
   after a night. Seventy-two hours is long enough for someone who looks once a day to see each
   one, and short enough that the page is not a record of the owner's adherence.

## Consequences

- The portal allowlist gains `skippedDoses`, a list of `{at, zoneId}`, present only for a link
  with the grant. The response-shape and canary tests cover it, and a link without the grant is
  unchanged.
- The page's heading for these times names doses but never a medication. The disclosure says that
  the link reveals that a medication is taken.
- The exposure gate in section 12 still stands. This widens what an exposed portal could show, so
  the threat model and privacy notes say so.
- The fold that applies corrections to dose records moves into `core/medication`, so the desktop
  and the server read a dose the same way.

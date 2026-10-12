# ADR 0062: Calendars by the private address their provider publishes

- Status: accepted
- Date: 2026-10-12
- Resolves limitation 2 ("a calendar file is refreshed by importing the updated file again") and
  brings Google and Outlook calendars in without OAuth. Extends
  [ADR-0023](0023-local-calendar-ownership-and-adapters.md)'s read-only import.

## Context

Imported calendars are snapshots. A file is refreshed by importing it again, and a CalDAV calendar
by adding it again with its address. Only the calendar accepted times are written to keeps a sign-in
and refreshes from its row. A commitment added later in the owner's calendar is missing from
ZeitBoard until someone remembers to import again, so plans can be made over it.

Google Calendar, Outlook and iCloud each publish a private address for a calendar in iCal form.
Anyone holding that address can read the calendar, so it is a secret, but it needs no sign-in, no
OAuth app registered in the owner's name, and no write access. Connecting to these providers
through OAuth still waits on such registrations.

## Decision

1. **Subscribe by address.** Data Sources › Calendars accepts a calendar's private address:
   `https://`, or `webcal://`, read as `https://`. It is fetched once to preview, exactly as a file
   is previewed, and kept only on "Subscribe".
2. **The address is a secret.** It is kept in the owner-protected local database, as the write-back
   sign-in is.
   - It is never shown whole: Data Sources names only its host.
   - It is never synced, exported or logged.
   - It is erased with the calendar.
   - Redirects must stay on HTTPS.
3. **It refreshes itself.** Every hour while ZeitBoard runs, and from "Refresh" on its row, the
   address is fetched and the calendar replaced whole, as an import replaces it. A failed fetch
   keeps the last copy. The row then says that refreshing failed and when the copy is from.
4. **Bounded like a file.** The same parser, recurrence bounds and event caps apply, plus a 16 MiB
   response cap and a 20 second timeout.
5. **No schema change to what exists.** A subscribed calendar is an `ics` source, read-only like
   any import. Its address lives in a new table, `local_calendar_subscriptions`, which cascades
   with the source.

## Consequences

- Plans see a commitment within the hour of the owner adding it to their calendar.
- Calendars added from a file, or by CalDAV without write-back, still refresh by adding them again.
- Not done: OAuth, which would allow write access and calendars that publish no address, still
  waits on provider registrations in the owner's name.
- Update, 2026-10-12: [ADR-0064](0064-calendar-accounts-through-the-owners-registration.md) reads
  Google and Microsoft calendars through OAuth with an app the owner registers, so no registration
  in ZeitBoard's name is needed.

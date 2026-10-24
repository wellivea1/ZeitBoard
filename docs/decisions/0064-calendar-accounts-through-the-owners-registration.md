# ADR 0064: Google and Microsoft calendars through the owner's own app registration

- Status: accepted
- Date: 2026-10-12
- Delivers the planned "OAuth calendar providers" for reading, without app registrations in the
  owner's name or credentials shipped in ZeitBoard. Extends
  [ADR-0062](0062-calendars-by-their-private-address.md), whose private addresses need no sign-in
  but exist only where a provider publishes one.

## Context

ADR-0062 subscribes to a calendar by its private iCal address. Not every calendar has one: an
organisation may turn the addresses off, and a calendar shared with the owner may offer none.
Google and Microsoft both serve calendars through APIs behind OAuth 2.0. An app that signs in
that way is registered with the provider, and a registration in ZeitBoard's name would make
ZeitBoard a service the owner's sign-in passes through, needing the provider's verification.

Both providers let anyone register an app for themselves at no cost. An app registered by the
owner, used only by the owner, needs no verification and puts no third party between the owner
and their calendar. RFC 8252 describes how an installed app signs in: in the system browser, with
PKCE, returning to a one-time loopback address.

## Decision

1. **The owner's registration.** Data Sources › Add a calendar › Google or Microsoft asks for the
   client ID of an app the owner registers, and for Google its client secret. Google issues a
   secret even to desktop clients and says it is not confidential there. The steps to register sit
   beside the form. ZeitBoard ships no client ID, secret or proxy.
2. **Signing in, as RFC 8252 describes.** ZeitBoard opens the provider's sign-in in the owner's
   browser.
   - The request carries PKCE (S256) and a random state.
   - The answer comes back to a loopback port opened for it alone, on `127.0.0.1` (and `::1`).
     Google takes the IP literal RFC 8252 recommends. Microsoft takes `http://localhost` and
     matches it at any port.
   - The answer is taken only with the expected state, compared in constant time.
   - The port closes after one answer, a cancel, or five minutes. One sign-in waits at a time.
3. **Read-only scopes.** Google: `calendar.readonly`. Microsoft: `Calendars.Read` and
   `offline_access`. A sign-in that grants no refresh token is refused, because it could not
   be read again. So is one whose consent left reading unticked, which Google allows; it keeps
   nothing and says why.
4. **The refresh token is a credential.**
   - It is kept in the owner-protected local database, as the CalDAV write-back sign-in is, with
     the client ID and secret it was issued to.
   - It is never shown, synced, exported or logged. No error carries a token or a response body.
   - Access tokens live in memory for their hour. One is dropped early only when the provider
     refuses it; a calendar deleted at the provider fails on its own row.
   - A token a provider rotates replaces the old one. Refreshes take turns, and each refresh reads
     the sign-in as it is kept now, so a replaced token is never sent.
   - Removing the account erases it and every calendar brought in from it, compacting the
     database. Google is then asked to revoke the grant. Microsoft has no revocation address, so
     the confirmation says where to withdraw it.
5. **Calendars are chosen, previewed and kept like any other.** The account lists its calendars,
   its main one first. Each is previewed, then brought in.
   - A calendar is kept as a read-only imported source over the same span as a file.
   - Recurring events arrive as single occurrences.
   - Cancelled events are left out, and free time does not block.
   - Long names are shortened rather than refused.
   - The same event caps apply, with a 16 MiB answer cap, 50 pages, and paging that may not leave
     the provider's host.
   - Microsoft's times are asked for in UTC, so nothing depends on Windows zone names. They show
     in the zone the calendar is kept in.
   - An all-day event is a day, kept in the calendar's zone as a file's all-day dates are. Google
     states its dates. Graph states midnight in UTC, or midnight where the event was made, converted;
     either is read as its date.
   - An event that carries ZeitBoard's own UID, such as an accepted time imported from its export,
     is recognised as ZeitBoard's and not counted as busy time.
6. **Read again every hour.** Calendars from accounts refresh with the subscribed ones, each
   account's access refreshed once per pass. "Refresh" on a row reads at once. A failure keeps the
   last copy and says why on the row.
7. **An ended sign-in is said, and mended in one step.** A refresh the provider refuses ends the
   sign-in. Google ends them after seven days while an app is in testing. The account then says
   so, and "Sign in again" uses the kept registration. Signing in again to the same account
   replaces its sign-in and clears the problem.
8. **Tables of their own.** `local_calendar_accounts` holds the sign-ins.
   `local_calendar_account_calendars` links each kept calendar to its account and its provider ID,
   and cascades with both.

## Consequences

- Google and Outlook calendars come in without a published address, including calendars shared
  with the owner, and stay current within the hour.
- Registering an app takes the owner a few minutes, once. While Google's app stays in testing,
  its sign-in lasts seven days; publishing the owner's own app avoids that.
- The refresh token is unencrypted in the database, behind owner-only permissions and, by
  choice, Windows' encryption (ADR-0063). Anyone who can read it could read the owner's calendars
  until it is revoked.
- Writing accepted times to these calendars is a separate, later decision; CalDAV write-back
  (ADR-0053) is unchanged.
- Update, 2026-10-12: [ADR-0065](0065-accepted-times-written-to-google-and-microsoft-calendars.md)
  writes accepted times to a calendar from an account. The account is asked to allow it only
  when the owner turns writing on.
- Update, 2026-10-15: a code review of this change found the all-day, token, consent and
  cancellation cases now written into decisions 3 to 5. They are fixed, with a test each (completion
  plan, "Calendar accounts after review").

- Update, 2026-10-24: the browser's answer is now taken by the loopback receiver that the ChatGPT
  sign-in brought (ADR-0066), so the two sign-ins share it.
  - It answers only at the redirect's root.
  - It takes only a request whose Host names the receiver's own address: 127.0.0.1, or for
    Microsoft localhost or ::1. A page that rebinds its own name to loopback is turned away.
  - It refuses an answer that names a parameter twice.

  The redirects, the five-minute wait and the wording are unchanged.

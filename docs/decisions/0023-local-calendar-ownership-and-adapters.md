# ADR 0023: Local calendar ownership and adapters

- Status: accepted
- Date: 2026-07-22
- Builds on [ADR-0003](0003-default-deny-sharing.md),
  [ADR-0013](0013-desktop-local-data.md),
  [ADR-0016](0016-approvals-unification.md), and
  [ADR-0018](0018-user-owned-tasks.md).

## Context

The scheduler can avoid fixed intervals, but the desktop does not currently
load real calendar events into that input. Its Calendar route renders a
synthetic preview, and approving a local proposal changes only in-memory UI
state. Phase 2 requires a real local calendar without weakening ZeitBoard's
privacy boundary or allowing an imported calendar to be mutated accidentally.

Calendar data is unusually sensitive. Event titles, locations, descriptions,
attendee data, conference links, server addresses, and credentials must not be
included in server projections merely because event times are useful to the
scheduler. Imported calendar content also has ownership semantics that differ
from a block created by ZeitBoard after explicit approval.

## Decision

### Ownership boundary

The desktop keeps three source kinds in its local SQLite database:

- `ics` is a snapshot imported from an owner-selected iCalendar file;
- `caldav` is a snapshot fetched from an owner-entered CalDAV collection; and
- `zeitboard` contains blocks created by ZeitBoard after approval.

Events from `ics` and `caldav` sources have `imported` ownership and are
read-only inside ZeitBoard. Refresh may atomically replace that source's whole
snapshot, and removing the source may erase the snapshot, but no edit,
approval, undo, or export operation may update an imported event in place.

Events in the `zeitboard` source have `app_owned` ownership. Approving a local
placement transactionally appends a decision record and creates one app-owned
block linked to the exact task revision and proposal. Rejecting appends a
decision but creates no block. Undo appends a compensating decision and removes
only the linked app-owned block. Decision history is append-only; imported
records are never the target of a placement write.

The desktop exports only app-owned blocks. It does not present a re-export of
imported private calendars as though ZeitBoard owned them.

### Adapter boundary

iCalendar input follows RFC 5545 content-line unfolding, escaping, date/time,
duration, recurrence, cancellation, and transparency semantics. The parser
rejects structurally invalid or unsupported values instead of guessing a
different instant. File and network inputs are limited to 8 MiB, 20,000 event
components, and 50,000 materialized occurrences. Recurrence expansion covers
366 days before import through 732 days after import; each source records that
coverage so the UI can disclose when a requested range is incomplete.

File import uses separate preview and commit operations. Commit reparses the
selected file and replaces the source snapshot in one transaction rather than
trusting preview data supplied by the frontend.

CalDAV v1 is deliberately read-only and device-side. The adapter issues a
bounded `calendar-query` REPORT for `VEVENT` data. The collection URL, username,
and password are accepted only for that preview or import call. Credentials,
URL user-info, query strings, and fragments are not stored or logged. The
persisted endpoint is a sanitized HTTPS origin and path; loopback HTTP is
allowed for local self-hosting and tests. Authentication is not forwarded
across redirects. Google OAuth and provider-specific write APIs are outside
this slice.

### Privacy and projection boundary

`calendar-event-set.schema.json` is a private, device-local contract. It may
contain titles, locations, and notes because the local Calendar UI needs them.
Those fields are never added to schedule requests, trusted views, MCP payloads,
sync records, telemetry, or server projections.

The scheduler receives only the event identifier and half-open UTC interval
for busy occurrences. `TRANSP:TRANSPARENT` and cancelled events remain visible
where useful for source review but do not block placement. Free/busy decisions
do not inspect event text.

The Calendar screen obtains a display-specific local DTO that combines real
events with predicted sleep and waking windows. Civil-date queries always name
an IANA zone. All persisted instants are UTC; source and display zones are
retained separately where needed for faithful civil-time rendering.

### Proposal consistency

Local proposal identifiers are deterministic over the task identity and
revision, estimate identity, proposed half-open interval, immutable sleep-data
snapshot, and fixed-event snapshot used to compute them. Approval recomputes
proposals in the desktop service and accepts only an exact current identifier.
The storage transaction then rechecks the task revision plus sleep and
text-free busy-event fingerprints before writing. If the task, sleep evidence,
estimate, or calendar changed, the stale proposal is refused and the UI must
refresh.

Every successful placement write therefore has explicit approval and a
revision trail. Approval never edits the imported event that constrained the
proposal, even when the imported source is later refreshed or removed.

### Export

The app-owned export is an RFC 5545 calendar with CRLF line endings, folded and
escaped content lines, stable UIDs, and UTC `DTSTART`/`DTEND` values. UTC output
avoids inventing `VTIMEZONE` definitions. The export is a file operation under
owner control and does not upload event content.

## Consequences

- Real imported events constrain scheduling without exposing calendar text to
  the backend or assistant surfaces.
- Import, refresh, source removal, proposal approval, undo, and export have
  distinct ownership-preserving operations.
- Recurring calendars are a bounded snapshot, not an unbounded expansion. The
  coverage range is visible and refreshable.
- CalDAV v1 supports interoperable read-only collections but intentionally
  excludes OAuth-only providers and remote write-back.
- A block approved in ZeitBoard is immediately visible in Calendar and in the
  app-owned ICS export. It does not appear in the source calendar unless the
  owner imports that export elsewhere.

# Privacy

> **Architecture: connected, self-hosted, BYOK**
> ([ADR-0007](decisions/0007-connected-cloud-architecture.md) +
> [ADR-0008](decisions/0008-self-hostable-backend-byok-llm.md)). The backend is **entirely
> self-hostable** (the project operates no service and collects no telemetry); the user's
> data syncs to *their own* instance, and the assistant LLM is **bring-your-own-key**. The
> Milestone 1 sync path, Milestone 2 BYOK assistant backend, Milestone 3 server-side
> read projections, and the Milestone 4 local MCP connector are implemented; cloud skill
> packaging remains future work.
> Not legal advice.

## Commitments

- **Self-hostable, user-controlled.** The backend is entirely self-hostable; the project
  runs no service and collects no telemetry. The operator of an instance controls the
  data and is its data controller.
- **Bring-your-own LLM.** The assistant uses the user's own provider key (OpenCode Zen /
  OpenRouter / OpenAI / Anthropic, modeled on OpenCode). The project ships no keys; only
  minimized, redacted context is sent, to the provider the user chose and discloses —
  that provider relationship and its terms are the user's.
- No advertising, data brokerage, or third-party tracking SDKs.
- User-controlled acquisition, correction, export, sharing, and deletion.
- Data minimization at collection, storage, logging, projection, and testing.
- Encryption in transit (TLS) and at rest on the instance; BYOK credentials in a secret
  store, never logged.
- Explicit consent and least privilege for every platform permission and data source.
- **Legal scope: US / North Carolina** — honest representations (NC UDAP) and
  breach-notification awareness (NC Identity Theft Protection Act) for instance
  operators. Compliance outside the US/NC is the user's responsibility.

## Collection defaults

The desktop runs on the user's manually entered sleep data and shows an honest
empty state until entries exist; synthetic data appears only in clearly labeled
sample mode. Local sleep-file import reads only the JSON or CSV the user
explicitly selects; it does not scan arbitrary folders or upload the file.
Calendar import likewise reads only an owner-selected ICS file or an explicitly
entered CalDAV collection. CalDAV is read-only, bounded, and device-side. The
password is used for one request and cleared; it is never persisted. Stored
collection endpoints are sanitized to exclude credentials and query secrets.
Medication data is created only from labels, optional form/strength and
clinician-rule text, optional owner-recorded start markers, schedules, and
taken/skipped events the owner explicitly enters. The app does not query a drug
database, infer a schedule from a name or history, move a scheduled time, or
upload medication text. Desktop medication
notifications are off by default. Enabling them for an explicit clock schedule
allows the local operating-system notification surface to display the
owner-entered label at those times, including forecasted sleep overlaps.
Windows activity and Health Connect collection are disabled until
the user enables them through a visible permission flow, and backend sync is
opt-in and off by default. The application does not collect keystrokes, typed
content, screenshots, browser history, active-window titles, clipboard data,
precise location, or unrelated files.

Rhythm context markers are created only when the owner explicitly enters one
of four self-report categories, a retrospective/current civil time, and an
optional private note. The app does not infer markers from sleep, medication,
calendar, activity, location, or text.

Activity collection, when enabled, records only the minimum coarse state needed
by the product boundary. It must not retain application names or content.

Concretely, the desktop collector records a closed set of eight behavioural
states — startup, active, idle, locked, unlocked, suspended, resumed, shutdown
— each with a time and how long the previous state lasted. That is the whole
recorded shape; there is no field for an application, a window title, a
document, a URL, or a keystroke, and a test asserts the encoded payload
carries no key suggesting one. It answers "was this machine in use", never
"what was it used for".

It reads that state through two narrow system calls: time since last input,
which cannot expose what the input was, and whether the interactive desktop is
locked. It does not hook input, capture the screen, read the foreground window,
or sample at a rate that could reconstruct activity within a session — an
ordinary hour of work produces no records at all. Suspend and resume are
*inferred* from wall-clock gaps rather than observed, and the collector does
not claim a power-event capability it does not have.

This evidence is one input to sleep inference and is not a sleep record on its
own. Inferred sleep is marked as such, never overwrites a raw observation, and
does not reach planning until a documented validation decision allows it.

## Local storage

Private data lives in the local SQLite database and syncs to the user's
self-hosted instance. Database files, write-ahead logs, exports, and backups must
be treated as sensitive and have file permissions restricted to the owner.
**At-rest encryption (local and on the instance) and TLS in transit are required**,
not optional — the operator holds the keys.

Deletion removes local derived data and source records according to the user's
explicit request, subject to a clear confirmation flow. When backend sync is
enabled, erasure propagates: the self-hosted instance hard-deletes its synced
copy and mints a metadata-only tombstone (record id plus original kind when
known, no health content) so every other
enrolled device erases its copy on the next pull, and an erased record can
never be re-pushed (ADR-0017). A device that never syncs again retains its
local copy until it does. Export must be an intentional action and should
identify whether it contains private data or only a minimized projection.
Task deletion applies to the logical task: the instance erases every retained
immutable revision and rejects all later revisions, including revisions that
were not present when deletion was requested.


The instance also keeps a small operational record of when analysis last ran
(ADR-0033). Each entry holds a digest of the inputs and of what was published,
plus times, a reason from a closed set, and any failure message. The digests are
encrypted with everything else in that database: they are one-way, but a digest
still answers "did they sleep at 04:12 on Tuesday?" for anyone willing to guess,
and the reason the file is encrypted at rest is that a copy of it answers
nothing. The table is capped at 200 entries and is not an audit trail; it holds
no observation payload, no window, and no share-link label.

Imported sleep observations use the same local table, export path, correction
layer, sync path, and erasure controls as manual observations. Conversion tools
request mode `0600`, require explicit overwrite, and still rely on an
owner-controlled directory ACL on Windows. Preview reports may show row
timestamps in the local UI; logs and committed verification retain counts and
aggregate metrics only.

Imported calendar snapshots remain private local records. Their titles,
locations, and notes are available to the desktop Calendar UI but are excluded
from scheduling requests, backend sync, trusted views, MCP tools, and LLM
context. The scheduler receives only opaque event identifiers and UTC
intervals. Removing an imported source requires typed confirmation, cascades
through its events, checkpoints the WAL, and vacuums the local database.
Imported events cannot be edited or suppressed in place. Approval creates a
separately owned ZeitBoard block; rejection creates no event; undo removes only
that app-owned block. Calendar export contains app-owned placements only and
never copies imported text.

Medication definitions, optional start markers, schedules, raw events,
corrections, and reminder claims remain in local SQLite. Derived wake/sleep
relationships are recomputed and are not included in the medication export.
Reminder claims contain only an
opaque occurrence ID, medication ID, and scheduled/claimed UTC instants; they
are inserted before delivery to prevent duplicate prompts and are excluded
from export and sync. Exclusion appends a correction and retains the raw
evidence; typed `DELETE` erasure removes an event and its corrections, or a
definition and all dependent events, corrections, schedules, and reminder
claims, then checkpoints and vacuums SQLite. Medication labels, clinician
rules, and notes are excluded from backend sync, trusted views, MCP/assistant
context, telemetry, and logs in M-A/M-B/M-C. Future sync requires a new reviewed
redaction and tombstone path before those records may leave the device.
Erasing the SQLite record cannot retract a reminder label already delivered to
Windows notification history; the opt-in disclosure treats that OS-managed
copy as outside ZeitBoard's erasure boundary.

The M-C clinician report is generated entirely in the desktop process. Go
applies redaction before returning either preview data or standalone HTML:
diagnosis, calendar/location information, and clinician-entered medication
guidance are always omitted; medication labels/forms/strengths are aliases by
default; medication notes and marker notes require separate opt-ins. Export
requires typed `EXPORT`, has an offline content-security policy and no scripts
or external assets, and makes no network request. Once the owner saves or shares
an explicitly generated file, that copy is outside ZeitBoard's storage,
revocation, and erasure boundary.

Rhythm context markers and their notes remain in local SQLite. They are
excluded from estimation, scheduling, reminders, backend sync, trusted views,
MCP/assistant context, telemetry, and logs. The owner may deliberately export
the strict v1 marker set, which includes private notes. Individual erasure
requires typed `DELETE`, then checkpoints the WAL and vacuums SQLite; tests
assert that a unique private-note marker no longer exists in either file.

## Logging

Logs may contain component names, error categories, counts, durations, and
opaque correlation IDs. They must not contain observation payloads, medication
labels or notes, rhythm-marker kinds or notes, health details, calendar
content, tokens, exact behavioral timestamps, or raw
trusted-view URLs. Debug logging does not relax this rule.

## Sharing

Sharing is default-deny. Every field is separately allowlisted, profiles expire,
and revoked or expired profiles produce no view. Projection code constructs the
trusted DTO field by field and never serializes the private model. Server-side overview,
rhythm, and accuracy projections are authenticated read DTOs built from the decrypted
sync store and omit raw sync payloads, source record IDs, notes, medication names, and
tokens. Rhythm markers are not a grantable field and the strict trusted-view
schema rejects them. The phase-one trusted website is static, synthetic, and
makes no network request.

### Availability portal (implemented, off by default, not exposed)

The portal is off by default and is not exposed. When an operator turns it on
and the user creates a link, this is what a recipient can and cannot see.

A recipient with the link **and** its passcode sees broad windows when the user
is likely awake, when that estimate was last refreshed, and how far ahead it
runs. They see nothing else: no sleep record, no medication, no task, no
calendar text, no note, no marker, and no confidence label. Confidence is
withheld on purpose — ADR-0022 measured the buckets inverted on real history,
so a published label would mislead. Every view carries the measured uncertainty
in plain language, and an estimate more than a day old is withheld rather than
shown.

Windows are rendered in the user's own time zone, which the page states. That
discloses the user's zone to anyone holding the link; it is unavoidable if the
times are to mean anything.

Sharing a live projection is observable over time: a recipient who checks
repeatedly can watch the user's rhythm drift. That is inherent to the feature,
is disclosed when a link is created, and is a reason to share deliberately.

The portal keeps its own database. It holds hashed link tokens, hashed
passcodes, the materialized windows, sessions, rate-limit counters, and a
coarse access log. It holds no health data, and public request handlers have no
route to the private database at all. The user's own name for a link — "Mum", a
clinician — is stored encrypted in the *private* database and never reaches the
portal.

Abuse limits need to tell visitors apart, so the portal stores a keyed hash of
a normalized network address and discards the address itself. This is
pseudonymous, not anonymous: several people behind one router share an
identifier, and one person moving between networks gets several. Rotating the
key makes old identifiers unlinkable and deletes access rows past the retention
window. The Sharing screen shows counts and a last-access time, not a browsing
trail.

Links expire (90 days at most), can be revoked at any moment, and revocation is
immediate: existing sessions stop working and the shared windows are deleted
from the portal database. Open requests are closed rather than deleted, so
nobody is left watching a status that never moves. What a recipient already
saw, screenshotted, or remembers cannot be recalled.

### Asking for a time

When a link allows it, a recipient can ask for a window that suits them and
optionally give a name and a short note. That text is theirs, and it is
private: encrypted in the portal database, encrypted again inside the owner's
copy, shown to the owner because judging a request requires it, and never
placed in the availability page, the shared projection, an access-log row, or
a notification title. The product does not claim it never accepts names — it
accepts them and protects them.

Until the request has actually reached the owner's queue it is shown as
"saved and on its way", never as sent. Claiming delivery the visitor cannot
verify would be a small lie with real consequences for someone waiting.

Approving a request tells the requester the exact time chosen. That is
necessary to meet, and the owner is told so before choosing. Declining tells
them only that the time did not work — never a reason, because any reason
would disclose sleep, calendar, or health.

Anyone who creates a request receives a one-time code proving they wrote it.
Holding the shared link and its passcode is not enough to read someone else's
request: several people can hold the same link and still cannot see each
other's asks.

## Agent connectors

There are two, and neither can approve or apply anything.

**Backend connector.** A stateless adapter over the self-hosted backend API. It holds only
the backend URL and a device token, exposes read projections and propose-only tools, and
stores no health data. It has no approval/apply tool and cannot consume approval tokens;
human approval remains in the backend's existing one-use decision endpoint. Missing or
unreachable backend configuration exposes no tools.

**Desktop-local endpoint (ADR-0028).** The desktop app serves its own MCP endpoint on
loopback while it runs, so an agent on the same machine can read state and change
appearance without the backend. It exposes allowlisted read projections, one direct
display action (`set_appearance`, reversible and non-health per ADR-0021), and
propose-only scheduling tools that require an enrolled backend. It has no approve/apply
tool either.

What the local endpoint may return includes **medication timing facts and rhythm context
markers** drawn from local records - for example how long after waking a dose was logged,
or that a travel marker exists on a date. These are the same projections the local UI
shows, never raw records: no clinician notes, no free-text medication notes, and no
observation payloads. Medication *labels* are the user's own private text and stay on the
device; they are not sent to any LLM provider by this path, because the endpoint answers
from local records without calling one. Requests that ask for a medical decision return
the standard refusal instead of an answer.

Access to the endpoint requires a bearer token held in a descriptor file that is
restricted to the current user (an explicit owner-only DACL on Windows, mode 0600
elsewhere). Any process already running as the user can read that file; the endpoint is a
user-level boundary, not a sandbox.

## Development data

Fixtures, tests, screenshots, demos, and static assets contain synthetic data
only. The `tools/` fixture generator is deterministic and checks trusted-view
fixtures for forbidden private keys. Real exports, converted import files,
transcription sheets, validation databases, detailed backtest points, and chart
renders must never be committed, attached to issues, or used in CI. Aggregate
backtest metrics may be committed when they contain no raw timestamps or
identifiers.

## Product claims

The product presents estimated sleep-wake phase, predicted sleep and waking
windows, and confidence/uncertainty. Calendar overlays show scheduling
constraints; they do not make those estimates more physiologically exact. The
product does not claim exact circadian phase or DLMO and does not provide
diagnosis, treatment, or behavioral recommendations.

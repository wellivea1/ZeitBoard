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
clinician-rule text, schedules, and taken/skipped events the owner explicitly
enters. The app does not query a drug database, infer a schedule from a name or
history, move a scheduled time, or upload medication text. Desktop medication
notifications are off by default. Enabling them for an explicit clock schedule
allows the local operating-system notification surface to display the
owner-entered label at those times, including forecasted sleep overlaps.
Windows activity and Health Connect collection are disabled until
the user enables them through a visible permission flow, and backend sync is
opt-in and off by default. The application does not collect keystrokes, typed
content, screenshots, browser history, active-window titles, clipboard data,
precise location, or unrelated files.

Activity collection, when enabled, records only the minimum coarse state needed
by the product boundary. It must not retain application names or content.

## Local storage

Private data lives in the local SQLite database and syncs to the user's
self-hosted instance. Database files, write-ahead logs, exports, and backups must
be treated as sensitive and have file permissions restricted to the owner.
**At-rest encryption (local and on the instance) and TLS in transit are required**,
not optional — the operator holds the keys.

Deletion removes local derived data and source records according to the user's
explicit request, subject to a clear confirmation flow. When backend sync is
enabled, erasure propagates: the self-hosted instance hard-deletes its synced
copy and mints a tombstone (record id only, no health data) so every other
enrolled device erases its copy on the next pull, and an erased record can
never be re-pushed (ADR-0017). A device that never syncs again retains its
local copy until it does. Export must be an intentional action and should
identify whether it contains private data or only a minimized projection.

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

Medication definitions, schedules, raw events, corrections, and reminder
claims remain in local SQLite. Derived wake/sleep relationships are recomputed
and are not included in the medication export. Reminder claims contain only an
opaque occurrence ID, medication ID, and scheduled/claimed UTC instants; they
are inserted before delivery to prevent duplicate prompts and are excluded
from export and sync. Exclusion appends a correction and retains the raw
evidence; typed `DELETE` erasure removes an event and its corrections, or a
definition and all dependent events, corrections, schedules, and reminder
claims, then checkpoints and vacuums SQLite. Medication labels, clinician
rules, and notes are excluded from backend sync, trusted views, MCP/assistant
context, telemetry, and logs in M-A/M-B. Future sync requires a new reviewed
redaction and tombstone path before those records may leave the device.
Erasing the SQLite record cannot retract a reminder label already delivered to
Windows notification history; the opt-in disclosure treats that OS-managed
copy as outside ZeitBoard's erasure boundary.

## Logging

Logs may contain component names, error categories, counts, durations, and
opaque correlation IDs. They must not contain observation payloads, medication
labels or notes, health details, calendar content, tokens, exact behavioral
timestamps, or raw
trusted-view URLs. Debug logging does not relax this rule.

## Sharing

Sharing is default-deny. Every field is separately allowlisted, profiles expire,
and revoked or expired profiles produce no view. Projection code constructs the
trusted DTO field by field and never serializes the private model. Server-side overview,
rhythm, and accuracy projections are authenticated read DTOs built from the decrypted
sync store and omit raw sync payloads, source record IDs, notes, medication names, and
tokens. The phase-one trusted website is static, synthetic, and makes no network request.

## Agent connector

The local MCP connector is a stateless adapter over the self-hosted backend API. It holds
only the backend URL and a device token, exposes read projections and propose-only tools,
and stores no health data. It has no approval/apply tool and cannot consume approval
tokens; human approval remains in the backend's existing one-use decision endpoint. Missing
or unreachable backend configuration exposes no tools.

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

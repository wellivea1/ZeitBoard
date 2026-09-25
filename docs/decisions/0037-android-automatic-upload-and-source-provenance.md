# ADR 0037: Android automatic upload and source provenance

> Updated by [ADR-0039](0039-pre-release-contracts-and-streamlining.md): the owner confirmed there are no packaged releases or deployed users. Historical migration and compatibility requirements below are superseded; current durability and erasure guarantees remain.

- Status: accepted; C1 remains partial
- Date: 2026-09-08
- Follow-up: [ADR-0038](0038-android-companion-cache-and-erasure.md) adds pull, cache and erasure,
  replaces device-dependent provider parent links, and records the outstanding strict-v1
  compatibility migration before release.
- Extends [ADR-0032](0032-android-sleep-synchronisation.md) and the
  [completion plan](../completion-plan.md).

## Decision

One application-owned container supplies repositories to the activity and WorkManager workers.
Settings exposes enrollment, upload, disconnect and a separate, persisted, default-off
automatic-refresh switch. The foreground refreshes the selected real source on resume. A unique
hourly worker imports sleep only with enrollment, real-data mode, the automatic-refresh setting,
READ_SLEEP and feature-supported READ_HEALTH_DATA_IN_BACKGROUND permission. Permission requests
remain separate and visible. Fixture data cannot upload.

Import works without network access. Upload is a separate unique job constrained to a connected
network, with exponential backoff, at most five attempts per job and four requests of at most 100
records per invocation. A later periodic run can retry remaining work. Foreground-started uploads
can finish while closed; turning off periodic refresh does not cancel them. Disconnect cancels both
jobs and forgets enrollment and the upload queue, retaining the original local data. Android can
defer jobs for battery policy; force-stop requires reopening the app. This is not an exact hourly
delivery guarantee.

Use pinned WorkManager 2.11.2 and kotlinx.serialization JSON 1.9.0. Health Connect 1.1.0 already
exposes feature detection and the background permission. See the primary
[background-read documentation](https://developer.android.com/health-and-fitness/health-connect/read-data),
[WorkManager releases](https://developer.android.com/jetpack/androidx/releases/work) and
[serialization release](https://github.com/Kotlin/kotlinx.serialization/releases/tag/v1.9.0).

## Durable enrollment and immutable revisions

Network enrollment must succeed before changing local authorization or queue state. A newly selected
server gets a new persisted queue generation. The configuration commits before queue activation;
every repository access selects that generation before reading records. On a restart after an
interrupted server switch, obsolete rows are discarded before any upload. A failed request preserves
the prior server and queue. Canonically equivalent re-enrollment keeps the generation. Failed
preference commits restore their prior in-memory values. Schema 4 adds `queue_scope`; migrations
from versions 1–3 retain existing rows under the legacy generation. No database is dropped or
recreated.

Mapping considers all durably queued source revisions, including pending ones. The first revision
after an observation has no fictitious correction parent; later revisions supersede the actual
preceding provider correction. FIFO upload preserves dependency order. Reading the immutable payload
preserves nanoseconds even for old outbox rows whose bookkeeping used milliseconds. Repeated mapping
does not duplicate an observation or replay the same source change forever.

HTTP requests run off the UI thread, use normal TLS validation, refuse redirects, restrict cleartext
to device loopback, bound responses and validate versioned acknowledgments. An atomic duplicate-only
server response may accept zero new rows. Empty, malformed or incompatible responses cannot mark
records uploaded. Cancellation leaves retryable records intact. UI errors do not echo payloads,
secrets or arbitrary exception text. A successful empty push does not invent a last-upload time;
upload bookkeeping is not estimate freshness.

## Provider corrections do not imply human confirmation

The v1 correction object gains an optional `acquisition_method` field: `manual` or `health_connect`.
Provider corrections must use `source_conflict` and may change timestamps only, targeting Health
Connect observations. Their evidence stays observed/imported with the provider revision time and
correction IDs. Unattributed legacy Health Connect timestamp conflicts are conservatively
unconfirmed; explicit manual corrections retain human confirmation.

A newer provider revision conflicting with an older active user correction withholds the effective
projection pending renewed review. It must not silently override that user's correction. This
conservative refusal needs a more direct review handoff in the remaining C1/C2 work.

Update the backend and desktop core before this Android build: old strict decoders can reject the
new optional field. Rejections retain queued data. Existing payloads remain readable; stored
observations and corrections are never rewritten merely to add the field. The shared generated
Android wire fixture is checked by Kotlin, Go validation, Go replay and JSON Schema.

The server also rejects any new correction targeting an erased observation, even when the provider's
new revision has a previously unseen correction ID. This prevents an offline phone from restoring
erased behavioral timestamps.

## Remaining acceptance

This increment implements automatic import/upload plumbing and its reachable UI. It does **not**
finish C1: Android still lacks pull/tombstone application, remote corrections/tasks and cached real
estimates. Manual corrections and medication events remain local. Travel/missing-offset records
remain visibly held. Disconnect is not server erasure or remote device revocation.

Unit tests, SQLite migration/reopen tests, a disposable Android 16 emulator and real HTTP calls into
a disposable Go API establish the software paths. They do not establish real wearable delivery, OEM
battery behavior, background runtime across all supported devices, production TLS installation, or
the private pilot. Keep those operational checks open in the completion plan.

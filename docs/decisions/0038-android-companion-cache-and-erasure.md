# ADR 0038: Android companion downloads and erasure

> Updated by [ADR-0039](0039-pre-release-contracts-and-streamlining.md): the owner confirmed there are no packaged releases or deployed users. Historical migration and compatibility requirements below are superseded; current durability and erasure guarantees remain.

- Status: accepted implementation; C1 remains partial
- Date: 2026-09-08
- Extends [ADR-0037](0037-android-automatic-upload-and-source-provenance.md).

## Private server projection

`GET /v2/companion` requires an enrolled device token and returns an explicit private DTO with UTC
windows, interpretation zones, algorithm, confidence, freshness, a refusal when necessary, and an
at-most-15-minute cache validity. The response identifies synthetic input. Its sleep rows use
presentation hashes, not observation IDs. The endpoint has no portal, MCP or provider exposure.
Android displays the Go result; it does not fit or extend a forecast itself.

The server reads encrypted sleep observations and corrections in one SQLite snapshot, together with
the sync high-water mark. It bounds the snapshot at 100,000 records / 16 MiB and refuses larger
history explicitly. The projection returns at most 256 recent effective sleep rows; raw sync remains
paginated. The pull endpoint accepts an optional 1–500 record limit without changing its existing
default. Responses prohibit intermediary caching.

## Download, upload and lifecycle

Android SQLite schema 5 adds a replica, cursor, forecast cache, erasure markers, and source-target
metadata, migrating versions 1–4 without dropping evidence. The replica belongs to an enrollment
generation plus a token fingerprint. Changing authorization clears downloaded state; a failed
enrollment preserves it.

Every sync pulls before uploading, atomically commits each page and cursor, and invalidates derived
output whenever records change. Four 100-record pages per phase bound an invocation; pending pages
continue through durable retries. A forecast is cached only when its source cursor matches the
complete replica. Unknown versions, malformed windows, unexpected projection fields and invalid
acknowledgments fail without being represented as successful sync.

Server rollback is distinguished from ordinary incoming pages. A lower cursor produces an actionable
re-enrollment message. After a complete new download, locally accepted uploads absent from the
server are requeued, respecting all retained erasure markers. Re-enrollment reconciles a phone's
newer observation with an older server original as an append-only provider correction. Equal
provider revisions with conflicting timestamps retain the pending record and refuse instead of
silently overwriting it.

Provider timestamp payloads no longer depend on which intervening revisions a phone saw: newly
emitted provider corrections omit a device-local parent link. Go folds provider revisions before
manual overlays. Superseding a provider revision manually preserves its unedited endpoint; a newer
provider revision still requires review when it conflicts with an older manual correction. Creation
times from either correction endpoint contribute to freshness.

## Erasure

The server indexes correction targets, backfilling older encrypted records in bounded pages. Erasing
an observation also hard-deletes retained dependent corrections and publishes their tombstones.
Later corrections cannot restore an erased source. Task erasure applies to every logical revision.

Android applies tombstones before other page records. It removes replica rows, dependent
corrections, queued uploads, affected imported Health Connect versions and the forecast cache in the
same transaction. Metadata-only Health Connect suppression survives disconnection and re-enrollment,
preventing a subsequent provider import from restoring those records. The app enables SQLite secure
deletion and durably retries WAL truncation / vacuum after erasure. These are application-level
deletion measures, not a forensic guarantee for flash storage.

## Reachable UX and remaining work

My data mode can be used without Health Connect permission to read an enrolled server. Status
displays forecast age, explicit synthetic labeling, cache expiry, refusal and recent effective
sleep. Tasks displays latest downloaded revisions and constraints, with an explicit read-only
desktop editing handoff. The task list exposes truncation above 500 logical tasks. The five phone
destinations remain compact; sample mode stays separate. Foreground refresh and a visible clock
update prevent an open screen from treating yesterday's cache as current.

This does not finish the companion: phone-authored manual sleep corrections and medication events
remain local, and task editing/placement remains on desktop. The UI states those limitations. Travel
records lacking a supported IANA mapping remain held. Real wearable/background/battery/TLS/pilot
evidence remains open.

The optional correction provenance field stays in the current contract. Prototype
compatibility is unnecessary under the owner's clarification; ADR-0039 removes
alternate routes, export version branching and old development backfills.

## Verification

Go tests cover authenticated projection/refusal, bounded expiry, effective erasure,
correction-target backfill and provider/manual layering. Shared fixtures validate the new v2
projection. Android unit/lint/build checks and disposable Android 16 SQLite/HTTP instrumentation
cover revision downloads, cache estimates, erasure, re-import suppression, migration, restart and
restored-server replay. Exact results and operational limitations are recorded in `verification.md`.

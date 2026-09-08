# ADR 0040: Review context for synchronized sleep corrections

- Status: accepted
- Date: 2026-09-08
- Extends ADR-0037/0038 under the pre-release rule in ADR-0039.

## Decision

A manual correction records the source revision and active manual edits the owner
reviewed. A later creation time does not establish that newer source evidence was
seen. The current correction contract uses `based_on_source_revision` and a bounded
`supersedes_correction_ids` list. A resolution can acknowledge multiple concurrent
edits. There is no second wire format or conversion path for the prototype format.

The Go core applies provider revisions as the source baseline, then the single
active manual overlay. A provider revision older than the original cannot undo it.
Unreviewed source changes and concurrent manual heads withhold estimation. The
review model separately retains the source window and competing edits, so refusal
does not remove the owner's ability to inspect or repair a record.

`GET /v1/sleep/{id}/review` is an authenticated, no-store owner endpoint. Its explicit
allowlist contains the target's editing context only. A coherent encrypted-log
snapshot bounds one review to 10,000 records / 2 MiB, and at most 256 concurrent
manual heads. The private review is not added to portal, assistant or MCP projections.

Android uses one full exchange: bounded pull, source reconciliation, push, pull and
Go projection caching. Replica storage is required; upload-only execution methods
and nullable-storage branches are removed. Enrollment must have a persisted queue
scope; the prototype scope fallback and schema defaults are removed.

Connected Correct lets the owner select a downloaded sleep observation, review its
source and manual alternatives, edit endpoints/classification/exclusion, and queue
a complete immutable resolution in the existing durable outbox. An unchanged or
chosen endpoint retains its seconds, nanoseconds and explicit UTC offset. Cached
review supports offline saving; only one pending manual correction per target is
allowed until sync completes. Manual edits never advance provider revision tracking.
Incoming changes mark an open review stale. Erasure and enrollment changes clear
open context; erasure also clears durable review caches. Reload explicitly replaces
the draft. Source selection is bounded to the latest 500 synchronized sleep sources.

Desktop uses the same core review model for its editable sleep log. Estimation
continues to refuse conflicts. A token binds each form to the source revision and
manual heads shown, so an older form cannot silently claim review of newer records.
Suppression requires current, undisputed context. Editing can preserve or remove
exclusion, and supports unknown classification. Timestamp inputs include offsets
and full precision. Correction storage drops the redundant single-parent SQL
column; immutable JSON owns the reviewed-parent list.

## Verification and remaining work

Core/server/desktop tests cover concurrent branches, stale-source review, explicit
resolution, erasure, repeated hours and subsecond precision. Android tests use the
same generated review fixture. A disposable emulator exchanges actual HTTP with
the Go API: a queued manual edit survives database reopen, reaches the server,
resolves a competing edit and disappears from open/cached review on erasure.
The native form was also exercised through enrollment, selection and save.

Local-only Android corrections still use the local repository. They do not yet
transfer automatically on later enrollment; consent and UI copy disclose this.
Complete that current-user workflow by consolidating its record production with
the reviewed correction contract, without adding historical database migration or
legacy-format support. Desktop lifecycle acceptance and C2–C8 remain in the full
completion plan. Synthetic checks do not qualify wearable/OEM behavior or a pilot.

# ADR 0045: Atomic desktop enrollment and restore reconciliation

- Status: accepted
- Date: 2026-09-21
- Continues completion-plan C1 after ADR-0044.
- Supersedes the separate desktop credential/config files in ADR-0015.

## Findings

Desktop enrollment wrote a disabled replacement config and deleted the old token
before trying the new server. A failed switch therefore destroyed a working
connection. Cursor and acknowledgment state did not belong to the enrollment,
so another server could inherit progress it had never accepted. Pull also skipped
this device's envelopes, preventing restoration of its own missing records.

Additional recovery gaps occurred at the persistence boundary. Corrections whose
source arrived on a later page were discarded while the cursor advanced. A
downloaded task could overwrite an unsent edit or falsely acknowledge a different
edit at the same revision. Erasure relied on saved upload acknowledgments, which
cannot prove whether an interrupted request committed remotely. Deletion markers
were forgotten after acknowledgment and could not protect a later restore.

## Decision

Enrollment settings and the bearer token now occupy one owner-protected SQLite
row, in separate columns. Only non-secret settings have a serializable DTO. A
successful remote enrollment commits the credential, settings and reconciliation
boundary atomically alongside the data they authorize. Failed enrollment or local
commit leaves the previous connection intact. Disabled state clears the credential
and compacts local storage. Status updates cannot change enrollment identity or
revive a disabled connection. Tokens remain absent from UI, exports and sync
payloads; the local database and backups must be protected as credentials as well
as health data. No historical development-file migration or alternate credential
path is retained, following the pre-release decision in ADR-0039.

Each enrollment starts its pull cursor at zero. A temporary set records IDs seen
in the new stream; old acknowledgments remain until a complete download. Then
only acknowledgments absent from that stream are removed, making missing retained
records eligible for upload. Existing local sleep/task data intentionally remains
part of the owner's profile when changing servers. Settings discloses that these
records and deletion markers go to the newly selected server; changing servers
does not erase the previous server's copies.

Sync downloads before uploading, applies local erasures, uploads pending records,
then downloads again. Pull drains up to four 500-record pages per phase and keeps
progress durable if another attempt is needed. Envelope order, cursor, identity,
kind and required metadata are checked before applying a page. Records authored by
this same device are included. The server checks cursor versus history in one
read transaction and returns HTTP 409 if a cursor is ahead after rollback. Desktop
and Android direct the owner to re-enroll and reconcile. A rollback whose log has
already grown beyond the old cursor cannot be inferred from that cursor alone;
server restore procedures must still re-enroll clients.

SQLite retains metadata-only suppression markers after local and remote erasure,
including task identity across revisions. Enrollment requeues those markers.
Deletes include records without saved acknowledgments; an upload completing after
local deletion queues erasure rather than recreating tracking state. Replayed
pages and source imports cannot resurrect the same erased IDs. Derived analysis
still invalidates through ADR-0044. These markers are necessary for retry and
restore integrity; they do not retain erased record payloads.

Corrections waiting for a source retain their immutable payload in a private
deferred table. Later pages apply them when the source arrives. Source/tombstone
erasure removes deferred copies. Settings reports their count and waiting state,
as well as pending deletions. Immutable payload mismatches and task conflicts
retain local data and refuse the page instead of silently acknowledging or
overwriting it. A complete reviewed task-conflict resolution flow remains part of
C2's approval/editing work.

## Verification and limits

Storage tests cover failed enrollment rollback/reopen, delayed reconciliation,
cross-page dependencies, suppression, interrupted acknowledgments and unsent task
conflicts. Stateful TLS tests cover failed and successful server switching,
rollback before upload, missing accepted records, erasure after restore and a
501-record same-device download. An opt-in desktop integration test also passes
against the actual disposable loopback daemon: same-credential restoration,
correction exchange, erasure and re-enrollment. It uses synthetic data only.

This is software recovery evidence. Native login/hide/suspend/resume/quit,
supported hardware and private pilot qualification remain open. Full-history
performance and a reviewed task-conflict UI remain explicit follow-up work;
neither is established by successful round-trip tests. No owner installation,
production service or release package is deployed by this change.

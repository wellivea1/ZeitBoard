# ADR 0044: Desktop background analysis consumed by readers

- Status: accepted
- Date: 2026-09-08
- Continues completion-plan C1 and ADR-0033 after ADR-0043.

## Decision

The desktop starts an owned analysis worker with the app, independent of mounted
views. It uses the existing Go estimator, shared freshness policy and recompute
orchestrator. The former server-only worker now lives in `core/recompute`; both
consumers use that implementation, without a compatibility alias or second loop.
The scheduler has no Wails, provider, network or subprocess dependency.

Desktop SQLite holds one derived sleep-analysis snapshot and at most 200 operational
journal entries. Empty, refused and estimated results are explicit. Sources are
not copied into the snapshot. Readers obtain the corrected source fold, its
fingerprint and the snapshot in one transaction. They accept only matching inputs,
algorithm, content checksum, as-of minute and unexpired validity. A missing,
corrupt or expired result uses the same serialized pipeline, then rereads the
evidence; a changing source permits bounded retry, never an old forecast fallback.
Existing planning fingerprint checks still protect proposal construction and
approval. A correction-review conflict still withholds estimation.

The analysis as-of instant is normalized to a minute. Validity ends at the next
minute or an earlier `freshness.NextChange` boundary. Foreground current-state
claims reassess freshness at the actual requested instant. Clock regression
invalidates a snapshot from a later minute, and the scheduler reconciles a
regressed clock on its next wake. Expiry bypasses the evidence-burst interval
floor; a failed refresh retains backoff, including against heartbeat retries.
Synchronous refresh reschedules a sleeping worker when it brings expiry forward.

Input changes atomically invalidate the snapshot through current-schema SQLite
triggers. Source deletion also clears the journal. Both journal creation and
snapshot publication recheck the source fingerprint transactionally, so erasure
during computation cannot republish a forecast or reinsert the erased inputs'
fingerprint. Explicit erase-all clears derived rows even when source tables are
already empty. The journal stores generic failures rather than raw SQL or health
errors; interrupted runs are sanitized during startup recovery. These local
tables use the desktop file protection from ADR-0035 and have no public endpoint.

Recovery and publication serialize. Closing the worker cancels and joins both
scheduled work and foreground calculations before storage closes, and rejects
later work. A duplicate start returns the active completion signal. The app emits
one payload-free native update event, bridged at the React root into existing
evidence-dependent refresh listeners. Changes and erasure invalidate visible state
without placing health data on the event bus.

The content-change stamp excludes housekeeping times, generated IDs and changing
age counters. Refreshing unchanged content updates computation time without
claiming that evidence just changed. The Overview label reports the content-change
time; its freshness block separately reports evidence age and withholding.

## Verification and limits

Tests exercise expiry without a mounted view, foreground cache consumption,
reopen, stable content timestamps, correction invalidation, clock regression,
erasure during computation, corrupt derived data, bounded/sanitized journals,
interrupted recovery, cancellation/join and timer rescheduling. Server integration
tests use the moved worker; the browser bridge has event and cleanup checks.

This delivers consumed background analysis while the process runs. Native Windows
login, hide, suspend/resume and quit qualification remains separate. Desktop
enrollment/restore reconciliation is the next C1 software work. Full-history input
fold/fingerprint cost still needs the bounded-history/performance work in C8;
caching the estimator does not establish an unmeasured performance claim. No
activity-derived sleep promotion, automatic proposal, notification, network
disclosure or packaged release is introduced.

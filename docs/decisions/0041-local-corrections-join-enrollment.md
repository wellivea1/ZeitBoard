# ADR 0041: Local sleep corrections join enrollment

- Status: accepted
- Date: 2026-09-08
- Completes the local-only handoff left open in ADR-0040.

## Decision

Sleep corrections made before enrollment participate in the same current sync
contract as corrections made through connected review. The local form freezes the
source version and earlier edit shown. Saves carry stable contract-valid IDs and
reviewed parent IDs; a stale form cannot silently claim review of a changed record.
Original source versions remain immutable, so their saved modification instant is
the reviewed source revision when the correction is encoded for upload.

One manual-correction encoder serves both local handoff and connected review.
The full sync exchange selects up to 100 local corrections per attempt, includes
their required source observations, and uses the existing durable outbox, pull,
acknowledgment and retry path. This includes corrections whose source has aged out
of the recent Health Connect snapshot. A missing source after server restore can
be reconstructed from its retained local evidence. No separate upload mode,
historical conversion or compatibility schema is added.

Enrollment text explicitly includes saved corrections and their source evidence.
Health Connect permission still gates new collection. Previously saved corrections
can sync after enrollment without another provider read. Sample corrections have
no eligible Health Connect source and never enter the queue. Medication sync is
outside this change and remains explicitly incomplete.

Time-zone holds are durable metadata tied to the attempted home zone. Held records
cannot block later eligible corrections; a different configured zone makes them
eligible for reconsideration. The UI reports held local corrections separately.
Erasure removes local correction payloads and their pending copies, preventing
enrollment or retry from recreating them.

Local correction order uses an explicit SQLite append sequence, which survives
compaction and clock changes. Creation time remains provenance, not an ordering
claim. A saved correction and its parent validation are transactional. The local
projection reads the saved head after a write, including an idempotent retry.
Timestamp inputs preserve seconds, nanoseconds and offsets.

## Verification

Android unit/lint/build checks and 21 disposable emulator tests pass. The native
handoff test saves a correction before enrollment, closes/reopens the database,
ages its source out of the current snapshot, and synchronizes it against an actual
Go API. A newer source and an unseen remote edit remain reviewable conflicts until
an explicit resolution; they are not silently replaced. SQLite tests cover clock
regression, stale parents, replay, missing-source recovery, held-page progress,
changed home zones, erasure and sample exclusion.

The integration fixtures erase their remote records after each case. An older
synthetic sleep record otherwise correctly caused the estimator's ambiguous-cycle
refusal in a later test. Test isolation was fixed; the estimator's refusal gate was
not weakened.

## Remaining completion work

The desktop lifecycle audit found implementation work still required: the app
constructs a memory-only activity sink and starts collection without a persisted
consent setting. Wire permission-gated durable collection and background execution
before claiming hidden/resume/startup qualification. C1 and subsequent milestones
remain active; these synthetic Android checks do not establish real-device pilot
or release readiness.

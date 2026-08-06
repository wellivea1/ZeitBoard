# ADR 0032: Android sleep synchronisation

- Status: accepted
- Date: 2026-08-06
- Implements slice 2 of phase P7 in [`phase-goals.md`](../phase-goals.md), the
  change the [automaticity review](../automaticity-review-2026-08-04.md) rated
  highest-value and [ADR-0031](0031-evidence-freshness-and-shadow-inference.md)'s
  validation gate independently pointed at.
- Reuses [ADR-0015](0015-device-sync.md) enrollment, [ADR-0013](0013-append-only-sleep-model.md)
  append-only corrections, and [ADR-0017](0017-server-erasure-tombstones.md) erasure.

## Context

The Android companion could read Health Connect sleep and store it durably, and
that was where it stopped. The device most likely to hold fresh wearable sleep
could not reach the estimator, so the desktop's rhythm still depended on the
user typing what a watch already knew.

ADR-0031's validation gate then made the case sharper from the other direction.
Desktop inactivity has an accuracy ceiling set by user habits, not by the
inference rules: a two-hour wind-down or a two-hour delay before touching the
machine after waking destroys a boundary no algorithm recovers. A source that
observes sleep directly does not have that ceiling.

## Decision

1. **A revised source record becomes a correction, not a second observation.**
   The record id is derived from the *logical* source key and stays stable
   across revisions. Pushing a new observation on each revision would leave the
   server holding two episodes for one night, and the drift fit indexes by
   cycle, so an extra episode shifts it. The revision supersedes through the
   append-only chain instead, keeping the original evidence.

2. **A record is never labelled with a time zone that contradicts its own
   evidence.** Health Connect stores UTC offsets; a v1 observation requires an
   IANA zone. Rather than inventing one, an episode syncs only when its stored
   offset matches the configured home zone at that instant. Episodes that
   disagree — typically travel — are **held back and counted**, not guessed at
   and not silently dropped. The check consults the zone's rules per instant, so
   it follows daylight saving rather than comparing against a fixed offset.

3. **Record ids are hashed, not sanitised.** A source package name can contain
   anything; the server's identifier rule cannot. Hashing keeps every id valid
   whatever the source is called, and keeps a vendor package name off the wire.

4. **Map, enqueue durably, then push.** Mapping straight into a request would
   lose evidence whenever the process is killed mid-flight, which on Android is
   routine rather than exceptional. The outbox shares the app database so an
   erasure removes a queued record and the episode it came from together.

5. **Idempotency is established at the source.** The outbox remembers the
   revision last accepted per record id, so a repeated pass over unchanged
   episodes produces nothing at all rather than relying on the far end to
   deduplicate. A retry after a lost response is safe for the same reason.

6. **Sync states are distinguishable.** `OFF`, `QUEUED`, `SYNCING`, `SYNCED`,
   `ERROR`. Collapsing them into a spinner would hide a backend that has been
   unreachable for a week. On failure the queue stays intact and the last
   successful time stays visible, because "nothing since Tuesday" is the useful
   fact rather than the transport error.

7. **Re-enrolling against a different instance clears the queue.** Records
   captured for one server must not be delivered to another.

8. **Android still runs no estimator.** It observes and forwards. The rhythm is
   computed once, in the shared Go core, so two devices cannot disagree about
   what the user's rhythm is.

9. **The token lives outside the app database.** A database export is something
   a user may reasonably hand to a clinician; a bearer token is not health data
   they intend to share. Keeping them apart means an export cannot carry it.

## Consequences

- Travel records do not sync until the observation contract can carry true
  offsets. They are visible as held rather than missing. That contract change
  is the natural follow-up and is not attempted here.
- `source_conflict` is the correction reason used for a source revision. None of
  the four contract reasons says "source revision"; this is the closest, and the
  wording gap is real.
- The Kotlin builds payloads by hand and nothing in the Go build imports it, so
  the two can drift silently. `apps/server/internal/sync/android_contract_test.go`
  pins the exact payload shape as a fixture.

## Verification

Unit tests cover the mapping rules, the DST-aware offset check, revision
handling, idempotency, batching, failure recovery, and the local-only mode.

The chain was also run end to end against a real daemon with the app installed
on an emulator: enrollment, a push of Android-shaped records, an idempotent
replay accepting zero, and a twelve-episode history producing a server estimate
of `+50 minutes per observed sleep cycle` — the rhythm that was generated.

That run found a defect no unit test on either side could see. The Kotlin sent
`acquisition_method: "device_sensor"`, which is outside the server's closed
enum, and the first push was rejected as an invalid batch. It is now
`health_connect`, and the Go-side fixture test fails if it changes again.

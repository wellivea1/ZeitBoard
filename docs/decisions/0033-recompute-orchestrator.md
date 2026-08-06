# ADR 0033: The recompute orchestrator

- Status: accepted
- Date: 2026-08-06
- Implements slice 6 of phase P7 in [`phase-goals.md`](../phase-goals.md).
- Finishes what [ADR-0031](0031-evidence-freshness-and-shadow-inference.md)
  started: it names "no recompute orchestrator exists" as its outstanding gap,
  and its own freshness decision does not take effect without one.
- Corrects the materialization behaviour recorded in
  [ADR-0029](0029-availability-portal-foundation.md) §5.

## Context

Analysis ran when somebody looked. The desktop recomputed on a screen load; the
server republished the shared projection inside whatever HTTP request happened
to arrive. That is enough while a person is watching and wrong the rest of the
time, which is most of the time, and for a disorder whose whole difficulty is
that the day drifts away from the clock, "the rest of the time" is the part that
matters.

ADR-0031 then made the shape of the problem sharper. It moved the freshness
decision to materialization, which was the right place: the withholding is
decided on the age of the evidence, before anything is published. But that makes
the decision *conditional on a materialization happening*, and a materialization
only happened when something else caused one. The policy's most useful rule —
sleep was expected by now and none has been recorded — fires for a user who is
recording nothing, and a user who is recording nothing causes no pushes, no
edits, and no materialization. The rule was correct and unreachable.

Three smaller defects sat in the same place.

`refreshPortal` ran on the request's context, so a device that hung up mid-push
cancelled the recomputation its own data had just made necessary.

Every push paid for a full recomputation, and a device sync arrives as a burst.
Twelve records pushed one at a time meant twelve estimates over the same history.

And `generatedAt` moved on every materialization, including ones caused by data
that says nothing about sleep. The rendered page and the JSON DTO both key their
staleness warnings on the age of that field, so pushing a shopping-list task
reset the visitor's only signal that the estimate was old. ADR-0031 identified
this and fixed only the status; the stamp itself still moved.

## Decision

1. **Durability by reconciliation, not by queue.** There is no table of pending
   recomputes. A recompute is a pure function of its inputs, so nothing about a
   request is worth remembering; what is worth remembering is which inputs have
   already been processed. A restart fingerprints the current inputs, compares
   them with the last completed run, and re-derives the need from the data. A
   queue would add a way to lose work — a dropped message is gone — in exchange
   for nothing.

2. **Two fingerprints, because there are two questions.** The *input*
   fingerprint answers "would the analysis read the same thing", and is what
   makes a restart able to tell that work is unnecessary. The *content*
   fingerprint answers "would it publish the same thing", and decides the stamp.
   Neither includes anything derived from the current instant; one that moved
   with the clock would report a change on every run and make the mechanism
   decorative.

3. **An unchanged projection keeps the time it last changed.** This is the
   correction that makes a freshness signal mean something. `generatedAt` is now
   *when the content last differed*, so its age tracks the evidence rather than
   the housekeeping. A page can no longer read "updated just now" over week-old
   records because something unrelated synced.

4. **Publishing happens on every run anyway.** Skipping the write when nothing
   changed would be a small saving and a real hazard: the published state would
   depend on what this process happens to remember, so a portal database restored
   from a backup could never be rebuilt. Instead the write is made idempotent —
   the stamp is a function of the content, so a repeat rewrites the same answer
   rather than announcing a new one.

5. **A result reports when it expires, and the loop wakes for it.** The freshness
   policy is a function of time, so `freshness.NextChange` returns the earliest
   instant its verdict could change with no new evidence at all. The guarantee is
   one-sided by design: never later than the real transition, sometimes earlier.
   An early wake republishes the same answer and costs milliseconds; a late one
   leaves a claim standing that the evidence no longer supports.

6. **A burst is coalesced, and a trickle is not starved.** The first request in
   a burst sets the deadline and later ones do not push it back. Extending on
   every request is the obvious implementation and it never runs at all while
   requests keep arriving — the failure mode that looks exactly like the feature
   working.

   The pending flag is cleared when a run **begins**, not when it finishes. The
   other way round drops every request that arrives mid-run, which is when they
   arrive, and those requests are not redundant: the run may have read its inputs
   before the change they report landed. This was a real defect in the first
   implementation and is recorded in `verification.md`.

7. **Emission is keyed, not counted.** `Apply` receives a stamp carrying the
   run's input fingerprint. Nothing in this slice emits a proposal, but when
   something does, its record id derives from that key, so "cannot emit duplicate
   proposals" holds by construction rather than by each emitter remembering to
   check. The precedent is already in the private store: the visitor request
   bridge is idempotent because `portal_request_proposals.portal_request_id` is
   unique, not because the bridge counts.

8. **A stored snapshot contains no "now".** Waking windows are published
   unfiltered and unclipped; dropping past windows and clipping one already in
   progress are render-time rules, applied identically by `BuildView` and by the
   availability DTO. They previously ran at materialization, where they were
   correct only for the instant the row was written — a four-hour-old snapshot
   showed a start four hours in the past regardless. Moving them makes the
   visitor's view right at every instant *and* makes two snapshots comparable,
   which decision 2 needs.

9. **The journal is encrypted like everything else in that database.** A
   fingerprint is one-way, but it still answers "did they sleep at 04:12 on
   Tuesday?" for anyone willing to guess, and the reason the file is encrypted at
   rest is that a copy of it answers nothing. The journal is bounded at 200 rows:
   it is an operational record on somebody's own machine, not an audit trail with
   a retention obligation.

10. **The scheduler reaches nothing.** No LLM, no provider client, no network,
    no subprocess — asserted on the package's own import graph, not by
    convention. A loop that runs with no screen, no user and no obvious cost is
    the easiest place in the system to add an assistant call and the hardest
    place to notice one.

## Consequences

- A push no longer guarantees that the projection is current by the time the
  response returns. Nothing was reading it that fast, and the guarantee cost a
  full recomputation per record.
- Creating a share link still waits, because the recipient would otherwise open a
  blank page. It is the only synchronous path, and it goes through the same
  orchestrator so there is exactly one way to publish and one idea of when the
  content last changed.
- `MaterializeAll` is gone. Two publication paths would mean two stamps, and the
  snapshot store's version guard would let the wrong one win.
- The daemon now runs two background workers: this one for analysis, and the
  existing maintenance worker for expiry sweeps and the request outbox. They stay
  separate because they are idempotent for different reasons — this one by
  fingerprint, the bridge by unique key — and because a slow analysis run should
  not delay a retention sweep.
- **The desktop is unchanged.** It recomputes on screen load, which is correct
  for a foreground application; the background guarantee lives in the daemon,
  which is the part that runs with no window. A desktop that is closed computes
  nothing, and that is not something an orchestrator can fix.
- With the portal disabled there is nothing to publish, so no worker starts.
  The analysis loop currently has exactly one consumer.

## What this does not do

- **It does not make the desktop recompute in the background.** ADR-0031's
  activity collector still runs only while the app runs; that half of slice 3
  remains open and this changes nothing about it.
- **It emits no proposals.** Decision 7 provides the key an emitter would use and
  nothing uses it yet.
- **The heartbeat is a backstop, not a mechanism.** An hour without a run
  triggers a recheck so that state drifting underneath the process is repaired
  without a restart. Nothing should depend on it.

## Verification

`core/recompute` and `core/freshness` carry the decision logic and their own
suites: burst coalescing, the starved-trickle case, the minimum-interval floor,
failure backoff, expiry scheduling, the input fingerprint's order- and
clock-independence, the length-prefixing that stops two histories colliding,
reverting inputs (A → B → A must still republish), a failed apply not becoming
the new truth, and restart recovery. `NextChange` is checked by walking the clock
forward a minute at a time and asserting the state never changes before the
predicted instant, plus one test at nanosecond resolution for the strict
comparison in the overdue rule.

`apps/server/internal/analysis` covers the end the user sees: a quiet day
withholding availability with nobody pushing anything, an unchanged projection
keeping its stamp, new evidence moving it, a new link not restamping everybody
else's page, published windows not depending on the instant, and a real worker
goroutine coalescing a burst into one publication.

The chain was also run against a live `zeitboardd` with the portal enabled:

- twelve single-record pushes produced one recomputation —
  `analysis: recompute (evidence) absorbed 11 further request(s)`
- availability appeared on the public page with no request driving it
- an unrelated task push was accepted and left `generatedAt` unmoved, which is
  the ADR-0031 defect this ADR finishes closing
- a restart reconciled to the same content and did not restamp
- erasing the twelve records withdrew the projection — `status: refused`, zero
  windows — again with nothing refreshing anything

The freshness-expiry wake is covered by tests rather than by the live run,
because observing it in a real daemon means waiting hours for a real threshold.

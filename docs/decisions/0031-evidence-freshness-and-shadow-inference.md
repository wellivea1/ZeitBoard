# ADR 0031: Evidence freshness, real activity evidence, and shadow sleep inference

- Status: accepted
- Date: 2026-08-04
- Implements the first three slices of phase P7 in [`phase-goals.md`](../phase-goals.md),
  redirected by [`automaticity-review-2026-08-04.md`](../automaticity-review-2026-08-04.md).
- Corrects a defect in [ADR-0029](0029-availability-portal-foundation.md).
- Inherits [ADR-0022](0022-local-sleep-import-and-real-history-validation.md)'s
  measured-delta gate, which now also governs inferred episodes.

## Context

The project could only learn that the user had slept if the user said so. Three
surfaces nonetheless made confident current-state claims, and two of them were
wrong in the same way: they measured how recently the *analysis* ran rather than
how recently the *evidence* arrived.

The desktop set "Likely awake" as the default whenever the newest stored sleep
interval did not contain now. Because it recomputes on every screen load,
nothing ever aged out — the claim survived exactly as long as the user went
without recording anything, which is precisely when it is least true.

The availability portal appeared to have solved this and had not. `GeneratedAt`
is when the snapshot row was written, and materialization runs after *any*
accepted sync push, including a task push that says nothing about sleep. Pushing
a task refreshed the freshness signal while the sleep evidence underneath was
days old, and the page read "updated just now".

Meanwhile the desktop activity collector emitted one observation saying
`startup` and then blocked forever, while being named `SafeCollector` in a
package called `activity`.

## Decision

1. **One freshness policy, keyed on evidence age.** `core/freshness` decides
   whether a current-state claim may be made at all, and the desktop, the
   server projection, and the portal materializer all use it. Evidence age is
   measured from when a record was *recorded*, not when the sleep it describes
   occurred: a record entered today about last week is fresh evidence, and a
   week-old record of last night is not.

2. **An unrecorded expected sleep withholds the claim.** This is the case a
   pure age threshold cannot catch, and it is how "Likely awake" survives one:
   the last wake is old but not old enough to trip a 24-hour rule, while the
   sleep that must have happened since is simply absent. The grace period is six
   hours because ADR-0022 measured a 5.41 h P90 onset error, and a shorter one
   would withhold on entirely correct behaviour.

3. **Withholding happens at materialization, not at rendering.** The portal
   decides on evidence age before publishing, so the public DTO does not change
   and no new field crosses the boundary. `generatedAt` keeps its honest
   meaning: when the row was written.

4. **The server projection gets the corrected state string and no new field.**
   A structured freshness block would be a new field on a strict v1 response,
   and the desktop's own backend client decodes with `DisallowUnknownFields` —
   exactly the "existing strict consumers would reject the payload" case that
   `contracts/README.md` reserves for a new contract version. The correction
   that matters does not need the field; carrying the *reason* to a synced
   client does, and waits for v2.

5. **Activity evidence is a closed set of behavioural states.** Startup,
   active, idle, locked, unlocked, suspended, resumed, shutdown — each with a
   time and how long the previous state lasted. The recorded shape is the
   privacy argument: there is no field for an application, a window title, a
   document, or a keystroke, so widening it means changing the type and the
   commitment that constrains it rather than adding a line somewhere. A test
   asserts no payload key suggests content capture.

6. **State is read through two narrow calls.** Time since last input, which
   cannot reveal what the input was, and whether the interactive desktop is
   locked. Lock state uses `OpenInputDesktop` rather than session notifications
   because a message loop would be a larger surface for no additional evidence.
   Suspend and resume are *inferred* from wall-clock gaps rather than observed,
   and the source does not claim a power-event capability it lacks.

7. **Judgement lives in a pure state machine.** Idle is backdated to when input
   stopped rather than to the poll that noticed, because up to a whole threshold
   of error lands directly on a sleep boundary. A clock jump beyond five poll
   intervals is recorded as suspend/resume rather than idle, because the process
   was not running and calling it idle would assert something never observed.
   Lock takes precedence over idle, so a deliberate lock does not read as
   walking away mid-task. An ordinary hour of work produces no records at all.

8. **Inferred sleep is a hypothesis, and shadow-only.** `core/inference`
   produces candidates carrying start and end uncertainty, named supporting and
   conflicting sources, and an algorithm version. Desktop inactivity brackets
   sleep rather than marking it — someone stops using a machine before falling
   asleep and returns some time after waking — so the uncertainty is part of the
   claim. Nothing in the package may reach planning, the estimator, or any
   projection until a documented validation decision, on ADR-0022's gate.

9. **Disagreement is recorded, not resolved.** When two sources contradict each
   other about the same night, the conflict survives into the candidate so a
   person can be asked later. Silently preferring one source would discard the
   only signal that something is wrong.

10. **A shutdown does not open a quiet interval.** The machine being off says
    nothing about the person, and treating it as evidence would turn every
    holiday into a fortnight of sleep.

## Consequences

- The desktop and portal now say "Unknown" in situations where they previously
  said "Likely awake". That is the point, and it will look like a regression to
  anyone who has not read this ADR.
- The portal leak-test fixture described someone who had not slept in 2.3 days
  and was rebuilt backwards from now, because the policy correctly refused to
  publish it.
- Non-Windows builds get an activity source that asserts nothing rather than
  fabricated idle values, so a Linux build cannot contribute evidence it never
  measured.
- `GetLastInputInfo` truncates to 32 bits and wraps about every 49 days, so the
  subtraction is done in 32-bit space; the natural way round reports a 49-day
  idle time shortly after each wrap.

## What this does not do

- **The collector is not yet wired into a background service.** It runs while
  the app runs. Reliable collection across a hidden window is P7 slice 3's
  remaining half.
- **Inference is not connected to storage or to a correction prompt.** It
  computes candidates from supplied intervals; persisting them, showing them,
  and asking the user about conflicts are separate slices.
- **Android still cannot sync**, which remains the single highest-value gap.
  Health Connect sleep still cannot reach the estimator.
- ~~**No recompute orchestrator exists.**~~ Closed 2026-08-06 by
  [ADR-0033](0033-recompute-orchestrator.md), which also finishes decision 3:
  withholding at materialization only takes effect if something causes a
  materialization, and a user who records nothing causes nothing — which is
  precisely the case this policy was written for. The daemon now recomputes when
  the verdict is due to change, not only when someone pushes. The desktop still
  recomputes on screen load, which is correct for a foreground application.
- **Desktop confidence labels are still rendered** despite ADR-0022's measured
  inversion. The freshness work makes the *state* honest; the confidence badge
  is a separate correction.
- **Detecting desktop use during an asserted sleep** needs the active side of
  the record, which `FromActivity` does not report. It is left undone rather
  than approximated.

## Validation gate outcome (2026-08-04)

The gate decision 8 requires has now been measured, and it is **negative**:
inferred episodes stay shadow-only. Five of ten synthetic scenarios meet the
pilot targets. The full table is in `verification.md`.

The result is more useful than the pass count. The error is systematic — desktop
inactivity brackets sleep by 33–49 minutes before onset and 18–30 minutes after
wake — rather than noisy, so it is a correctable offset. The scenarios that fail
badly fail on user behaviour rather than on the rules: a two-hour wind-down or a
two-hour delay before touching the machine after waking destroys a boundary no
interval logic can recover. And nothing was invented: zero false positives
throughout, with coverage degrading honestly when evidence is missing.

Correcting the bias against these numbers is explicitly refused. The measured
offset is close to the generator's own behaviour parameters, so fitting to it
would measure an assumption back. Calibration must come from live data against
user-confirmed boundaries.

## Verification

`core/freshness` and `core/inference` carry their own suites, including the
unrecorded-expected-sleep case, the grace period against measured onset error,
and the payload allowlist. `docs/verification.md` records the measured results
and what remains unmeasured.

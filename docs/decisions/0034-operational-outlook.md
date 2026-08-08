# ADR 0034: The 48–72 hour operational view

- Status: accepted
- Date: 2026-08-08
- Implements slice 8 of phase P7 in [`phase-goals.md`](../phase-goals.md), the
  last one, and the first that is about what the user sees rather than what the
  system knows.
- Reads the shared freshness policy from
  [ADR-0031](0031-evidence-freshness-and-shadow-inference.md) and is kept current
  by [ADR-0033](0033-recompute-orchestrator.md).
- Uses the scheduler and the measured accuracy budget from
  [ADR-0022](0022-local-sleep-import-and-real-history-validation.md).

## Context

Everything before this slice made the system know things. This one asks it to
answer the question the product exists for: over the next two or three days,
when will I be awake, and what can I actually do in that time.

For someone with a 24 h 50 m day that is not a question a calendar answers. The
waking hours walk about an hour later each day and right around the clock over a
fortnight, so "can I ring the pharmacy this week" has a real answer, it changes
daily, and working it out in your head while tired is exactly the burden the app
is supposed to remove.

The desktop already had the ingredients and none of the dish. Overview reported
one "useful task window" — a single interval, today. Calendar showed imported
events against predicted bands. Tasks listed tasks. Nothing put a rhythm, a
working week, a set of commitments and a list of things to do on the same axis.

## Decision

1. **Three states, not two.** The estimator does not predict a sleep onset; it
   predicts an envelope that widens with forecast distance, and its sleep and
   waking envelopes deliberately overlap. An instant inside both is one where the
   model does not know which side of the boundary it is on. The timeline
   therefore has **awake**, **asleep**, and **uncertain**, and the uncertain band
   is drawn as a band.

   Collapsing that into a two-state timetable would be the single most
   misleading thing this feature could do. ADR-0022 measured a 5.41 h P90 onset
   error; a crisp line would be hours wrong several times a month while looking
   exactly as confident as a correct one.

2. **The current period is not a forecast gap.** The estimator's first waking
   window is the one *after* the next sleep, so the period the reader is standing
   in has no window of its own. Left alone the next several hours — the most
   useful part of the whole view — read "unknown". The view fills it from now to
   the earliest plausible onset, which is where the next sleep envelope opens,
   and hands over to the envelopes after that.

3. **Office hours are the headline, not a detail.** "When am I awake while
   somewhere is open" is the question a drifting rhythm makes genuinely hard and
   that no other tool answers. Each office-open stretch reports two numbers that
   are **never added together**: `reachable`, the overlap with confidently awake
   time, and `possible`, the overlap with the uncertain band. A window with only
   `possible` time is shown as such and names no time to ring, because a plan
   made on an unpinned boundary is a plan made on arithmetic.

4. **Tasks are placed only in confidently awake time.** The scheduler is fed the
   awake segments, not the full predicted waking envelope it used to get.
   Suggesting a task for a stretch where the model does not know whether the
   person is up is how a suggestion lands mid-sleep. This makes the desktop's
   suggestions strictly more conservative than before, on purpose.

5. **A commitment that lands in predicted sleep is named.** For a rhythm that
   drifts, anything booked more than a few days out has a real chance of falling
   inside sleep, and being told so days ahead is the difference between moving an
   appointment and missing it.

6. **Stale evidence withholds the whole view, not just a label.** A forecast is
   anchored to where the person is *now* in their cycle. When the shared policy
   will not support a current-state claim there is no anchor, and the next three
   days are a shape rather than a plan. Drawing office windows over it would
   invite someone to book a call against arithmetic. The refusal says which kind
   of nothing it has: no estimate at all, or an estimate with no anchor.

7. **The core carries no private text.** `core/outlook` works on event *ids* and
   returns them; the desktop joins titles back locally. The view is local, so
   titles belong on the screen — but a package that never sees them cannot leak
   them into a projection later, and this one never sees them.

8. **It lives on Overview, not on a ninth destination.** The
   [UI guideline review](../ui-guideline-review-2026-08-04.md) found eight
   equal-weight destinations already too many for someone operating under
   fatigue. Overview is the screen a person opens; this is what they open it for.
   It replaces the single "useful task window" fact, which it subsumes, so the
   screen gains one section and loses one tile.

9. **Nothing here is medical.** Everything is derived from recorded sleep-wake
   times and civil-clock arithmetic. No circadian phase claim, no DLMO, no
   judgement about fitness to drive or work, and no advice about what to do with
   any of it.

## Consequences

- The desktop's task suggestions will sometimes place later than they used to,
  or refuse where they previously offered a window on the edge of the envelope.
  That is decision 4 working.
- The view is computed from local records even when backend sync is on. The
  synced projection is a v1 contract of rendered strings, not predicted windows,
  so the server cannot supply the envelopes this is built from. Sync moves the
  underlying records, so the local store holds the same evidence; it is a
  question of which contract to read, not of which device knows more.
- Office hours are **not yet configurable**: Monday to Friday, 09:00–17:00 local,
  matching the scheduler's own business-hours default so the two cannot
  disagree. `outlook.OfficeHours` is a parameter with a documented default, so
  wiring a setting later is a settings-surface change and nothing else. For a
  shift worker or a different jurisdiction the current defaults are simply wrong,
  and that is a real limitation rather than a deferred nicety.
- The uncertain band is hatched rather than solid. The three state colours are
  separated by hue but sit close in luminance, so three solid blocks are hard to
  tell apart in greyscale, in the amber-glasses theme, or for a reader with a
  colour vision deficiency. Stripes also *mean* the right thing.

## What this does not do

- **No assistant surface.** P7's invariants forbid a new assistant action, and a
  read tool is close enough to that line to wait for a decision rather than
  assume one.
- **No portal exposure.** A visitor already sees waking windows; office overlap
  and commitments are not on the public allowlist and are not proposed for it.
- **It does not measure whether the view helps.** Every acceptance test here
  shows the software does what it claims. Whether the user rings more pharmacies
  is the pilot question recorded in `verification.md`.

## Verification

`core/outlook` carries the arithmetic and its own suite: the timeline covers the
horizon with no gaps and no unmerged neighbours; every boundary has an uncertain
stretch and those stretches widen with distance; the current period is awake
rather than unknown and ends at the earliest plausible onset; a recorded episode
overrides the forecast and is marked observed; office windows separate reachable
from possible and never exceed their own span; the weekend is skipped; a
commitment inside predicted sleep is named; tasks land only in awake segments and
an unplaceable one says why; stale evidence withholds everything; a refusal keeps
the estimator's own code; the horizon clamps to its band; and the whole thing
survives a daylight-saving transition — the sweep runs on absolute instants while
the office day moves with the civil clock.

One test drives a seeded 40-day generator through a fortnight of drift and
asserts that office hours are reachable on some days and not others. A view that
reported the same answer every day would not be reporting anything.

The desktop binding is tested end to end against a real store: an available view
with usable layout offsets, all three presence states present, withholding on
four-day-old evidence, refusal with no history, honest office labelling, and no
estimator internals in the DTO.

The rendered panel was checked in the running app: thirteen contiguous bands
across a 72-hour strip, day marks at 24-hour intervals, legend swatches matching
the band fills including the hatch, office icons coloured by status, no console
errors, and no horizontal overflow.

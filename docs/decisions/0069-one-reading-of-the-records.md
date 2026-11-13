# ADR 0069: One reading of the records, on every surface

- Status: accepted
- Date: 2026-11-13
- Builds on [ADR-0022](0022-local-sleep-import-and-real-history-validation.md),
  [ADR-0031](0031-evidence-freshness-and-shadow-inference.md),
  [ADR-0034](0034-operational-outlook.md) and
  [ADR-0029](0029-availability-portal-foundation.md).

## Context

Two reviews on 2026-11-13, one made in the running app and one by an outside reviewer reading the
code at `890b30d`, found the same kind of fault from different sides: parts of ZeitBoard drew
different conclusions from the same sleep records. Each was reproduced before anything changed.

- **The planner was surer than Home.** With the newest sleep 64 hours old, Home said nothing about
  the present and the outlook was withheld, while the planner offered a time "in a predicted
  waking window". It asked only whether an estimate existed.
- **The planner and the shared page read the estimator's windows as they stand.** The windows are
  envelopes, and a sleep window overlaps the waking windows on both sides. The outlook already
  reads the overlap as uncertain. The planner offered tasks in it, and the shared page said
  "Likely awake right now" through it.
- **An excluded record refreshed the evidence.** The estimator ignores a night the owner excluded.
  The age of the evidence did not, on the computer or on the server. Adding a recent record by
  mistake and excluding it put a three-day-old forecast back on the shared page as current.
- **Cycles were counted against 24 hours.** A gap between two recorded sleeps is a whole number of
  cycles. Counted against the day, a rhythm of 27 hours lost its place whenever nights went
  unrecorded: two missing nights (81 hours) were refused, and five (162 hours) read as seven
  cycles and bent the fit to about 25 hours 43 minutes. A refused gap stayed refused until it left
  the 21-night window, about three weeks.
- **A long nap read as a cycle.** The phone reports every Health Connect session as main sleep. A
  three-hour sleep twelve hours after a night's start stopped the estimate.
- **Reports of one sleep were merged two at a time.** Each result was averaged with the next
  report, so later reports weighed more, and an automatic source could move a night the owner had
  confirmed while the result still read as confirmed.

## Decision

1. **"Awake" has one definition, and it is the outlook's.** `outlook.ConfidentWaking` gives the
   time inside a predicted waking window and outside every predicted sleep window and every
   recorded sleep. The planner, the assistant's planning context and the approval check plan in
   that time alone. The shared page publishes each waking window without the parts a sleep window
   also covers.
2. **The planner keeps Home's freshness.** On records too old for a current-state claim it offers
   no time, and lists each task with the reason and the step that brings suggestions back.
3. **Excluded records are not evidence of the present.** The freshness inputs skip them, on the
   computer and on the server.
4. **Cycles are counted in the rhythm's own period.** The period is the middle of the gaps that
   can only be one cycle (16 to 36 hours); with fewer than three such gaps it is the day, as
   before. The period is not refined from the counted starts: with few nights, one miscounted gap
   moves a fitted period far enough to make its own miscount look right.
5. **A gap that cannot be counted is left behind.** Once seven nights are recorded since it, they
   are fitted alone, and the estimate's reasons say so. Until then the refusal says how many
   nights are recorded since the gap.
6. **One sleep start opens each cycle.** Of two main sleeps that begin less than 16 hours apart,
   the longer is the cycle's sleep. The other stays in the record and out of the fit. Two records
   of the same start are still refused as a duplicate.
7. **Overlapping reports merge as a group.** The merged times are the middle of the group. Where
   any report is user-confirmed at both ends, the confirmed reports alone decide the times and the
   result reads as confirmed; otherwise it reads as imported. Every report's identifiers are kept.
8. The algorithm version is `theil-sen-sleep-start/v2`.

## Measured on the owner's history

ADR-0022's gate holds estimator changes to a measured result. The aggregate backtest was run on
the same rebuilt validation database before and after (953 observations, 2021 to 2023; the
database and its per-night detail stay private):

| | Forecasts | Refusals | Coverage | Median error | Mean error | P90 error | Hit rate | Mean window |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Before (v1) | 809 | 136 | 0.856 | 1.71 h | 2.40 h | 5.41 h | 0.78 | 14.71 h |
| Cycles counted in the rhythm's period, gaps left behind | 909 | 36 | 0.962 | 1.71 h | 2.38 h | 5.36 h | 0.80 | 14.68 h |
| And one sleep start for each cycle | 921 | 21 | 0.978 | 1.72 h | 2.38 h | 5.33 h | 0.80 | 14.68 h |

The group merge changed nothing here, since this history holds its overlapping sources out.

A hundred and twelve more nights get a forecast, and the forecasts are as accurate as before.
The confidence buckets are unchanged in kind: High still does not rank above Medium, so the
labels stay withheld from shared pages and qualified on Home.

## Consequences

- A person on a long cycle keeps a forecast through ordinary missing nights, and gets one back a
  week after a gap that cannot be counted, where it took three.
- Fewer task suggestions are offered: none in the hours around a predicted sleep, and none on old
  records. The reason is said in both cases.
- The shared page says "Likely awake right now" for fewer hours, and those hours are the ones Home
  also draws as awake.
- A night the owner corrected keeps its times when a wearable reports the same night.

## Not decided here

These came from the same reviews and need the owner's choice or a device, so they are recorded
in `usability-review-2026-11-13.md` and not built:

- a reported current status ("trying to sleep", "awake", "do not disturb") that syncs and expires;
- freshness thresholds that follow the cycle length, so a healthy 27-hour source is not read as
  stale near the end of each waking day;
- Health Connect sessions the provider deleted, and sessions recorded in another time zone;
- how sleep in several pieces counts towards the typical duration;
- the portal's rate limit behind a reverse proxy.

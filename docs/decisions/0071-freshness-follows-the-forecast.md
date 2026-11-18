# ADR 0071: Freshness follows the forecast

- Status: accepted
- Date: 2026-11-17
- Revises the freshness thresholds of [ADR-0031](0031-evidence-freshness-and-shadow-inference.md).
- Chosen by the owner on 2026-11-17, from four options.

## Context

ADR-0031 made one freshness rule for every surface: the desktop's Home, the server's companion
projection, the local agent and the share page. It counted hours from the newest sleep record.
After 6 hours Home said "Current state uncertain". After 24 hours, or 6 hours after a forecast
sleep should have begun with none recorded, nothing was shown at all.

Those hours assumed a source that reports soon after each change. A watch reports a night only
once it is over, and the phone reads it some time after that. On a 27-hour day the newest
record is the last wake, so the caution started six hours into every waking day, and the share
page went blank in the second half of every sleep. The longer the owner's cycle, the more of
each day was spent "uncertain" while the forecast was going exactly as predicted.

The question the rule should ask is not how old the newest record is. It is whether a record
that should have arrived has not.

## Decision

1. **While the day goes as forecast, the records are current.** `core/freshness` takes the
   first predicted sleep window after the newest recorded sleep: its earliest likely onset and
   its latest likely wake. Until that latest likely wake has passed, the state is current,
   whatever the cycle length.
2. **A record that is due and missing brings a caution, then nothing.** Once the latest likely
   wake has passed and no record of that sleep has arrived, the state is stale with the reason
   `sleep_record_due`: "A sleep was expected to be over by now and no record of it has arrived
   yet". Six hours later (`RecordOverdueGrace`) it is withheld with `expected_sleep_unrecorded`.
   The grace absorbs a watch that syncs late and ordinary forecast error; ADR-0022 measured a
   5.41-hour P90.
3. **Sleep recorded since the forecast discharges it.** A sleep recorded at or after the
   forecast onset means the person slept and the estimate has not caught up. Until it does, the
   fixed hours of ADR-0031 decide, from the newest record.
4. **With no forecast, the fixed hours stand.** When the estimator refused, or there are records
   but no forecast, 6 and 24 hours from the newest record still apply.
5. **A page keeps its own deadlines.** The policy reports both instants, `Deadlines`, and each
   share-page snapshot stores them. The page adds its caution and withholds its windows at those
   instants itself, so a claim cannot outlive them if nothing republishes. A snapshot without
   them falls back to the fixed hours from when it was made.
6. **One rule everywhere still.** The desktop, the server's projection, the assistant snapshot
   and the share page all call the same policy, as ADR-0031 required. `sleep_record_due` joins the
   assistant snapshot's reasons.

## Consequences

- On a 27-hour day with a watch, Home reads plainly through the whole waking stretch and the
  sleep after it, and the share page shows the forecast through both.
- A night that was never recorded is noticed later than before: at the forecast's latest wake
  rather than six hours after its onset. The page says nothing wrong in between, since it shows
  a forecast sleep window for those hours, not "awake".
- `TestA27HourDayIsSharedForAsLongAsItGoesAsForecast` walks a simulated 27-hour history
  through the materializer and checks each turn; `core/freshness` and the portal's view tests
  hold the rest.

## Not decided here

- Whether the grace should follow measured forecast error rather than stay at six hours.

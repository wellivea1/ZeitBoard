# ADR 0054: Sources say what they covered

- Status: accepted
- Date: 2026-10-11
- Implements "missingness recorded per source" from the roadmap's phase 2 and section 6.3 of the
  rhythm analysis specification. Builds on [ADR-0032](0032-android-sleep-synchronisation.md) and
  [ADR-0037](0037-android-automatic-upload-and-source-provenance.md) (Health Connect upload),
  [ADR-0048](0048-medication-and-marker-sync.md) (a sync kind defined once in `core`) and
  [ADR-0049](0049-assistant-snapshot.md) (the assistant snapshot).

## Context

When the newest sleep record ages, the desktop says so: "The newest sleep record is 3 days old."
It cannot say why. The specification asks for missing data to be represented explicitly, and
for these cases to be told apart:

- the sensor was unavailable;
- permission was revoked;
- the device was off;
- no event occurred;
- the import failed;
- the data are delayed.

Each case has a different remedy. The owner wears the watch, opens the phone, grants a
permission, or waits.

The phone knows most of this. Before each Health Connect read it checks whether Health Connect is
available and whether reading is permitted. It knows whether the read succeeded and which window
it covered. It also knows which records it held back because their offset disagrees with the
home zone. None of this left the phone: the desktop saw only the sleep records that arrived.

## Decision

1. **A source report is a sync record.** It is a device's statement about one source at one
   moment. The new `source_report` kind carries `contracts/v1/source-report.schema.json#/$defs/report`:
   - `source`: a closed set, `health_connect` for now;
   - `outcome`: what the device found;
   - `reported_at`;
   - `background_reads`: whether the device reads the source without being opened.

   A successful read also carries `covered`, the window read in full, and `held`, the number of
   records in that window the device read but did not send.

2. **The outcomes map onto the specification's cases.**
   - `unavailable`: Health Connect is not installed, or needs an update. This is the unavailable
     sensor.
   - `permission_missing`: the device may not read sleep. This is the revoked permission.
   - `read_failed`: the read was attempted and failed. This is the failed import.
   - `read`: the source was read in full over `covered`. Every sleep record it holds there has
     been sent or is queued, except the `held` ones. A night inside the window with no record is
     a night the source recorded nothing, so no event occurred as far as the source knows.
   - No report: a device that is off, or has not been opened, says nothing. The desktop words the
     silence as "not read since", because from afar the two look alike. With `background_reads`
     false, the phone reads only while it is open, which explains the delayed case.

3. **Reports are rare and carry no sleep.** The phone reports after a Health Connect check when
   anything it would report has changed. Otherwise it reports at most every six hours, so a
   healthy hourly read sends four reports a day. A report holds instants of checks and a count,
   never a sleep time. Reports are append-only and are never corrected.

4. **One definition.** `core/sources` holds:
   - the report type and its validation, used by the server's push validation and the desktop's
     store;
   - the status derivation: per source, the newest report from the device that reported last,
     since when its outcome has held, and the end of the newest read window.

5. **The desktop says it where data is judged.**
   - Data Sources shows each reported source on its own row, worded from its status: read when,
     permission off since when, unavailable, last read failed, or records held on the phone.
   - When the estimate is stale or withheld, Home and Week add one sentence naming the likely
     cause. For example: "The phone read Health Connect 2 hours ago and found no sleep after Tue
     7:40 AM." The shared freshness policy is unchanged. The server reads no reports, so the
     portal and the server projection keep the policy's own words.

6. **An assistant can ask the same question.** The snapshot's `sync` section lists each source's
   outcome, report time, end of the newest read window, background reads and held count. These
   instants describe checks, not sleep, so the snapshot's rule against exact times of recorded
   evidence holds.

7. **Not an estimator input.** Reports change no estimate, forecast or proposal. They are
   encrypted at rest on the owner's server like every record. The server validates them, stores
   them and passes them on, and reads none of them.

## Consequences

- "The newest record is 3 days old" now comes with its cause, when a report has one.
- The desktop and the companion must both know the kind before any phone pushes it. They ship
  together, as ADR-0048's kinds did.
- Deleting all sleep data on the desktop also deletes the reports kept there; the next report
  starts over.
- Other sources do not report yet. The manual log has no coverage to state: a night not logged
  is just not logged. A file import's range and the activity collector's running time are known
  on the desktop itself, and can be added to the same status model without a sync record.
- A device that reports nothing is indistinguishable from one that is off. Telling them apart
  would need a heartbeat, which this decision does not add.

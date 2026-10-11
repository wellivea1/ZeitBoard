import { useState } from "react";
import { notifySleepDataChanged } from "../data/sleepDataEvents";
import { suppressSleepEntry, type SleepEntry, type SleepOverlap } from "../data/sleepEntries";
import {
  civilClock,
  minutesWording,
  nightWording,
  sourceWording,
  type NightTimes,
} from "../utils/sleepWording";

// The nights more than one record describes, as the estimator merges them.
// Each record is drawn on its own line above the night the estimator uses in
// their place, all on one scale, so a disagreement shows before it is read.
// Excluding a record leaves it out; Undo on its row in the sleep log takes
// that back.

function recordTimes(record: SleepEntry): NightTimes {
  return {
    startLocal: record.effectiveStartLocal,
    endLocal: record.effectiveEndLocal,
    startLabel: record.effectiveStartLabel,
    endLabel: record.effectiveEndLabel,
  };
}

function apartWording({ startApartMinutes, endApartMinutes }: SleepOverlap) {
  if (startApartMinutes === 0 && endApartMinutes === 0) return "The same times";
  const start =
    startApartMinutes === 0 ? "Same start" : `Starts ${minutesWording(startApartMinutes)} apart`;
  const end = endApartMinutes === 0 ? "same end" : `ends ${minutesWording(endApartMinutes)} apart`;
  return `${start}, ${end}`;
}

interface Scale {
  from: number;
  span: number;
}

/** From the earliest start to the latest end of every night drawn. */
function scaleOf(nights: NightTimes[]): Scale | undefined {
  const starts = nights.map((night) => civilClock(night.startLocal)?.getTime() ?? NaN);
  const ends = nights.map((night) => civilClock(night.endLocal)?.getTime() ?? NaN);
  const from = Math.min(...starts);
  const span = Math.max(...ends) - from;
  return Number.isFinite(span) && span > 0 ? { from, span } : undefined;
}

function Bar({ night, scale }: { night: NightTimes; scale: Scale | undefined }) {
  const start = civilClock(night.startLocal)?.getTime();
  const end = civilClock(night.endLocal)?.getTime();
  return (
    <span className="sleep-overlap-track" aria-hidden="true">
      {scale && start !== undefined && end !== undefined && (
        <span
          style={{
            left: `${((start - scale.from) / scale.span) * 100}%`,
            width: `${((end - start) / scale.span) * 100}%`,
          }}
        />
      )}
    </span>
  );
}

function OverlapNight({
  overlap,
  busy,
  onExclude,
}: {
  overlap: SleepOverlap;
  busy: boolean;
  onExclude: (record: SleepEntry) => void;
}) {
  const night = nightWording(overlap);
  const scale = scaleOf([...overlap.records.map(recordTimes), overlap]);
  return (
    <li className="sleep-overlap">
      <div className="sleep-overlap-head">
        <strong>{night.day}</strong>
        <small>{apartWording(overlap)}</small>
      </div>
      <ul className="sleep-overlap-records">
        {overlap.records.map((record) => {
          const times = recordTimes(record);
          const time = nightWording(times).time;
          const source = sourceWording(record.provenanceLabel);
          return (
            <li key={record.observationId}>
              <span>{source}</span>
              <span className="sleep-overlap-time">{time}</span>
              <Bar night={times} scale={scale} />
              <button
                className="button ghost compact"
                type="button"
                aria-label={`Exclude ${time} on ${night.day}, ${source}, from estimates`}
                disabled={busy}
                onClick={() => onExclude(record)}
              >
                Exclude
              </button>
            </li>
          );
        })}
        <li data-merged>
          <span>Estimate uses</span>
          <span className="sleep-overlap-time">{night.time}</span>
          <Bar night={overlap} scale={scale} />
        </li>
      </ul>
    </li>
  );
}

export function SleepOverlapList({ overlaps, count }: { overlaps: SleepOverlap[]; count: number }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  if (count === 0 && !status) return null;

  const exclude = async (record: SleepEntry) => {
    setBusy(true);
    setError("");
    setStatus("");
    try {
      await suppressSleepEntry(record.observationId, record.reviewToken);
      notifySleepDataChanged();
      const night = nightWording(recordTimes(record));
      setStatus(
        `${night.day}, ${night.time} is left out of estimates. Undo is on its row in the sleep log.`,
      );
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The record could not be excluded.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="sleep-overlaps" aria-labelledby="sleep-overlaps-title">
      <h3 id="sleep-overlaps-title">
        Nights recorded more than once
        {count > 0 && <span className="count">{count}</span>}
      </h3>
      {count > 0 && (
        <p className="diff-note">
          The estimator uses the middle of their times. Exclude the record that is wrong to leave it
          out.
        </p>
      )}
      <ul className="sleep-overlap-list">
        {overlaps.map((overlap) => (
          <OverlapNight
            key={overlap.records.map((record) => record.observationId).join(" ")}
            overlap={overlap}
            busy={busy}
            onExclude={(record) => void exclude(record)}
          />
        ))}
      </ul>
      {count > overlaps.length && (
        <p className="diff-note">
          The newest {overlaps.length} of {count} are listed.
        </p>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {status && (
        <p className="form-status" role="status">
          {status}
        </p>
      )}
    </section>
  );
}

import { useCallback, useRef, useState, type PointerEvent } from "react";
import type { AccountActivity } from "../data/accountActivity";
import { axisHour } from "../utils/relativeTime";
import {
  DAY,
  RECENT_DAYS,
  figureHeight,
  lastDays,
  nightPath,
  rowLabels,
  spanWords,
} from "./activityNightsLayout";
import { TimeProbe } from "./TimeProbe";
import { civilProbeLabel, useTimeProbe } from "./timeProbeLogic";

// Nights a Google export suggests, drawn against the recorded ones (ADR-0059).
// It is a double plot: each row is a day and the day after it, so a night that
// crosses midnight is one shape, and a drifting rhythm runs down the page as a
// slanting band. It opens on the last weeks, where rows are tall enough to set
// a suggestion against a recorded night; the whole export, years of rows drawn
// as two paths in a box the height of a page, is one press away.

function figureSummary(activity: AccountActivity, recent: boolean) {
  const recorded = activity.agreement?.recorded ?? 0;
  const beside = recorded > 0 ? `, beside ${recorded} recorded` : "";
  const nights = `${activity.nights} suggested ${activity.nights === 1 ? "night" : "nights"}`;
  return recent
    ? `The last ${RECENT_DAYS / 7} weeks of nights suggested by your Google activity. In all it covers ${activity.spanLabel}, with ${nights}${beside}.`
    : `Nights suggested by your Google activity, ${activity.spanLabel}: ${nights}${beside}.`;
}

export function ActivityNightsFigure({ activity }: { activity: AccountActivity }) {
  const [everything, setEverything] = useState(false);
  const long = activity.days > RECENT_DAYS;
  const shown = long && !everything ? lastDays(activity, RECENT_DAYS) : activity;
  const { days, firstDate, zoneId } = shown;
  const height = figureHeight(days);
  const row = useRef(0);
  const resolve = useCallback(
    (fraction: number) => ({
      position: fraction,
      label: civilProbeLabel(firstDate, row.current * DAY + fraction * 2 * DAY),
      zoneId,
    }),
    [firstDate, zoneId],
  );
  const probe = useTimeProbe(resolve);
  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect();
    if (bounds.height <= 0) return;
    const at = Math.floor(((event.clientY - bounds.top) / bounds.height) * days);
    row.current = Math.min(days - 1, Math.max(0, at));
    // The reading sits just above the pointer's row rather than under it.
    const top = ((row.current + 0.5) / days) * 100;
    event.currentTarget.style.setProperty("--probe-label-top", `calc(${top}% - 16px)`);
    probe.onPointerMove(event);
  };

  return (
    <figure className="activity-nights-figure">
      {long && (
        <div className="activity-nights-span" role="group" aria-label="Days drawn">
          {[false, true].map((all) => (
            <button
              className={`filter${everything === all ? " active" : ""}`}
              type="button"
              aria-pressed={everything === all}
              onClick={() => setEverything(all)}
              key={String(all)}
            >
              {all ? `All ${spanWords(activity.days)}` : `Last ${RECENT_DAYS / 7} weeks`}
            </button>
          ))}
        </div>
      )}
      <div className="activity-nights-axis" aria-hidden="true">
        {[0, 12, 24, 36, 48].map((hour) => (
          <span key={hour}>{axisHour(hour % 24)}</span>
        ))}
      </div>
      <div className="activity-nights-body">
        <div className="activity-nights-dates" style={{ height }} aria-hidden="true">
          {rowLabels(firstDate, days, height).map((label) => (
            <time key={label.row} style={{ top: `${(label.row / days) * 100}%` }}>
              {label.text}
            </time>
          ))}
        </div>
        <div
          className="activity-nights-plot has-time-probe"
          style={{ height }}
          role="img"
          aria-label={figureSummary(activity, shown !== activity)}
          onPointerMove={onPointerMove}
          onPointerLeave={probe.onPointerLeave}
        >
          <svg viewBox={`0 0 ${2 * DAY} ${days}`} preserveAspectRatio="none" aria-hidden="true">
            <path className="activity-nights-suggested" d={nightPath(shown.suggested)} />
            <path className="activity-nights-recorded" d={nightPath(shown.recorded, 0.3)} />
          </svg>
          <TimeProbe probeRef={probe.probeRef} labelRef={probe.labelRef} />
        </div>
      </div>
      <figcaption className="activity-nights-legend">
        <span>
          <i className="activity-nights-key is-suggested" aria-hidden="true" /> Suggested by your
          activity
        </span>
        <span>
          <i className="activity-nights-key is-recorded" aria-hidden="true" /> Recorded
        </span>
        <p>Each row is a day followed by the next, so a night across midnight stays whole.</p>
      </figcaption>
    </figure>
  );
}

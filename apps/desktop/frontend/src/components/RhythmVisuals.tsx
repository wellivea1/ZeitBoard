import { useCallback, useState } from "react";
import type { RhythmDriftPointFixture, RhythmSleepBandFixture } from "../data/phaseTwo";
import type { RhythmActogram, RhythmDrift } from "../data/rhythm";
import {
  rhythmMarkerKindLabels,
  type RhythmMarker,
  type RhythmMarkerKind,
} from "../data/rhythmMarkers";
import { RhythmMarkerGlyph } from "./RhythmMarkerGlyph";
import { TimeProbe } from "./TimeProbe";
import { civilProbeLabel, formatCivilDate, formatClock24, useTimeProbe } from "./timeProbeLogic";

const DOUBLE_PLOT_HOURS = 48;
const DAY_HOURS = 24;

function hourToPercent(hour: number) {
  return `${(hour / DOUBLE_PLOT_HOURS) * 100}%`;
}

function bandStyle(startHour: number, durationHours: number) {
  return {
    left: hourToPercent(startHour),
    width: `${(durationHours / DOUBLE_PLOT_HOURS) * 100}%`,
  };
}

function bandAriaLabel(band: RhythmSleepBandFixture) {
  const prefix = band.kind === "forecast" ? "Predicted sleep window" : "Sleep interval";
  return `${prefix}: ${band.day}, ${band.startLabel} to ${band.wakeLabel}, ${band.durationLabel}, ${band.source}, ${band.confidence} confidence`;
}

function dayPercent(hour: number) {
  return `${(hour / DAY_HOURS) * 100}%`;
}

function circularSegments(startHour: number, durationHours: number) {
  const start = ((startHour % DAY_HOURS) + DAY_HOURS) % DAY_HOURS;
  const boundedDuration = Math.min(Math.max(durationHours, 0), DAY_HOURS);
  const firstDuration = Math.min(boundedDuration, DAY_HOURS - start);
  const segments = [{ start, duration: firstDuration }];
  const remainder = boundedDuration - firstDuration;
  if (remainder > 0) segments.push({ start: 0, duration: remainder });
  return segments;
}

function withinCircularSpan(hour: number, startHour: number, durationHours: number) {
  const start = ((startHour % DAY_HOURS) + DAY_HOURS) % DAY_HOURS;
  const point = ((hour % DAY_HOURS) + DAY_HOURS) % DAY_HOURS;
  const duration = Math.min(Math.max(durationHours, 0), DAY_HOURS);
  return (point - start + DAY_HOURS) % DAY_HOURS <= duration;
}

interface CycleStripProps {
  actogram: RhythmActogram;
  usefulWindowLabel: string;
  sleepWindowLabel: string;
}

export function CycleStrip({ actogram, usefulWindowLabel, sleepWindowLabel }: CycleStripProps) {
  const forecast = actogram.forecastRows[0];
  const nowHour = ((actogram.now.hour % DAY_HOURS) + DAY_HOURS) % DAY_HOURS;
  const forecastStart = forecast ? ((forecast.startHour % DAY_HOURS) + DAY_HOURS) % DAY_HOURS : 0;
  const awakeDuration = (forecastStart - nowHour + DAY_HOURS) % DAY_HOURS;
  const resolveProbe = useCallback(
    (fraction: number) => {
      if (!forecast) return undefined;
      const hour = fraction * DAY_HOURS;
      const predicted = withinCircularSpan(hour, forecastStart, forecast.durationHours);
      return {
        position: fraction,
        label: civilProbeLabel(actogram.now.civilDate, fraction * DAY_HOURS * 60, {
          predicted,
          approximate: predicted,
        }),
        zoneId: actogram.now.zoneId,
      };
    },
    [actogram.now.civilDate, actogram.now.zoneId, forecast, forecastStart],
  );
  const probe = useTimeProbe(resolveProbe);

  if (!forecast) return null;

  return (
    <figure
      className="cycle-strip"
      aria-label={`Today in your cycle. ${usefulWindowLabel}. Predicted sleep window ${sleepWindowLabel}. ${forecast.confidence} confidence.`}
    >
      <figcaption>
        <span>
          <strong>Today in your cycle</strong>
          <small>Forecast is approximate and widens ahead.</small>
        </span>
        <a href="#/rhythm">Open full rhythm</a>
      </figcaption>
      <div className="cycle-strip-axis" aria-hidden="true">
        <span>12 AM</span>
        <span>6 AM</span>
        <span>Noon</span>
        <span>6 PM</span>
        <span>12 AM</span>
      </div>
      <div
        className="cycle-strip-track has-time-probe"
        aria-hidden="true"
        onPointerMove={probe.onPointerMove}
        onPointerLeave={probe.onPointerLeave}
      >
        {circularSegments(nowHour, awakeDuration).map((segment) => (
          <span
            className="cycle-strip-segment is-awake"
            style={{ left: dayPercent(segment.start), width: dayPercent(segment.duration) }}
            key={`awake-${segment.start}`}
          />
        ))}
        {circularSegments(forecastStart, forecast.durationHours).map((segment) => (
          <span
            className="cycle-strip-segment is-sleep"
            style={{ left: dayPercent(segment.start), width: dayPercent(segment.duration) }}
            key={`sleep-${segment.start}`}
          />
        ))}
        <span className="cycle-strip-now" style={{ left: dayPercent(nowHour) }}>
          now
        </span>
        <TimeProbe probeRef={probe.probeRef} labelRef={probe.labelRef} />
      </div>
      <div className="cycle-strip-legend">
        <span>
          <i className="is-awake" /> Estimated waking span
        </span>
        <span>
          <i className="is-sleep" /> Predicted sleep: {sleepWindowLabel}
        </span>
        <span className="cycle-strip-useful">Useful task window: {usefulWindowLabel}</span>
      </div>
    </figure>
  );
}

function ActogramBand({
  band,
  duplicate = false,
}: {
  band: RhythmSleepBandFixture;
  duplicate?: boolean;
}) {
  const startHour = duplicate ? band.startHour + 24 : band.startHour;
  return (
    <span
      className={`actogram-band is-${band.kind}${duplicate ? " is-duplicate" : ""}`}
      style={bandStyle(startHour, band.durationHours)}
      tabIndex={duplicate ? undefined : 0}
      aria-hidden={duplicate || undefined}
      aria-label={duplicate ? undefined : bandAriaLabel(band)}
      role={duplicate ? undefined : "img"}
    >
      {!duplicate && (
        <span>{band.kind === "forecast" ? "Predicted sleep window" : band.source}</span>
      )}
    </span>
  );
}

function ActogramMarker({
  marker,
  duplicate = false,
}: {
  marker: RhythmMarker;
  duplicate?: boolean;
}) {
  const hour = duplicate ? marker.hour + DAY_HOURS : marker.hour;
  const label = `${marker.kindLabel}, self-reported context: ${marker.rangeLabel}${marker.note ? `. Note: ${marker.note}` : ""}`;
  return (
    <span
      className={`actogram-marker${duplicate ? " is-duplicate" : ""}`}
      style={{ left: hourToPercent(hour) }}
      role={duplicate ? undefined : "img"}
      aria-hidden={duplicate || undefined}
      aria-label={duplicate ? undefined : label}
      tabIndex={duplicate ? undefined : 0}
    >
      <RhythmMarkerGlyph kind={marker.kind} decorative />
    </span>
  );
}

function ActogramRow({
  band,
  now,
  markers,
}: {
  band: RhythmSleepBandFixture;
  now: RhythmActogram["now"];
  markers: RhythmMarker[];
}) {
  const duplicateFits = band.startHour + 24 < DOUBLE_PLOT_HOURS;
  const showNowTick = Boolean(
    band.zoneId && now.zoneId && band.civilDate === now.civilDate && band.zoneId === now.zoneId,
  );
  const resolveProbe = useCallback(
    (fraction: number) => ({
      position: fraction,
      label: civilProbeLabel(band.civilDate, fraction * DOUBLE_PLOT_HOURS * 60, {
        predicted: band.kind === "forecast",
      }),
      zoneId: band.zoneId,
    }),
    [band.civilDate, band.kind, band.zoneId],
  );
  const probe = useTimeProbe(resolveProbe);

  return (
    <div className="actogram-visual-row">
      <time>{band.day}</time>
      <div
        className="actogram-visual-track has-time-probe"
        onPointerMove={probe.onPointerMove}
        onPointerLeave={probe.onPointerLeave}
      >
        {band.originalStartHour !== undefined && band.originalDurationHours !== undefined && (
          <span
            className="actogram-band is-original"
            style={bandStyle(band.originalStartHour, band.originalDurationHours)}
            aria-hidden="true"
            title={band.originalLabel}
          />
        )}
        <ActogramBand band={band} />
        {duplicateFits && <ActogramBand band={band} duplicate />}
        {markers.map((marker) => (
          <ActogramMarker marker={marker} key={marker.markerId} />
        ))}
        {markers.map((marker) => (
          <ActogramMarker marker={marker} duplicate key={`${marker.markerId}-duplicate`} />
        ))}
        {showNowTick && (
          <span
            className="actogram-now-tick"
            style={{ left: hourToPercent(now.hour) }}
            aria-hidden="true"
          />
        )}
        <TimeProbe probeRef={probe.probeRef} labelRef={probe.labelRef} />
      </div>
      <small>{band.confidence}</small>
    </div>
  );
}

export function ActogramPanel({
  actogram,
  markers = [],
}: {
  actogram: RhythmActogram;
  markers?: RhythmMarker[];
}) {
  const [showForecast, setShowForecast] = useState(false);
  const forecastRows = showForecast ? actogram.forecastRows : [];
  const allBands = [...actogram.observedRows, ...forecastRows];
  const rowKey = (civilDate: string, zoneId?: string) => (zoneId ? `${civilDate}::${zoneId}` : "");
  const plottedRows = new Set(allBands.map((band) => rowKey(band.civilDate, band.zoneId)));
  const plottedMarkers = markers.filter((marker) =>
    plottedRows.has(rowKey(marker.civilDate, marker.zoneId)),
  );
  const hiddenMarkerCount = markers.length - plottedMarkers.length;
  const presentMarkerKinds = (
    ["travel", "illness", "disruption", "forced_schedule"] as RhythmMarkerKind[]
  ).filter((kind) => plottedMarkers.some((marker) => marker.kind === kind));
  const markersByRow = new Map<string, RhythmMarker[]>();
  for (const marker of plottedMarkers) {
    const key = rowKey(marker.civilDate, marker.zoneId);
    const sameRow = markersByRow.get(key) ?? [];
    sameRow.push(marker);
    markersByRow.set(key, sameRow);
  }
  const markersFor = (band: RhythmSleepBandFixture) =>
    markersByRow.get(rowKey(band.civilDate, band.zoneId)) ?? [];

  return (
    <section className="rhythm-visual-surface actogram-panel" aria-labelledby="actogram-title">
      <div className="panel-heading actogram-heading">
        <div>
          <p className="section-kicker">Recent cycles</p>
          <h2 id="actogram-title">Double-plot actogram</h2>
        </div>
        <div className="actogram-controls">
          <label>
            <input
              type="checkbox"
              checked={showForecast}
              onChange={(event) => setShowForecast(event.target.checked)}
            />{" "}
            Show forecast
          </label>
        </div>
      </div>

      <div className="actogram-chart" role="group" aria-label={actogram.summary}>
        <div className="actogram-visual-axis" aria-hidden="true">
          <span>0</span>
          <span>6</span>
          <span>12</span>
          <span>18</span>
          <span>0 (24)</span>
          <span>6</span>
          <span>12</span>
          <span>18</span>
          <span>0 (48)</span>
        </div>
        <div className="actogram-visual-grid">
          {actogram.observedRows.map((band) => (
            <ActogramRow band={band} now={actogram.now} markers={markersFor(band)} key={band.id} />
          ))}
          {showForecast && (
            <div className="actogram-now-line" aria-hidden="true">
              <span>{actogram.now.label}</span>
            </div>
          )}
          {forecastRows.map((band) => (
            <ActogramRow band={band} now={actogram.now} markers={markersFor(band)} key={band.id} />
          ))}
        </div>
      </div>

      <div className="actogram-footer">
        <span>
          <i className="legend-observed" /> observed
        </span>
        <span>
          <i className="legend-inferred" /> inferred
        </span>
        <span>
          <i className="legend-forecast" /> predicted
        </span>
        <span>| now</span>
        {presentMarkerKinds.length > 0 && (
          <span className="actogram-marker-legend" aria-label="Context marker legend">
            {presentMarkerKinds.map((markerKind) => (
              <span key={markerKind}>
                <RhythmMarkerGlyph kind={markerKind} decorative />
                {rhythmMarkerKindLabels[markerKind]}
              </span>
            ))}
          </span>
        )}
        <p>Approximate. Forecast widens with time and is shown as ranges, not hard lines.</p>
        {hiddenMarkerCount > 0 && (
          <p className="actogram-marker-note">
            {hiddenMarkerCount} context {hiddenMarkerCount === 1 ? "marker falls" : "markers fall"}{" "}
            outside the civil dates and time zones currently plotted.
          </p>
        )}
      </div>

      <table className="sr-table">
        <caption>Actogram sleep and forecast table</caption>
        <thead>
          <tr>
            <th>Day</th>
            <th>Sleep start</th>
            <th>Wake</th>
            <th>Duration</th>
            <th>Source</th>
            <th>Confidence</th>
            <th>Context markers</th>
          </tr>
        </thead>
        <tbody>
          {allBands.map((band) => (
            <tr key={band.id}>
              <td>{band.day}</td>
              <td>{band.startLabel}</td>
              <td>{band.wakeLabel}</td>
              <td>{band.durationLabel}</td>
              <td>{band.source}</td>
              <td>{band.confidence}</td>
              <td>
                {markersFor(band)
                  .map(
                    (marker) =>
                      `${marker.kindLabel}: ${marker.rangeLabel}${marker.note ? `; ${marker.note}` : ""}`,
                  )
                  .join(" | ") || "None"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function scaleDriftX(index: number, points: RhythmDriftPointFixture[]) {
  if (points.length === 1) return 50;
  return 8 + (index / (points.length - 1)) * 84;
}

function scaleDriftY(hour: number, yMinHour: number, yMaxHour: number) {
  const normalized = (hour - yMinHour) / (yMaxHour - yMinHour);
  return 90 - normalized * 72;
}

// Format an unwrapped onset hour (which may run below 0 or above 24) back to a
// civil clock label for the dynamic y-axis ticks.
function formatClockHour(hour: number) {
  const normalized = ((hour % 24) + 24) % 24;
  let display = Math.floor(normalized);
  let minute = Math.round((normalized - display) * 60);
  if (minute === 60) {
    display = (display + 1) % 24;
    minute = 0;
  }
  const period = display >= 12 ? "PM" : "AM";
  const hour12 = display % 12 === 0 ? 12 : display % 12;
  return minute === 0
    ? `${hour12} ${period}`
    : `${hour12}:${String(minute).padStart(2, "0")} ${period}`;
}

const DRIFT_TICKS = 4;

export function DriftPanel({ drift }: { drift: RhythmDrift }) {
  const points = drift.points;
  const { yMinHour, yMaxHour } = drift;
  const resolveProbe = useCallback(
    (fraction: number) => {
      if (points.length === 0) return undefined;
      const cursorX = fraction * 100;
      let nearestIndex = 0;
      let nearestDistance = Number.POSITIVE_INFINITY;
      points.forEach((_, index) => {
        const distance = Math.abs(scaleDriftX(index, points) - cursorX);
        if (distance < nearestDistance) {
          nearestDistance = distance;
          nearestIndex = index;
        }
      });
      const point = points[nearestIndex];
      if (!point) return undefined;
      const onset = formatClock24(point.onsetHour);
      const fit = formatClock24(point.fitHour);
      const fitLabel = fit === onset ? "" : ` (fit ${fit})`;
      return {
        position: scaleDriftX(nearestIndex, points) / 100,
        label: `${formatCivilDate(point.civilDate, 0, false)} · onset ${onset}${fitLabel}`,
        zoneId: point.zoneId,
      };
    },
    [points],
  );
  const probe = useTimeProbe(resolveProbe);
  // Ticks run top (latest onset) to bottom (earliest), derived from the data
  // range so genuinely free-running onsets are never clipped.
  const ticks = Array.from(
    { length: DRIFT_TICKS },
    (_, i) => yMaxHour - (i / (DRIFT_TICKS - 1)) * (yMaxHour - yMinHour),
  );
  const fitPoints = points
    .map(
      (point, index) =>
        `${scaleDriftX(index, points)},${scaleDriftY(point.fitHour, yMinHour, yMaxHour)}`,
    )
    .join(" ");
  const bandPoints = [
    ...points.map(
      (point, index) =>
        `${scaleDriftX(index, points)},${scaleDriftY(point.bandHighHour, yMinHour, yMaxHour)}`,
    ),
    ...[...points]
      .reverse()
      .map(
        (point, reverseIndex) =>
          `${scaleDriftX(points.length - 1 - reverseIndex, points)},${scaleDriftY(point.bandLowHour, yMinHour, yMaxHour)}`,
      ),
  ].join(" ");

  return (
    <section className="rhythm-visual-surface drift-panel" aria-labelledby="drift-title">
      <div className="panel-heading">
        <div>
          <p className="section-kicker">Phase / drift</p>
          <h2 id="drift-title">{drift.title}</h2>
        </div>
        <div className="drift-summary">
          <strong>{drift.slopeLabel}</strong>
          <span>{drift.confidence} confidence</span>
        </div>
      </div>

      <div className="drift-body">
        <div className="drift-y-axis" aria-hidden="true">
          {ticks.map((hour) => (
            <span key={hour}>{formatClockHour(hour)}</span>
          ))}
        </div>
        <div className="drift-chart" role="img" aria-label={drift.summary}>
          <div
            className="drift-plot has-time-probe"
            onPointerMove={probe.onPointerMove}
            onPointerLeave={probe.onPointerLeave}
          >
            <svg
              className="drift-svg"
              viewBox="0 0 100 100"
              preserveAspectRatio="none"
              aria-hidden="true"
            >
              {ticks.map((hour) => (
                <line
                  className="drift-gridline"
                  x1="0"
                  x2="100"
                  y1={scaleDriftY(hour, yMinHour, yMaxHour)}
                  y2={scaleDriftY(hour, yMinHour, yMaxHour)}
                  vectorEffect="non-scaling-stroke"
                  key={hour}
                />
              ))}
              <polygon className="drift-band" points={bandPoints} />
              <polyline
                className="drift-fit"
                points={fitPoints}
                vectorEffect="non-scaling-stroke"
              />
              {points.map((point, index) => (
                <circle
                  className="drift-point"
                  cx={scaleDriftX(index, points)}
                  cy={scaleDriftY(point.onsetHour, yMinHour, yMaxHour)}
                  r="1.7"
                  vectorEffect="non-scaling-stroke"
                  key={point.id}
                />
              ))}
            </svg>
            <TimeProbe probeRef={probe.probeRef} labelRef={probe.labelRef} />
          </div>
          <div className="drift-x-axis" aria-hidden="true">
            {points.map((point) => (
              <span key={point.id}>{point.day}</span>
            ))}
          </div>
        </div>
      </div>

      <div className="actogram-footer">
        <span>
          <i className="legend-point" /> observed onset
        </span>
        <span>
          <i className="legend-fit" /> Theil-Sen fit
        </span>
        <span>
          <i className="legend-band" /> uncertainty band
        </span>
        <p>Y-axis is unwrapped so the free-running trend stays readable across midnight.</p>
      </div>

      <table className="sr-table">
        <caption>Drift trend table</caption>
        <thead>
          <tr>
            <th>Day</th>
            <th>Sleep onset</th>
            <th>Source</th>
            <th>Confidence</th>
            <th>Fitted onset</th>
          </tr>
        </thead>
        <tbody>
          {points.map((point) => (
            <tr key={point.id}>
              <td>{point.day}</td>
              <td>{point.onsetLabel}</td>
              <td>{point.source}</td>
              <td>{point.confidence}</td>
              <td>{formatClock24(point.fitHour)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

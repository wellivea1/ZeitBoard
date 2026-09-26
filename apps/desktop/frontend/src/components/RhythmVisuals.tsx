import { useCallback, useState } from "react";
import type { RhythmDriftPoint, RhythmSleepBand } from "../data/rhythm";
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

function bandAriaLabel(band: RhythmSleepBand) {
  const prefix = band.kind === "forecast" ? "Predicted sleep window" : "Sleep interval";
  return `${prefix}: ${band.day}, ${band.startLabel} to ${band.wakeLabel}, ${band.durationLabel}, ${band.source}`;
}

function ActogramBand({ band, duplicate = false }: { band: RhythmSleepBand; duplicate?: boolean }) {
  const startHour = duplicate ? band.startHour + 24 : band.startHour;
  return (
    <span
      className={`actogram-band is-${band.kind}${duplicate ? " is-duplicate" : ""}`}
      style={bandStyle(startHour, band.durationHours)}
      tabIndex={duplicate ? undefined : 0}
      aria-hidden={duplicate || undefined}
      aria-label={duplicate ? undefined : bandAriaLabel(band)}
      role={duplicate ? undefined : "img"}
    />
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
  band: RhythmSleepBand;
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
  const markersFor = (band: RhythmSleepBand) =>
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
          <i className="legend-observed" /> Observed
        </span>
        <span>
          <i className="legend-inferred" /> Inferred
        </span>
        <span>
          <i className="legend-forecast" /> Predicted
        </span>
        <span>
          <i className="legend-now" /> Now
        </span>
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

function scaleDriftX(index: number, points: RhythmDriftPoint[]) {
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

// Whole hours at a step that gives three to six lines across the data's range,
// so the axis reads "6 PM", not "6:20 PM".
const DRIFT_TICK_STEPS = [1, 2, 3, 4, 6, 8, 12, 24];

function driftTicks(yMinHour: number, yMaxHour: number): number[] {
  const span = Math.max(yMaxHour - yMinHour, 1);
  const step = DRIFT_TICK_STEPS.find((candidate) => span / candidate <= 5) ?? 24;
  const ticks: number[] = [];
  for (let hour = Math.ceil(yMinHour / step) * step; hour <= yMaxHour; hour += step) {
    ticks.push(hour);
  }
  return ticks;
}

// At most seven dates under the chart, evenly spread and always including the
// latest cycle, so the labels never crowd or stack.
const DRIFT_X_LABELS = 7;

function driftLabelIndexes(count: number): number[] {
  if (count === 0) return [];
  const step = Math.max(1, Math.ceil(count / DRIFT_X_LABELS));
  const indexes: number[] = [];
  for (let index = (count - 1) % step; index < count; index += step) indexes.push(index);
  return indexes;
}

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
  // Ticks come from the data range so genuinely free-running onsets are never
  // clipped, and each label sits level with its own gridline.
  const ticks = driftTicks(yMinHour, yMaxHour);
  const labelIndexes = driftLabelIndexes(points.length);
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
            <span key={hour} style={{ top: `${scaleDriftY(hour, yMinHour, yMaxHour)}%` }}>
              {formatClockHour(hour)}
            </span>
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
              {labelIndexes.map((index) => (
                <line
                  className="drift-gridline"
                  x1={scaleDriftX(index, points)}
                  x2={scaleDriftX(index, points)}
                  y1="0"
                  y2="100"
                  vectorEffect="non-scaling-stroke"
                  key={`x-${index}`}
                />
              ))}
              <polygon className="drift-band" points={bandPoints} />
              <polyline
                className="drift-fit"
                points={fitPoints}
                vectorEffect="non-scaling-stroke"
              />
              {/* A zero-length line with a round cap and an unscaled stroke is
                  a true circle however the plot stretches; a <circle> in this
                  stretched viewBox is drawn as an ellipse. */}
              {points.map((point, index) => {
                const x = scaleDriftX(index, points);
                const y = scaleDriftY(point.onsetHour, yMinHour, yMaxHour);
                return (
                  <line
                    className="drift-point"
                    x1={x}
                    x2={x}
                    y1={y}
                    y2={y}
                    vectorEffect="non-scaling-stroke"
                    key={point.id}
                  />
                );
              })}
            </svg>
            <TimeProbe probeRef={probe.probeRef} labelRef={probe.labelRef} />
          </div>
          <div className="drift-x-axis" aria-hidden="true">
            {labelIndexes.map((index) => (
              <span key={points[index]?.id} style={{ left: `${scaleDriftX(index, points)}%` }}>
                {points[index]?.day}
              </span>
            ))}
          </div>
        </div>
      </div>

      <div className="actogram-footer">
        <span>
          <i className="legend-point" /> Observed onset
        </span>
        <span>
          <i className="legend-fit" /> Trend (Theil–Sen)
        </span>
        <span>
          <i className="legend-band" /> Uncertainty
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
            <th>Fitted onset</th>
          </tr>
        </thead>
        <tbody>
          {points.map((point) => (
            <tr key={point.id}>
              <td>{point.day}</td>
              <td>{point.onsetLabel}</td>
              <td>{point.source}</td>
              <td>{formatClock24(point.fitHour)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

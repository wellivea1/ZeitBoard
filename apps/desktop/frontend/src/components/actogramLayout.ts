import type { RhythmActogram, RhythmSleepBand } from "../data/rhythm";
import type { RhythmMarker } from "../data/rhythmMarkers";
import { formatCivilDate } from "./timeProbeLogic";

// The geometry of Rhythm's actogram, apart from the component so it can be read
// and tested as arithmetic. It is a double plot: a row for each civil day,
// holding the 48 hours from its midnight, so each day is drawn twice, on its
// own row and on the right of the row before, and a sleep across midnight is
// one shape. Rows run newest first, as Rhythm always has, with the days to come
// above today. A day with nothing in its 48 hours has no row.

export const DOUBLE_PLOT_HOURS = 48;

const DAY_HOURS = 24;
const DAY_MS = 86_400_000;

export interface PlacedBand {
  band: RhythmSleepBand;
  /** Hours from the row's midnight, clipped to its 48. */
  from: number;
  to: number;
  /** On the row of the day it began: drawn at full strength, named and focusable. */
  primary: boolean;
}

export interface PlacedMarker {
  marker: RhythmMarker;
  hour: number;
  primary: boolean;
}

export interface ActogramRow {
  civilDate: string;
  label: string;
  zoneId?: string;
  bands: PlacedBand[];
  markers: PlacedMarker[];
  /** Where now falls, on today's row. */
  nowHour?: number;
  /** The hour from which the row is ahead of now, while the forecast is drawn. */
  predictedFrom?: number;
  /** Days with nothing to draw were left out between this row and the one above. */
  gapAbove: boolean;
}

export interface ActogramLayout {
  rows: ActogramRow[];
  /** The row the now line sits above: today's, when days to come are drawn over it. */
  nowLineAbove?: number;
  /** The markers drawn: those on a day with a row, in that row's time zone. */
  markers: RhythmMarker[];
}

function dayNumber(civilDate: string) {
  return Math.round(Date.parse(`${civilDate}T00:00:00Z`) / DAY_MS);
}

function civilDateOf(day: number) {
  return new Date(day * DAY_MS).toISOString().slice(0, 10);
}

/** A stretch that begins `startHour` into `day`, on the row for `rowDay`. */
function place(day: number, startHour: number, hours: number, rowDay: number) {
  const from = (day - rowDay) * DAY_HOURS + startHour;
  const to = from + hours;
  if (to <= 0 || from >= DOUBLE_PLOT_HOURS) return undefined;
  return { from: Math.max(0, from), to: Math.min(DOUBLE_PLOT_HOURS, to) };
}

export function actogramLayout(
  actogram: RhythmActogram,
  markers: readonly RhythmMarker[],
  showForecast: boolean,
): ActogramLayout {
  const bands = [...actogram.observedRows, ...(showForecast ? actogram.forecastRows : [])]
    .map((band) => ({ band, day: dayNumber(band.civilDate) }))
    .filter(({ day }) => Number.isFinite(day));
  if (bands.length === 0) return { rows: [], markers: [] };

  const now = actogram.now;
  const nowDay = now.civilDate ? dayNumber(now.civilDate) : Number.NaN;
  const first = Math.min(...bands.map(({ day }) => day));
  const last = Math.max(...bands.map(({ day }) => day), Number.isFinite(nowDay) ? nowDay : first);

  // A day's zone is that of the sleep that began on it, or else the last one's.
  const zones = new Map<number, string | undefined>();
  let zone: string | undefined;
  for (let day = first; day <= last; day++) {
    zone = bands.find((entry) => entry.day === day)?.band.zoneId ?? zone;
    zones.set(day, zone);
  }

  const drawn = markers
    .map((marker) => ({ marker, day: dayNumber(marker.civilDate) }))
    .filter(({ marker, day }) => marker.zoneId && zones.get(day) === marker.zoneId);

  const rows: ActogramRow[] = [];
  let above: number | undefined;
  let firstBegun: number | undefined;
  for (let day = last; day >= first; day--) {
    const rowZone = zones.get(day);
    const rowBands = bands.flatMap(({ band, day: began }): PlacedBand[] => {
      const span = place(began, band.startHour, band.durationHours, day);
      return span ? [{ band, ...span, primary: began === day }] : [];
    });
    const rowMarkers = drawn.flatMap(({ marker, day: on }): PlacedMarker[] => {
      const hour = (on - day) * DAY_HOURS + marker.hour;
      return hour >= 0 && hour < DOUBLE_PLOT_HOURS ? [{ marker, hour, primary: on === day }] : [];
    });
    const today = day === nowDay && rowZone !== undefined && rowZone === now.zoneId;
    if (rowBands.length === 0 && rowMarkers.length === 0 && !today) continue;
    if (firstBegun === undefined && day <= nowDay) firstBegun = rows.length;
    rows.push({
      civilDate: civilDateOf(day),
      label: formatCivilDate(civilDateOf(day), 0, false),
      ...(rowZone ? { zoneId: rowZone } : {}),
      bands: rowBands,
      markers: rowMarkers,
      ...(today ? { nowHour: now.hour } : {}),
      ...(showForecast && Number.isFinite(nowDay)
        ? { predictedFrom: (nowDay - day) * DAY_HOURS + now.hour }
        : {}),
      gapAbove: above !== undefined && above - day > 1,
    });
    above = day;
  }

  return {
    rows,
    ...(showForecast && firstBegun !== undefined && firstBegun > 0
      ? { nowLineAbove: firstBegun }
      : {}),
    markers: drawn.map(({ marker }) => marker),
  };
}

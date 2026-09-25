import type { CalendarBandSegment } from "../data/calendar";

export type RhythmState = "asleep" | "awake" | "uncertain";

export interface RhythmSegment {
  state: RhythmState;
  startMinute: number;
  endMinute: number;
}

/**
 * A day's predicted sleep and waking windows as three states, the same three
 * the Home outlook draws. The windows overlap on purpose: an instant inside
 * both is one where the model does not know which side of the boundary it is
 * on, so it is uncertain, not whichever band happened to be painted last.
 */
export function rhythmSegments(bands: CalendarBandSegment[]): RhythmSegment[] {
  const bounds = [...new Set(bands.flatMap((band) => [band.startMinute, band.endMinute]))].sort(
    (left, right) => left - right,
  );
  const segments: RhythmSegment[] = [];
  for (let index = 0; index + 1 < bounds.length; index += 1) {
    const start = bounds[index] ?? 0;
    const end = bounds[index + 1] ?? start;
    const middle = (start + end) / 2;
    const covered = (kind: CalendarBandSegment["kind"]) =>
      bands.some(
        (band) => band.kind === kind && band.startMinute <= middle && middle < band.endMinute,
      );
    const asleep = covered("predicted_sleep");
    const awake = covered("predicted_wake");
    if (!asleep && !awake) continue;
    const state: RhythmState = asleep && awake ? "uncertain" : asleep ? "asleep" : "awake";
    const previous = segments.at(-1);
    if (previous && previous.state === state && previous.endMinute === start) {
      previous.endMinute = end;
    } else {
      segments.push({ state, startMinute: start, endMinute: end });
    }
  }
  return segments;
}

import { describe, expect, it } from "vitest";
import type { CalendarBandSegment } from "../data/calendar";
import { rhythmSegments } from "./calendarLayout";

function band(
  kind: CalendarBandSegment["kind"],
  startMinute: number,
  endMinute: number,
): CalendarBandSegment {
  return {
    segmentId: `${kind}-${startMinute}`,
    kind,
    title: kind,
    startAt: "",
    endAt: "",
    startLabel: "",
    endLabel: "",
    startMinute,
    endMinute,
    confidence: "medium",
    continuesBefore: false,
    continuesAfter: false,
  };
}

describe("rhythmSegments", () => {
  // The estimator's sleep and waking windows overlap on purpose. Painted as
  // two translucent bands, the overlap was a muddle of both colours; it is the
  // stretch where the model does not know which side of the boundary it is on.
  it("draws the overlap of sleep and waking as uncertain", () => {
    expect(
      rhythmSegments([band("predicted_wake", 0, 600), band("predicted_sleep", 540, 1080)]),
    ).toEqual([
      { state: "awake", startMinute: 0, endMinute: 540 },
      { state: "uncertain", startMinute: 540, endMinute: 600 },
      { state: "asleep", startMinute: 600, endMinute: 1080 },
    ]);
  });

  it("leaves time with no prediction undrawn and joins touching windows", () => {
    expect(
      rhythmSegments([band("predicted_sleep", 60, 300), band("predicted_sleep", 300, 480)]),
    ).toEqual([{ state: "asleep", startMinute: 60, endMinute: 480 }]);
    expect(rhythmSegments([])).toEqual([]);
  });
});

import { describe, expect, it } from "vitest";
import { rhythmFixture, type RhythmActogram, type RhythmSleepBand } from "../data/rhythm";
import type { RhythmMarker } from "../data/rhythmMarkers";
import { actogramLayout, type ActogramRow } from "./actogramLayout";

const zone = "America/New_York";

function sleep(civilDate: string, startHour: number, durationHours: number): RhythmSleepBand {
  return {
    id: `sleep-${civilDate}`,
    day: civilDate,
    civilDate,
    zoneId: zone,
    startHour,
    durationHours,
    kind: "observed",
    startLabel: "",
    wakeLabel: "",
    durationLabel: "",
    source: "Manual sleep log",
    confidence: "High",
  };
}

function actogram(observedRows: RhythmSleepBand[], now: { civilDate: string; hour: number }) {
  return {
    summary: "",
    observedRows,
    forecastRows: [],
    now: { label: "now", day: "", zoneId: zone, ...now },
  } satisfies RhythmActogram;
}

/** Each row's bands as "began@from-to", starred on the row of the day they began. */
function drawn(row: ActogramRow | undefined) {
  return row?.bands.map(({ band, from, to, primary }) => {
    const name = `${band.civilDate.slice(5)}@${from}-${Math.round(to * 10) / 10}`;
    return primary ? `*${name}` : name;
  });
}

describe("actogramLayout", () => {
  // Each row was a sleep, and its right half the same sleep a day later, so
  // the half the time probe calls the next day showed the day before.
  it("draws each day's 48 hours: the day itself, then the next", () => {
    const { rows } = actogramLayout(rhythmFixture.actogram, [], false);
    expect(rows.map((row) => row.civilDate)).toEqual([
      "2026-06-16",
      "2026-06-15",
      "2026-06-14",
      "2026-06-13",
      "2026-06-12",
      "2026-06-11",
      "2026-06-10",
    ]);
    const june14 = rows.find((row) => row.civilDate === "2026-06-14");
    // Jun 13's sleep runs into the morning, Jun 14's crosses midnight whole,
    // and on the right Jun 15's begins where the next row has it.
    expect(drawn(june14)).toEqual(["06-15@45.7-48", "*06-14@20.9-28.4", "06-13@0-2.6"]);
    expect(rows.find((row) => row.civilDate === "2026-06-15")?.bands[0]?.from).toBe(21.7);
  });

  it("marks now on today's row, which holds the end of last night", () => {
    const { rows, nowLineAbove } = actogramLayout(rhythmFixture.actogram, [], false);
    expect(rows[0]).toMatchObject({ civilDate: "2026-06-16", nowHour: 13.5, label: "Jun 16" });
    expect(drawn(rows[0])).toEqual(["06-15@0-5.1"]);
    expect(rows.slice(1).every((row) => row.nowHour === undefined)).toBe(true);
    expect(nowLineAbove).toBeUndefined();
  });

  // The forecast was drawn under the oldest night, with the now line between
  // the two, so time ran up the page and then down it.
  it("puts the days to come above today, with the now line between", () => {
    const { rows, nowLineAbove } = actogramLayout(rhythmFixture.actogram, [], true);
    expect(rows.slice(0, 4).map((row) => row.civilDate)).toEqual([
      "2026-06-18",
      "2026-06-17",
      "2026-06-16",
      "2026-06-15",
    ]);
    expect(nowLineAbove).toBe(2);
    // What lies ahead of now on each row, for the time probe's "predicted".
    expect(rows.slice(0, 4).map((row) => row.predictedFrom)).toEqual([-34.5, -10.5, 13.5, 37.5]);
  });

  it("leaves out days with nothing to draw, and marks where", () => {
    const { rows } = actogramLayout(
      actogram([sleep("2026-06-01", 20, 8), sleep("2026-06-10", 20, 8)], {
        civilDate: "2026-06-10",
        hour: 23,
      }),
      [],
      false,
    );
    expect(rows.map((row) => [row.civilDate, row.gapAbove])).toEqual([
      ["2026-06-10", false],
      ["2026-06-09", false],
      ["2026-06-02", true],
      ["2026-06-01", false],
    ]);
    expect(drawn(rows[1])).toEqual(["06-10@44-48"]);
    expect(drawn(rows[2])).toEqual(["06-01@0-4"]);
  });

  it("draws context on its own day and the day before, in the day's own zone", () => {
    const marker = (markerId: string, civilDate: string, zoneId: string): RhythmMarker => ({
      markerId,
      kind: "travel",
      kindLabel: "Travel / time-zone context",
      startAt: `${civilDate}T23:00:00Z`,
      zoneId,
      civilDate,
      hour: 19,
      startLabel: "",
      rangeLabel: "",
      recordedLabel: "",
    });
    const { rows, markers } = actogramLayout(
      rhythmFixture.actogram,
      [
        marker("travel", "2026-06-15", zone),
        marker("elsewhere", "2026-06-14", "America/Chicago"),
        marker("before", "2026-05-01", zone),
      ],
      false,
    );
    expect(markers.map((drawnMarker) => drawnMarker.markerId)).toEqual(["travel"]);
    const placed = rows.flatMap((row) =>
      row.markers.map(({ hour, primary }) => [row.civilDate, hour, primary]),
    );
    expect(placed).toEqual([
      ["2026-06-15", 19, true],
      ["2026-06-14", 43, false],
    ]);
  });
});

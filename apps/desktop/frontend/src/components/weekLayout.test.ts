import { describe, expect, it } from "vitest";
import type { CalendarBandSegment, CalendarData, CalendarEventSegment } from "../data/calendar";
import type { MedicationsData } from "../data/medications";
import type { ProposalRecord } from "../data/proposals";
import type { SleepEntry } from "../data/sleepEntries";
import { assignLanes, hourLabel, weekColumns, weekTitle } from "./weekLayout";

const zoneId = "America/New_York";
// Thursday 24 September, 4:10 PM in New York.
const now = new Date("2026-09-24T20:10:00Z");

function band(
  kind: CalendarBandSegment["kind"],
  date: string,
  startMinute: number,
  endMinute: number,
  endAt = `${date}T23:59:00-04:00`,
): CalendarBandSegment {
  return {
    segmentId: `${kind}-${date}-${startMinute}`,
    kind,
    title: kind,
    startAt: `${date}T00:00:00-04:00`,
    endAt,
    startLabel: "",
    endLabel: "",
    startMinute,
    endMinute,
    confidence: "medium",
    continuesBefore: false,
    continuesAfter: false,
  };
}

function event(title: string, date: string, startMinute: number, endMinute: number) {
  const clock = (minute: number) =>
    `${String(Math.floor(minute / 60)).padStart(2, "0")}:${String(minute % 60).padStart(2, "0")}`;
  return {
    segmentId: `${title}-${date}`,
    eventId: title,
    sourceId: "source",
    sourceLabel: "Calendar",
    sourceKind: "ics",
    title,
    startAt: new Date(`${date}T${clock(startMinute)}:00-04:00`).toISOString(),
    endAt: new Date(`${date}T${clock(endMinute)}:00-04:00`).toISOString(),
    startLabel: "",
    endLabel: "",
    startMinute,
    endMinute,
    allDay: false,
    pointInTime: false,
    busy: true,
    ownership: "imported",
    readOnly: true,
    continuesBefore: false,
    continuesAfter: false,
  } satisfies CalendarEventSegment;
}

function calendar(days: CalendarData["days"]): CalendarData {
  return {
    status: "estimated",
    message: "",
    fixtureMode: false,
    zoneId,
    startCivilDate: days[0]?.civilDate ?? "",
    endCivilDate: days.at(-1)?.civilDate ?? "",
    updatedLabel: "",
    sources: [],
    days,
    warnings: [],
  };
}

const day = (civilDate: string, extra: Partial<CalendarData["days"][number]> = {}) => ({
  civilDate,
  label: civilDate,
  isToday: false,
  events: [],
  predictions: [],
  ...extra,
});

describe("assignLanes", () => {
  it("shares width only within a cluster of overlapping blocks", () => {
    const placed = assignLanes([
      { id: "a", startMinute: 60, endMinute: 180 },
      { id: "b", startMinute: 120, endMinute: 240 },
      { id: "c", startMinute: 240, endMinute: 300 },
      { id: "d", startMinute: 600, endMinute: 660 },
    ]);
    const lane = (id: string) => placed.find((item) => item.id === id);
    expect(lane("a")).toMatchObject({ lane: 0, lanes: 2 });
    expect(lane("b")).toMatchObject({ lane: 1, lanes: 2 });
    // c starts as b ends: it overlaps neither, so it takes the full width,
    // and so does d.
    expect(lane("c")).toMatchObject({ lane: 0, lanes: 1 });
    expect(lane("d")).toMatchObject({ lane: 0, lanes: 1 });
  });
});

describe("weekColumns", () => {
  const input = {
    suggestions: [] as ProposalRecord[],
    sleep: [] as SleepEntry[],
    medications: null as MedicationsData | null,
    now,
  };

  it("draws the forecast from now on and names the uncertain edges", () => {
    const [today, tomorrow] = weekColumns({
      ...input,
      calendar: calendar([
        day("2026-09-24", {
          predictions: [
            band("predicted_wake", "2026-09-24", 0, 1440),
            band("predicted_sleep", "2026-09-24", 1410, 1440),
          ],
        }),
        day("2026-09-25", {
          predictions: [
            band("predicted_sleep", "2026-09-25", 0, 610),
            band("predicted_wake", "2026-09-25", 95, 1440, "2026-09-26T12:00:00-04:00"),
          ],
        }),
      ]),
    });
    expect(today?.isToday).toBe(true);
    expect(today?.nowMinute).toBe(16 * 60 + 10);
    expect(today?.bands).toEqual([
      expect.objectContaining({ kind: "uncertain", startMinute: 1410, label: "Sleep may begin" }),
    ]);
    expect(tomorrow?.bands.map((item) => [item.kind, item.label])).toEqual([
      ["asleep", "Likely asleep"],
      ["uncertain", "Waking likely"],
    ]);
  });

  it("leaves past days to the record and splits a night across midnight", () => {
    const entry = {
      observationId: "night",
      startLocal: "2026-09-23T23:30",
      endLocal: "2026-09-24T07:45",
      effectiveStartLocal: "2026-09-23T23:30",
      effectiveEndLocal: "2026-09-24T07:45",
      durationLabel: "8 h 15 m",
      suppressed: false,
    } as SleepEntry;
    const [yesterday, today] = weekColumns({
      ...input,
      sleep: [entry],
      calendar: calendar([
        day("2026-09-23", { predictions: [band("predicted_sleep", "2026-09-23", 1380, 1440)] }),
        day("2026-09-24"),
      ]),
    });
    expect(yesterday?.isPast).toBe(true);
    expect(yesterday?.bands).toEqual([
      expect.objectContaining({ kind: "recorded", startMinute: 1410, endMinute: 1440 }),
    ]);
    expect(today?.bands).toEqual([
      expect.objectContaining({ kind: "recorded", startMinute: 0, endMinute: 465 }),
    ]);
  });

  it("places events, accepted times, suggestions and doses", () => {
    const [today] = weekColumns({
      ...input,
      suggestions: [
        {
          id: "p1",
          title: "Deep work",
          startAt: "2026-09-24T22:00:00Z",
          endAt: "2026-09-24T23:30:00Z",
        } as ProposalRecord,
      ],
      medications: {
        medications: [
          {
            medicationId: "m1",
            label: "Medication A",
            active: true,
            schedule: { forecast: { occurrences: [{ at: "2026-09-25T02:00:00Z" }] } },
          },
        ],
      } as unknown as MedicationsData,
      calendar: calendar([
        day("2026-09-24", {
          events: [
            event("Ring the pharmacy", "2026-09-24", 1050, 1070),
            { ...event("Accepted call", "2026-09-24", 1110, 1140), ownership: "app_owned" },
          ],
        }),
      ]),
    });
    expect(today?.blocks.map((block) => [block.kind, block.title, block.startMinute])).toEqual([
      ["event", "Ring the pharmacy", 1050],
      ["suggested", "Deep work", 1080],
      ["accepted", "Accepted call", 1110],
    ]);
    expect(today?.doses).toEqual([
      expect.objectContaining({ minute: 22 * 60, label: "Medication A" }),
    ]);
  });

  it("gives a block across midnight its controls once, where it begins on the board", () => {
    const [today, tomorrow] = weekColumns({
      ...input,
      suggestions: [
        {
          id: "late",
          title: "Deep work",
          // 11 PM to 12:30 AM in New York.
          startAt: "2026-09-25T03:00:00Z",
          endAt: "2026-09-25T04:30:00Z",
        } as ProposalRecord,
      ],
      calendar: calendar([
        day("2026-09-24", {
          // Began the evening before the first column.
          events: [{ ...event("Overnight shift", "2026-09-24", 0, 360), continuesBefore: true }],
        }),
        day("2026-09-25"),
      ]),
    });
    expect(today?.blocks.map((block) => [block.title, block.startMinute, block.primary])).toEqual([
      ["Overnight shift", 0, true],
      ["Deep work", 23 * 60, true],
    ]);
    expect(tomorrow?.blocks).toEqual([
      expect.objectContaining({
        title: "Deep work",
        startMinute: 0,
        endMinute: 30,
        continuesBefore: true,
        primary: false,
      }),
    ]);
  });

  it("shades what lies past the last forecast band", () => {
    const columns = weekColumns({
      ...input,
      calendar: calendar([
        day("2026-09-24", {
          predictions: [band("predicted_wake", "2026-09-24", 0, 1080, "2026-09-24T18:00:00-04:00")],
        }),
        day("2026-09-25"),
      ]),
    });
    expect(columns[0]?.forecastEndsAt).toBe(1080);
    expect(columns[1]?.forecastEndsAt).toBe(0);
  });
});

describe("wording", () => {
  it("titles a range the way a diary does", () => {
    expect(weekTitle("2026-09-24", "2026-09-27")).toBe("24 – 27 September");
    expect(weekTitle("2026-09-30", "2026-10-03")).toBe("30 September – 3 October");
    expect(weekTitle("2026-09-24", "2026-09-24")).toBe("Thursday 24 September");
  });

  it("labels the gutter in words at noon", () => {
    expect([3, 12, 15].map(hourLabel)).toEqual(["3 AM", "Noon", "3 PM"]);
  });
});

import { describe, expect, it } from "vitest";
import { outlookFixture, overviewFixture } from "./fixture";
import { awakeFor, diaryDays, leadParts } from "./homeLead";
import type { OutlookData, OutlookSegment } from "./outlook";
import type { OverviewData } from "./overview";
import type { CalendarDay, CalendarEventSegment } from "./calendar";

// Thursday 4:10 PM local, the moment the design sketches use.
const now = new Date(2026, 8, 24, 16, 10);

function segment(presence: OutlookSegment["presence"], from: number, to: number): OutlookSegment {
  return {
    presence,
    observed: false,
    rangeLabel: "",
    dayLabel: "",
    durationLabel: "",
    offsetHours: from,
    durationHours: to - from,
  };
}

const outlook: OutlookData = {
  ...outlookFixture,
  status: "available",
  horizonStart: now.toISOString(),
  segments: [
    segment("awake", 0, 7.333),
    segment("uncertain", 7.333, 9.417),
    segment("asleep", 9.417, 15.083),
    segment("uncertain", 15.083, 18),
    segment("awake", 18, 32),
  ],
  commitments: [],
};

const awake: OverviewData = {
  ...overviewFixture,
  status: "estimated",
  state: "Likely awake",
  timeSinceWake: "8 hours 2 minutes",
  freshness: { ...overviewFixture.freshness, trusted: true, state: "current" },
};

const text = (parts: { text: string }[]) => parts.map((part) => part.text).join("");

describe("the lead sentence", () => {
  it("says how long you have been awake and when sleep and waking are likely", () => {
    const parts = leadParts(awake, outlook, now);
    expect(text(parts)).toBe(
      "You have been awake for about 8 hours. Sleep is likely to begin between 11:30 PM " +
        "and 1:35 AM tonight, and you will probably wake between 7:15 and 10:10 AM tomorrow.",
    );
    // The times are what a glance looks for.
    expect(parts.filter((part) => part.strong).map((part) => part.text)).toEqual([
      "11:30 PM",
      "1:35 AM",
      "7:15",
      "10:10 AM",
    ]);
  });

  it("gives only the end of a sleep the forecast has already begun", () => {
    const inSleep = { ...outlook, segments: [segment("asleep", 0, 3), segment("uncertain", 3, 6)] };
    expect(text(leadParts({ ...awake, state: "Likely asleep" }, inSleep, now))).toContain(
      "This sleep is likely to end between 7:10 and 10:10 PM tonight.",
    );
  });

  it("leads with why the state is unknown when the records are stale", () => {
    const stale: OverviewData = {
      ...awake,
      state: "Current state uncertain",
      freshness: {
        ...awake.freshness,
        trusted: false,
        state: "stale",
        explanation: "Sleep was expected by now and none has been recorded",
      },
    };
    expect(text(leadParts(stale, outlook, now))).toMatch(
      /^Sleep was expected by now and none has been recorded\. Sleep is likely/,
    );
  });

  it("does not look ahead without an estimate", () => {
    expect(text(leadParts({ ...awake, status: "empty" }, outlook, now))).toBe(
      "There is not enough recorded sleep to look ahead yet.",
    );
  });

  it("counts the nights to a first forecast", () => {
    const before = (status: OverviewData["status"], nights: number) =>
      text(leadParts({ ...awake, status, progress: { nights, needed: 7 } }, outlook, now));
    expect(before("empty", 0)).toBe(
      "Nothing is recorded yet. ZeitBoard looks ahead once it has 7 nights.",
    );
    expect(before("refused", 3)).toBe(
      "You have recorded 3 of the 7 nights ZeitBoard needs before it can look ahead.",
    );
    // Naps and short sleeps are records, but not nights the estimate can use.
    expect(before("refused", 0)).toBe(
      "None of your records count yet. ZeitBoard needs 7 nights of main sleep.",
    );
    expect(text(leadParts({ ...awake, status: "refused" }, outlook, now))).toBe(
      "Your records do not give a forecast yet.",
    );
  });

  it("rounds long waking stretches to the hour", () => {
    expect(awakeFor("8 hours 42 minutes")).toBe("about 9 hours");
    expect(awakeFor("1 hour 40 minutes")).toBe("1 hour 40 minutes");
  });
});

describe("the diary", () => {
  function event(
    title: string,
    start: Date,
    minutes: number,
    extra: Partial<CalendarEventSegment> = {},
  ): CalendarEventSegment {
    return {
      segmentId: `${title}-segment`,
      eventId: title,
      sourceId: "source",
      sourceLabel: "Calendar",
      sourceKind: "ics",
      title,
      startAt: start.toISOString(),
      endAt: new Date(start.getTime() + minutes * 60_000).toISOString(),
      startLabel: "",
      endLabel: "",
      startMinute: 0,
      endMinute: 0,
      allDay: false,
      pointInTime: false,
      busy: true,
      ownership: "imported",
      readOnly: true,
      continuesBefore: false,
      continuesAfter: false,
      ...extra,
    };
  }
  const day = (events: CalendarEventSegment[]): CalendarDay => ({
    civilDate: "2026-09-24",
    label: "",
    isToday: true,
    events,
    predictions: [],
  });

  it("lists events and accepted times by day, with the forecast note on a boundary", () => {
    const standup = new Date(2026, 8, 25, 9, 30);
    const days = diaryDays(
      [
        day([
          event("Ring the pharmacy", new Date(2026, 8, 24, 17, 30), 20, { ownership: "app_owned" }),
          event("Finished earlier", new Date(2026, 8, 24, 9, 0), 30),
        ]),
        day([event("Team standup", standup, 45)]),
      ],
      {
        ...outlook,
        commitments: [
          {
            title: "Team standup",
            whenLabel: "",
            offsetHours: (standup.getTime() - now.getTime()) / 3_600_000,
            durationHours: 0.75,
            conflict: "near_uncertain_boundary",
            conflictLabel: "Sits where the sleep boundary is uncertain",
          } as OutlookData["commitments"][number],
        ],
      },
      null,
      now,
    );
    expect(days.map((group) => group.label)).toEqual(["Today", "Tomorrow"]);
    expect(days[0]?.entries.map((entry) => `${entry.time} ${entry.title} ${entry.kind}`)).toEqual([
      "5:30 PM Ring the pharmacy task",
    ]);
    expect(days[1]?.entries[0]).toMatchObject({
      title: "Team standup",
      kind: "event",
      note: "Sits where the sleep boundary is uncertain",
    });
  });

  it("keeps listing events while the forecast is withheld", () => {
    const withheld: OutlookData = { ...outlook, status: "withheld", segments: [], commitments: [] };
    const days = diaryDays(
      [day([event("Pharmacy pickup", new Date(2026, 8, 26, 12, 30), 30)])],
      withheld,
      null,
      now,
    );
    expect(days).toEqual([
      expect.objectContaining({
        label: "Saturday 26",
        entries: [expect.objectContaining({ title: "Pharmacy pickup", time: "12:30 PM" })],
      }),
    ]);
  });

  it("lists an event spanning midnight once", () => {
    const late = event("Night shift", new Date(2026, 8, 24, 22, 0), 8 * 60);
    const days = diaryDays(
      [day([late]), day([{ ...late, segmentId: "next", continuesBefore: true }])],
      outlook,
      null,
      now,
    );
    expect(days.flatMap((group) => group.entries)).toHaveLength(1);
  });
});

describe("the lead sentence when the outlook is withheld", () => {
  // Found at 5 AM on a stale profile: the fallback read "Sleep is likely Thu
  // Sep 24, 11:30 PM EDT to Fri Sep 25, 10:08 AM EDT", a window already begun.
  it("stops after saying why the state is unknown", () => {
    const stale: OverviewData = {
      ...awake,
      state: "Current state uncertain",
      freshness: {
        ...awake.freshness,
        trusted: false,
        state: "stale",
        explanation: "Sleep was expected by now and none has been recorded",
      },
    };
    const withheld: OutlookData = { ...outlook, status: "withheld", segments: [] };
    expect(text(leadParts(stale, withheld, now))).toBe(
      "Sleep was expected by now and none has been recorded. ",
    );
  });
});

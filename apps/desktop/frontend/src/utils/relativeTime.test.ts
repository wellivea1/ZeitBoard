import { afterEach, describe, expect, it } from "vitest";
import {
  atOffset,
  axisHour,
  civilDayOffset,
  clockRange,
  clockTime,
  hourOfDay,
  relativeDay,
  relativeRange,
  roundToMinutes,
  timeOfDay,
} from "./relativeTime";
import { setWindowClock } from "./windowClock";

afterEach(() => {
  setWindowClock("12");
});

// Local-time constructors keep these tests independent of the machine's zone.
const at = (day: number, hour: number, minute = 0) => new Date(2026, 8, day, hour, minute);
const now = at(24, 10, 40);

describe("relative days", () => {
  it("names the evening tonight and the morning today", () => {
    expect(relativeDay(at(24, 23, 30), now)).toBe("Tonight");
    expect(relativeDay(at(24, 15), now)).toBe("Today");
  });

  it("says tomorrow, then weekdays, then dates", () => {
    expect(relativeDay(at(25, 7), now)).toBe("Tomorrow");
    expect(relativeDay(at(27, 9), now)).toBe("Sunday");
    expect(relativeDay(at(30, 12), now)).toBe("Wednesday");
    expect(civilDayOffset(new Date(2026, 9, 3, 9), now)).toBe(9);
  });

  it("counts civil days, not 24-hour periods", () => {
    // 11:30 PM tonight is still today, 1:35 AM is tomorrow, though they are two
    // hours apart.
    expect(civilDayOffset(at(24, 23, 30), now)).toBe(0);
    expect(civilDayOffset(at(25, 1, 35), now)).toBe(1);
  });
});

describe("ranges", () => {
  it("does not repeat the half of the day", () => {
    const text = clockRange(at(25, 7, 15), at(25, 10, 10));
    if (/AM|PM/.test(text)) expect(text).toBe("7:15 – 10:10 AM");
  });

  it("keeps both halves when the range crosses noon or midnight", () => {
    const text = clockRange(at(24, 23, 30), at(25, 1, 35));
    if (/AM|PM/.test(text)) expect(text).toBe("11:30 PM – 1:35 AM");
  });

  it("is named by the day the range begins", () => {
    expect(relativeRange(at(24, 23, 30), at(25, 1, 35), now)).toMatch(/^Tonight /);
  });
});

// Every time the window writes is on the reader's clock (ADR-0057).
describe("the clock", () => {
  it("writes the twelve-hour clock", () => {
    setWindowClock("12");
    expect(clockTime(at(24, 21, 5))).toBe("9:05 PM");
    expect(timeOfDay(7, 0)).toBe("7:00 AM");
    expect([hourOfDay(18), hourOfDay(18, 20)]).toEqual(["6 PM", "6:20 PM"]);
    expect([0, 3, 12, 15, 24].map(axisHour)).toEqual([
      "Midnight",
      "3 AM",
      "Noon",
      "3 PM",
      "Midnight",
    ]);
  });

  it("writes the 24-hour clock, with no day period to repeat", () => {
    setWindowClock("24");
    expect(clockTime(at(24, 21, 5))).toBe("21:05");
    expect(timeOfDay(7, 0)).toBe("07:00");
    expect([hourOfDay(18), hourOfDay(18, 20)]).toEqual(["18:00", "18:20"]);
    expect([0, 3, 12, 15, 24].map(axisHour)).toEqual(["00:00", "03:00", "12:00", "15:00", "24:00"]);
    expect(clockRange(at(24, 23, 30), at(25, 1, 35))).toBe("23:30 – 01:35");
    expect(clockRange(at(25, 7, 15), at(25, 10, 10))).toBe("07:15 – 10:10");
  });
});

describe("offsets", () => {
  it("resolves an offset from the horizon start", () => {
    const start = new Date(Date.UTC(2026, 8, 24, 14, 40)).toISOString();
    expect(atOffset(start, 1.5)?.toISOString()).toBe("2026-09-24T16:10:00.000Z");
  });

  it("gives nothing rather than guessing without a start", () => {
    expect(atOffset(undefined, 3)).toBeUndefined();
    expect(atOffset("not a date", 3)).toBeUndefined();
  });

  it("rounds to a coarse step", () => {
    expect(roundToMinutes(at(24, 23, 32), 5).getMinutes()).toBe(30);
    expect(roundToMinutes(at(24, 23, 33), 5).getMinutes()).toBe(35);
  });
});

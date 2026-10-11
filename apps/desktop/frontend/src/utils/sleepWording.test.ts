import { describe, expect, it } from "vitest";

import { minutesWording, nightWording, sourceWording } from "./sleepWording";

describe("sleep wording", () => {
  it("says how far apart in hours and minutes", () => {
    expect(minutesWording(0)).toBe("0 minutes");
    expect(minutesWording(1)).toBe("1 minute");
    expect(minutesWording(25)).toBe("25 minutes");
    expect(minutesWording(60)).toBe("1 hour");
    expect(minutesWording(130)).toBe("2 hours 10 minutes");
  });

  it("names each source the way the log does", () => {
    expect(sourceWording("manual / user reported")).toBe("Logged by you");
    expect(sourceWording("file import / directly observed")).toBe("Imported");
    expect(sourceWording("health connect / directly observed")).toBe("Health Connect");
    expect(sourceWording("something new / inferred")).toBe("something new / inferred");
  });

  it("falls back to the stored labels when a time cannot be read", () => {
    expect(
      nightWording({
        startLocal: "not a time",
        endLocal: "2026-03-02T06:00",
        startLabel: "Sun Mar 1, 10:00 PM EST",
        endLabel: "Mon Mar 2, 6:00 AM EST",
      }),
    ).toEqual({ day: "Sun Mar 1, 10:00 PM EST", time: "to Mon Mar 2, 6:00 AM EST" });
  });
});

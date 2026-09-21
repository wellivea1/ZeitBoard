import { describe, expect, it } from "vitest";
import { civilCandidates, selectedCivilInstant } from "./civilTime";

describe("exact civil-time choices", () => {
  it("refuses skipped clock times and invalid dates", () => {
    expect(civilCandidates("2026-03-08T02:30", "America/New_York")).toEqual([]);
    expect(civilCandidates("2026-02-30T12:00", "UTC")).toEqual([]);
    expect(civilCandidates("", "UTC")).toEqual([]);
  });
  it("requires an explicit repeated-hour occurrence", () => {
    const wall = "2026-11-01T01:30",
      zone = "America/New_York";
    expect(civilCandidates(wall, zone).map((c) => c.instant)).toEqual([
      "2026-11-01T05:30:00.000Z",
      "2026-11-01T06:30:00.000Z",
    ]);
    expect(selectedCivilInstant(wall, undefined, zone)).toBe("");
    expect(selectedCivilInstant(wall, "2026-11-01T06:30:00.000Z", zone)).toBe(
      "2026-11-01T06:30:00.000Z",
    );
  });
  it("handles half-hour transitions and ordinary times", () => {
    expect(
      civilCandidates("2026-04-05T01:45", "Australia/Lord_Howe").map((c) => c.instant),
    ).toEqual(["2026-04-04T14:45:00.000Z", "2026-04-04T15:15:00.000Z"]);
    expect(selectedCivilInstant("2026-09-21T13:45", undefined, "UTC")).toBe(
      "2026-09-21T13:45:00.000Z",
    );
  });
});

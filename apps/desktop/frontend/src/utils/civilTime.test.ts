import { describe, expect, it, vi } from "vitest";
import {
  civilCandidates,
  localZone,
  offsetTimestamp,
  selectedCivilInstant,
  zonedWall,
} from "./civilTime";

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

// Where ICU cannot name the host's zone it reports "Etc/Unknown", which every
// formatter refuses, and Data Sources, Home and Plan failed to open.
describe("localZone", () => {
  it("is UTC where the computer's zone cannot be named", () => {
    const resolved = Intl.DateTimeFormat.prototype.resolvedOptions;
    const unknown = vi
      .spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions")
      .mockImplementation(function (this: Intl.DateTimeFormat) {
        return { ...resolved.call(this), timeZone: "Etc/Unknown" };
      });
    try {
      expect(localZone()).toBe("UTC");
    } finally {
      unknown.mockRestore();
    }
    expect(localZone()).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone);
  });
});

// A stored night is read in its own zone, and a time chosen on the form goes
// back with the offset of the occurrence meant.
describe("stored and chosen times", () => {
  it("reads a stored time as the wall clock of its zone, and keeps its instant", () => {
    expect(zonedWall("2026-10-25T14:00:00-04:00", "America/New_York")).toEqual({
      wall: "2026-10-25T14:00",
      at: "2026-10-25T18:00:00.000Z",
    });
    expect(zonedWall("2026-10-25T18:00:00.250Z", "Europe/London")).toEqual({
      wall: "2026-10-25T18:00",
      at: "2026-10-25T18:00:00.250Z",
    });
    expect(zonedWall("2026-03-01T22:30", "America/New_York")).toEqual({
      wall: "2026-03-01T22:30",
    });
  });
  it("writes each occurrence of a repeated hour with its own offset", () => {
    expect(offsetTimestamp("2026-11-01T01:30", "2026-11-01T05:30:00.000Z")).toBe(
      "2026-11-01T01:30:00-04:00",
    );
    expect(offsetTimestamp("2026-11-01T01:30", "2026-11-01T06:30:00.000Z")).toBe(
      "2026-11-01T01:30:00-05:00",
    );
    expect(offsetTimestamp("2026-04-05T01:45", "2026-04-04T14:45:00.000Z")).toBe(
      "2026-04-05T01:45:00+11:00",
    );
    expect(offsetTimestamp("2026-09-21T13:45", "2026-09-21T13:45:00.000Z")).toBe(
      "2026-09-21T13:45:00+00:00",
    );
  });
});

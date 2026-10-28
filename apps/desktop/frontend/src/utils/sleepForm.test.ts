import { describe, expect, it } from "vitest";
import type { SleepEntry } from "../data/sleepEntries";
import { correctionForm, occurrences, sleepFormInput, type SleepForm } from "./sleepForm";

const zone = "America/New_York";

function night(start: string, end: string): SleepEntry {
  return {
    reviewToken: "synthetic-review-token",
    needsReview: false,
    sourceWindowLabel: "Synthetic source window",
    activeEdits: [],
    canUndo: false,
    observationId: "observation_synthetic_01",
    startLocal: start,
    endLocal: end,
    startLabel: "Start",
    endLabel: "End",
    zoneId: zone,
    classification: "principal",
    effectiveStartLocal: start,
    effectiveEndLocal: end,
    effectiveStartLabel: "Start",
    effectiveEndLabel: "End",
    effectiveClassification: "principal",
    durationLabel: "8 hours",
    suppressed: false,
    sourceLabel: "Imported",
    provenanceLabel: "file import / directly observed",
    history: [],
  };
}

function form(start: string, end: string, extra: Partial<SleepForm> = {}): SleepForm {
  return {
    start: { wall: start },
    end: { wall: end },
    zoneId: zone,
    classification: "principal",
    ...extra,
  };
}

describe("the sleep form", () => {
  // The review of 2026-10-28 moved a wake time from 2:00 to 1:00 PM by
  // retyping "2026-10-25T14:00:00-04:00". The form now shows clock times.
  it("opens a correction at the night's clock times, keeping what is untouched exact", () => {
    const stored = night("2026-10-25T06:00:37-04:00", "2026-10-25T14:00:00-04:00");
    const opened = correctionForm(stored);
    expect(opened.start.wall).toBe("2026-10-25T06:00");
    expect(opened.end.wall).toBe("2026-10-25T14:00");

    const moved = { ...opened, end: { wall: "2026-10-25T13:00" } };
    expect(sleepFormInput(moved)).toEqual({
      input: {
        startLocal: "2026-10-25T06:00:37-04:00",
        endLocal: "2026-10-25T13:00:00-04:00",
        zoneId: zone,
        classification: "principal",
      },
    });
  });

  it("asks which one is meant when a clock time happens twice", () => {
    const repeated = form("2026-10-31T23:00", "2026-11-01T01:30");
    expect(occurrences(repeated.end, zone)?.length).toBe(2);
    expect(sleepFormInput(repeated)).toEqual({ problem: "Choose which 1:30 AM you mean." });

    const second = {
      ...repeated,
      end: { wall: "2026-11-01T01:30", at: "2026-11-01T06:30:00.000Z" },
    };
    expect(sleepFormInput(second)).toMatchObject({
      input: { endLocal: "2026-11-01T01:30:00-05:00" },
    });
  });

  it("says a skipped clock time did not happen", () => {
    const skipped = form("2027-03-13T23:00", "2027-03-14T02:30");
    expect(occurrences(skipped.end, zone)).toEqual([]);
    expect(sleepFormInput(skipped)).toEqual({
      problem: "2:30 AM did not happen that day in New York time: the clocks went forward.",
    });
  });

  it("refuses a wake before sleep, an empty time and an unknown zone", () => {
    expect(sleepFormInput(form("2026-10-25T06:00", "2026-10-25T05:00"))).toEqual({
      problem: "Wake time must be after sleep start.",
    });
    expect(sleepFormInput(form("", "2026-10-25T05:00"))).toEqual({
      problem: "Enter when you fell asleep.",
    });
    expect(
      sleepFormInput(form("2026-10-25T06:00", "2026-10-25T14:00", { zoneId: "Not/AZone" })),
    ).toEqual({ problem: "ZeitBoard does not know that time zone." });
  });

  it("never asks about an untouched stored time, even in the repeated hour", () => {
    const stored = night("2026-10-31T23:00:00-04:00", "2026-11-01T01:30:00-05:00");
    const opened = correctionForm(stored);
    expect(occurrences(opened.end, zone)).toBeUndefined();
    expect(sleepFormInput(opened)).toMatchObject({
      input: { endLocal: "2026-11-01T01:30:00-05:00" },
    });
  });
});

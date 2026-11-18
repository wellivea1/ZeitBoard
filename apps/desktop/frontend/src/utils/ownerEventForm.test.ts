import { describe, expect, it } from "vitest";
import { joinedWindows, ownerEventInput, sleepWindowOver } from "./ownerEventForm";

const zone = "America/New_York";

describe("what the event form sends", () => {
  it("reads a title and two wall-clock times as exact times", () => {
    const ready = ownerEventInput(
      {
        title: " Dentist ",
        start: { wall: "2026-07-08T15:00" },
        end: { wall: "2026-07-08T16:00" },
      },
      zone,
    );
    expect(ready).toEqual({
      input: {
        title: "Dentist",
        startLocal: "2026-07-08T15:00:00-04:00",
        endLocal: "2026-07-08T16:00:00-04:00",
        zoneId: zone,
      },
      startInstant: "2026-07-08T19:00:00.000Z",
      endInstant: "2026-07-08T20:00:00.000Z",
    });
  });

  it("names the one thing still wrong", () => {
    const at = (start: string, end: string, title = "Dentist") =>
      ownerEventInput({ title, start: { wall: start }, end: { wall: end } }, zone);
    expect(at("2026-07-08T15:00", "2026-07-08T16:00", "  ")).toEqual({
      problem: "Give the event a title.",
    });
    expect(at("", "2026-07-08T16:00")).toEqual({ problem: "Enter when it starts." });
    expect(at("2026-07-08T15:00", "")).toEqual({ problem: "Enter when it ends." });
    expect(at("2026-07-08T15:00", "2026-07-08T15:00")).toEqual({
      problem: "An event must end after it starts.",
    });
    expect(at("2026-07-08T15:00", "2026-07-09T15:01")).toEqual({
      problem: "An event can last at most a day.",
    });
    // A whole day is allowed.
    expect(at("2026-07-08T15:00", "2026-07-09T15:00")).toHaveProperty("input");
  });

  it("asks which hour is meant when the clocks go back, and refuses one that never happens", () => {
    const repeated = ownerEventInput(
      { title: "Call", start: { wall: "2026-11-01T01:30" }, end: { wall: "2026-11-01T03:00" } },
      zone,
    );
    expect(repeated).toHaveProperty("problem", expect.stringMatching(/^Choose which /));

    const skipped = ownerEventInput(
      { title: "Call", start: { wall: "2026-03-08T02:30" }, end: { wall: "2026-03-08T04:00" } },
      zone,
    );
    expect(skipped).toHaveProperty("problem", expect.stringMatching(/did not happen that day/));
  });
});

describe("predicted sleep windows beside an event", () => {
  const night = [
    { startAt: "2026-07-08T03:00:00Z", endAt: "2026-07-08T04:00:00Z" },
    // The calendar hands a night that crosses midnight over as two pieces.
    { startAt: "2026-07-08T04:00:00Z", endAt: "2026-07-08T11:00:00Z" },
    { startAt: "2026-07-09T05:00:00Z", endAt: "2026-07-09T12:00:00Z" },
  ];

  it("joins pieces that touch into one window", () => {
    expect(joinedWindows(night)).toEqual([
      { startAt: "2026-07-08T03:00:00Z", endAt: "2026-07-08T11:00:00Z" },
      { startAt: "2026-07-09T05:00:00Z", endAt: "2026-07-09T12:00:00Z" },
    ]);
  });

  it("finds the window an event falls in, and none for one that only touches it", () => {
    expect(sleepWindowOver(night, "2026-07-08T10:30:00Z", "2026-07-08T11:30:00Z")).toEqual({
      startAt: "2026-07-08T03:00:00Z",
      endAt: "2026-07-08T11:00:00Z",
    });
    expect(sleepWindowOver(night, "2026-07-08T11:00:00Z", "2026-07-08T12:00:00Z")).toBeUndefined();
    expect(sleepWindowOver([], "2026-07-08T10:30:00Z", "2026-07-08T11:30:00Z")).toBeUndefined();
  });
});

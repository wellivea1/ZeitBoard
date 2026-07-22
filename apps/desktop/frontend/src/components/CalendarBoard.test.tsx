import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { CalendarData, CalendarEventSegment } from "../data/calendar";
import { CalendarBoard } from "./CalendarBoard";
import { assignEventLanes } from "./calendarLayout";

function event(id: string, startMinute: number, endMinute: number): CalendarEventSegment {
  return {
    segmentId: `${id}_20260722`,
    eventId: id,
    sourceId: "calendar_source_test",
    sourceLabel: "Commitments",
    sourceKind: "ics",
    title: id,
    startAt: "2026-07-22T14:00:00Z",
    endAt: "2026-07-22T15:00:00Z",
    startLabel: "Jul 22, 10:00 AM EDT",
    endLabel: "Jul 22, 11:00 AM EDT",
    startMinute,
    endMinute,
    allDay: false,
    pointInTime: false,
    busy: true,
    ownership: "imported",
    readOnly: true,
    continuesBefore: false,
    continuesAfter: false,
  };
}

describe("CalendarBoard", () => {
  it("assigns overlapping events to separate lanes and reuses open lanes", () => {
    const layout = assignEventLanes([
      event("event_a", 60, 180),
      event("event_b", 120, 240),
      event("event_c", 240, 300),
    ]);
    expect(layout.count).toBe(2);
    expect(layout.lanes.get("event_a_20260722")).toBe(0);
    expect(layout.lanes.get("event_b_20260722")).toBe(1);
    expect(layout.lanes.get("event_c_20260722")).toBe(0);
  });

  it("opens exact local event details while retaining an accessible event table", () => {
    const selected = { ...event("Private appointment", 600, 660), location: "Private office" };
    const data: CalendarData = {
      status: "empty",
      message: "1 real event",
      fixtureMode: false,
      zoneId: "America/New_York",
      startCivilDate: "2026-07-22",
      endCivilDate: "2026-07-22",
      updatedLabel: "Updated now",
      sources: [
        {
          sourceId: "calendar_source_test",
          label: "Commitments",
          kind: "ics",
          readOnly: true,
          visibleEvents: 1,
          coverageLabel: "Current range",
          coverageStart: "2026-01-01T00:00:00Z",
          coverageEnd: "2027-01-01T00:00:00Z",
        },
      ],
      days: [
        {
          civilDate: "2026-07-22",
          label: "Wed, Jul 22",
          isToday: false,
          events: [selected],
          predictions: [],
        },
      ],
      warnings: [],
    };
    render(<CalendarBoard data={data} />);
    fireEvent.click(screen.getByRole("button", { name: /Private appointment/ }));
    expect(screen.getByRole("heading", { name: "Private appointment" })).toBeVisible();
    expect(screen.getByText("Private office")).toBeVisible();
    fireEvent.click(screen.getByText(/Event list/));
    expect(screen.getByRole("table")).toBeVisible();
    expect(screen.getByText("Blocks placement")).toBeVisible();
  });
});

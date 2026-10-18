import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { setWindowClock } from "../utils/windowClock";
import { WeekBoard } from "./WeekBoard";
import type { WeekColumn } from "./weekLayout";

function today(nowMinute: number): WeekColumn {
  return {
    civilDate: "2026-10-18",
    weekday: "Sun",
    dayNumber: "18",
    spokenDate: "Sunday 18 October, today",
    isToday: true,
    isPast: false,
    bands: [],
    allDay: [],
    blocks: [],
    doses: [],
    nowMinute,
  };
}

function gutter(nowMinute: number) {
  const { container } = render(
    <WeekBoard
      columns={[today(nowMinute)]}
      hour={40}
      deciding={false}
      onSelect={vi.fn()}
      onInspect={vi.fn()}
      onDecide={vi.fn()}
    />,
  );
  return [...container.querySelectorAll(".week-gutter span")].map((label) => label.textContent);
}

describe("WeekBoard", () => {
  // At 2:43 the time of now sat over "3 AM", which showed half of itself from
  // behind it.
  it("leaves off an hour that the time of now would cover", () => {
    expect(gutter(2 * 60 + 43)).toEqual(["6 AM", "9 AM", "Noon", "3 PM", "6 PM", "9 PM", "2:43"]);
    expect(gutter(4 * 60 + 30)).toContain("3 AM");
  });

  // The gutter's hours followed the window's clock and the time of now did
  // not: "4:30" among "15:00" and "18:00".
  it("writes the time of now on the window's clock", () => {
    setWindowClock("24");
    try {
      expect(gutter(16 * 60 + 30)).toEqual([
        "03:00",
        "06:00",
        "09:00",
        "12:00",
        "15:00",
        "18:00",
        "21:00",
        "16:30",
      ]);
    } finally {
      setWindowClock("12");
    }
  });
});

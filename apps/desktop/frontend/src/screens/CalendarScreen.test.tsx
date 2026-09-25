import { fireEvent, render, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { civilProbeLabel } from "../components/timeProbeLogic";
import { todayCivilDate } from "../data/calendar";
import { CalendarScreen } from "./CalendarScreen";

describe("CalendarScreen", () => {
  it("maps hover positions from structured civil dates and qualifies forecast spans", async () => {
    const { container } = render(<CalendarScreen />);
    await waitFor(() => expect(container.querySelector(".calendar-day-track")).not.toBeNull());
    const track = container.querySelector(".calendar-day-track");
    expect(track).not.toBeNull();
    vi.spyOn(track as Element, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      top: 0,
      right: 240,
      bottom: 40,
      left: 0,
      width: 240,
      height: 40,
      toJSON: () => ({}),
    } as DOMRect);

    fireEvent.pointerMove(track as Element, { clientX: 20 });

    expect(
      container.querySelector(".time-probe:not([hidden]) .time-probe-label"),
    ).toHaveTextContent(
      civilProbeLabel(todayCivilDate("America/New_York"), 120, {
        predicted: true,
        approximate: true,
      }),
    );
  });
});

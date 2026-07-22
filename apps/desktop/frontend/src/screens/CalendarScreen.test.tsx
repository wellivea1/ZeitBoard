import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { CalendarScreen } from "./CalendarScreen";

describe("CalendarScreen", () => {
  it("maps hover positions from structured civil dates and qualifies forecast spans", () => {
    const { container } = render(<CalendarScreen />);
    const track = container.querySelector(".day-track");
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
    ).toHaveTextContent("Mon Jun 15 · ~02:00 · predicted");
  });
});

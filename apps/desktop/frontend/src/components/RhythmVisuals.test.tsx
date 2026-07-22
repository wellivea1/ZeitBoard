import { Profiler } from "react";
import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { rhythmFixture } from "../data/rhythm";
import { ActogramPanel, CycleStrip, DriftPanel } from "./RhythmVisuals";

function setTrackBounds(element: Element, width: number) {
  vi.spyOn(element, "getBoundingClientRect").mockReturnValue({
    x: 0,
    y: 0,
    top: 0,
    right: width,
    bottom: 40,
    left: 0,
    width,
    height: 40,
    toJSON: () => ({}),
  } as DOMRect);
}

function visibleProbeLabel(container: HTMLElement) {
  return container.querySelector<HTMLElement>(".time-probe:not([hidden]) .time-probe-label");
}

describe("chronological time probes", () => {
  it("resolves the actogram second plot to the following civil day without rerendering", () => {
    let commits = 0;
    const { container } = render(
      <Profiler id="actogram" onRender={() => commits++}>
        <ActogramPanel actogram={rhythmFixture.actogram} />
      </Profiler>,
    );
    const observedRow = container.querySelector(".actogram-visual-row");
    const forecastRow = Array.from(container.querySelectorAll(".actogram-visual-row")).at(-1);
    const observedTrack = observedRow?.querySelector(".actogram-visual-track");
    const track = forecastRow?.querySelector(".actogram-visual-track");
    expect(observedTrack).not.toBeNull();
    expect(track).not.toBeNull();
    setTrackBounds(observedTrack as Element, 480);
    setTrackBounds(track as Element, 480);

    fireEvent.pointerMove(track as Element, { clientX: 360 });

    expect(visibleProbeLabel(container)).toHaveTextContent("Fri Jun 19 · 12:00 · predicted");
    expect(commits).toBe(1);

    fireEvent.pointerMove(observedTrack as Element, { clientX: 120 });

    expect(container.querySelectorAll(".time-probe:not([hidden])")).toHaveLength(1);
    expect(visibleProbeLabel(container)).toHaveTextContent("Mon Jun 15 · 12:00");
    expect(commits).toBe(1);

    fireEvent.pointerLeave(observedTrack as Element);

    expect(visibleProbeLabel(container)).toBeNull();
    expect(commits).toBe(1);
  });

  it("qualifies predicted positions on the Overview cycle strip", () => {
    const { container } = render(
      <CycleStrip
        actogram={rhythmFixture.actogram}
        usefulWindowLabel="Today, 3:00 PM to 6:00 PM"
        sleepWindowLabel="Today, 10:15 PM to 1:27 AM"
      />,
    );
    const track = container.querySelector(".cycle-strip-track");
    expect(track).not.toBeNull();
    setTrackBounds(track as Element, 240);

    fireEvent.pointerMove(track as Element, { clientX: 228 });

    expect(visibleProbeLabel(container)).toHaveTextContent("Tue Jun 16 · ~22:48 · predicted");
  });

  it("snaps the drift probe to the nearest cycle and reports observed and fitted onset", () => {
    const { container } = render(<DriftPanel drift={rhythmFixture.drift} />);
    const plot = container.querySelector(".drift-plot");
    expect(plot).not.toBeNull();
    setTrackBounds(plot as Element, 480);

    fireEvent.pointerMove(plot as Element, { clientX: 40 });

    expect(visibleProbeLabel(container)).toHaveTextContent("Jun 10 · onset 17:48 (fit 17:45)");
  });
});

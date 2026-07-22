import { Profiler } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
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
    const forecastToggle = screen.getByLabelText("Show forecast");
    expect(forecastToggle).not.toBeChecked();
    fireEvent.click(forecastToggle);
    const commitsAfterToggle = commits;
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
    expect(commits).toBe(commitsAfterToggle);

    fireEvent.pointerMove(observedTrack as Element, { clientX: 120 });

    expect(container.querySelectorAll(".time-probe:not([hidden])")).toHaveLength(1);
    expect(visibleProbeLabel(container)).toHaveTextContent("Mon Jun 15 · 12:00");
    expect(commits).toBe(commitsAfterToggle);

    fireEvent.pointerLeave(observedTrack as Element);

    expect(visibleProbeLabel(container)).toBeNull();
    expect(commits).toBe(commitsAfterToggle);
  });

  it("renders self-reported context with exact present-type legend and table text", () => {
    const { container } = render(
      <ActogramPanel
        actogram={rhythmFixture.actogram}
        markers={[
          {
            markerId: "marker_travel_01",
            kind: "travel",
            kindLabel: "Travel / time-zone context",
            startAt: "2026-06-15T23:00:00Z",
            zoneId: "America/New_York",
            civilDate: "2026-06-15",
            hour: 19,
            startLabel: "Jun 15, 7:00 PM",
            rangeLabel: "Jun 15, 7:00 PM onward",
            note: "Flight arrival",
            recordedLabel: "Jun 16, 1:00 PM",
          },
          {
            markerId: "marker_outside_01",
            kind: "illness",
            kindLabel: "Illness / health disruption",
            startAt: "2026-05-01T12:00:00Z",
            zoneId: "America/New_York",
            civilDate: "2026-05-01",
            hour: 8,
            startLabel: "May 1, 8:00 AM",
            rangeLabel: "May 1, 8:00 AM onward",
            recordedLabel: "May 1, 9:00 AM",
          },
          {
            markerId: "marker_wrong_zone_01",
            kind: "illness",
            kindLabel: "Illness / health disruption",
            startAt: "2026-06-14T13:00:00Z",
            zoneId: "America/Chicago",
            civilDate: "2026-06-14",
            hour: 8,
            startLabel: "Jun 14, 8:00 AM",
            rangeLabel: "Jun 14, 8:00 AM onward",
            note: "Different-zone marker",
            recordedLabel: "Jun 14, 1:00 PM",
          },
        ]}
      />,
    );

    expect(
      screen.getByRole("img", {
        name: /Travel \/ time-zone context, self-reported context:.*Flight arrival/,
      }),
    ).toBeVisible();
    const legend = screen.getByLabelText("Context marker legend");
    expect(legend).toHaveTextContent("Travel / time-zone context");
    expect(legend).not.toHaveTextContent("Illness / health disruption");
    expect(screen.getByText(/2 context markers fall outside/)).toBeVisible();
    expect(container.querySelectorAll(".actogram-marker")).toHaveLength(2);
    const table = container.querySelector(".sr-table");
    expect(table).toHaveTextContent("Flight arrival");
    expect(table).not.toHaveTextContent("Different-zone marker");
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

import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import type { SleepEntry, SleepOverlap } from "../data/sleepEntries";
import { SleepOverlapList } from "./SleepOverlapList";

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

function record(id: string, provenanceLabel: string, start: string, end: string): SleepEntry {
  return {
    observationId: id,
    reviewToken: `synthetic-review-token-${id}`,
    needsReview: false,
    sourceWindowLabel: "Synthetic source window",
    activeEdits: [],
    canUndo: false,
    startLocal: start,
    endLocal: end,
    startLabel: start,
    endLabel: end,
    zoneId: "America/New_York",
    classification: "principal",
    effectiveStartLocal: start,
    effectiveEndLocal: end,
    effectiveStartLabel: start,
    effectiveEndLabel: end,
    effectiveClassification: "principal",
    durationLabel: "8 hours 0 minutes",
    suppressed: false,
    sourceLabel: "Synthetic sleep",
    provenanceLabel,
    history: [],
  };
}

// Logged 11 PM to 7 AM, worn 11:30 PM to 7:10 AM; the estimator uses the middle.
const overlap: SleepOverlap = {
  startLocal: "2026-01-13T23:15",
  endLocal: "2026-01-14T07:05",
  startLabel: "Tue Jan 13, 11:15 PM EST",
  endLabel: "Wed Jan 14, 7:05 AM EST",
  startApartMinutes: 30,
  endApartMinutes: 10,
  records: [
    record("obs_logged", "manual / user reported", "2026-01-13T23:00", "2026-01-14T07:00"),
    record(
      "obs_worn",
      "health connect / directly observed",
      "2026-01-13T23:30",
      "2026-01-14T07:10",
    ),
  ],
};

describe("SleepOverlapList", () => {
  it("draws each record beside the night the estimator uses", () => {
    const { container } = render(<SleepOverlapList overlaps={[overlap]} count={1} />);

    expect(screen.getByRole("heading", { name: /Nights recorded more than once/ })).toBeVisible();
    expect(screen.getByText("Tue, Jan 13")).toBeVisible();
    expect(screen.getByText("Starts 30 minutes apart, ends 10 minutes apart")).toBeVisible();
    expect(screen.getByText("Logged by you")).toBeVisible();
    expect(screen.getByText("Health Connect")).toBeVisible();
    expect(screen.getByText("Estimate uses")).toBeVisible();

    // One scale per night: from the earliest start (11 PM) to the latest end
    // (7:10 AM), 490 minutes. The logged night starts at its left edge.
    const bars = [...container.querySelectorAll<HTMLElement>(".sleep-overlap-track > span")];
    expect(bars).toHaveLength(3);
    expect(bars[0]!.style.left).toBe("0%");
    expect(parseFloat(bars[1]!.style.left)).toBeCloseTo((30 / 490) * 100);
    expect(parseFloat(bars[2]!.style.width)).toBeCloseTo((470 / 490) * 100);
    // The estimator's night is told apart without colour.
    expect(bars[2]!.closest("li")).toHaveAttribute("data-merged");
    // Only the whole list is counted; a short list says it is the newest.
    expect(screen.queryByText(/The newest/)).toBeNull();
  });

  it("leaves a record out of estimates in one click", async () => {
    const excluded: unknown[] = [];
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          SuppressSleepEntry: async (input: unknown) => {
            excluded.push(input);
            return { ...overlap.records[1], suppressed: true };
          },
        },
      },
    };
    const changes: Event[] = [];
    const listen = (event: Event) => changes.push(event);
    window.addEventListener(sleepDataChangedEvent, listen);
    render(<SleepOverlapList overlaps={[overlap]} count={1} />);

    const worn = screen.getByRole("button", {
      name: /Exclude 11:30 PM – 7:10 AM .*Health Connect/,
    });
    fireEvent.click(worn);

    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent(
      "Tue, Jan 13, 11:30 PM – 7:10 AM is left out of estimates. Undo is on its row in the sleep log.",
    );
    expect(excluded).toEqual([
      { observationId: "obs_worn", reviewToken: "synthetic-review-token-obs_worn" },
    ]);
    expect(changes).toHaveLength(1);
    window.removeEventListener(sleepDataChangedEvent, listen);
  });

  it("says when it lists only the newest", () => {
    render(<SleepOverlapList overlaps={[overlap]} count={24} />);
    expect(within(screen.getByRole("heading")).getByText("24")).toBeVisible();
    expect(screen.getByText("The newest 1 of 24 are listed.")).toBeVisible();
  });

  it("shows nothing when every night has one record", () => {
    const { container } = render(<SleepOverlapList overlaps={[]} count={0} />);
    expect(container).toBeEmptyDOMElement();
  });
});

import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { SleepLogPanel } from "./SleepLogPanel";

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("SleepLogPanel", () => {
  // Moved out of Data Sources in slice U-H. The behaviour is unchanged and the
  // test came with it: a long history must page rather than either flooding the
  // screen or quietly dropping evidence.
  it("bounds sleep entries and correction history without discarding evidence", async () => {
    const corrections = Array.from({ length: 51 }, (_, index) => ({
      correctionId: `correction_${index + 1}`,
      createdLabel: `Created ${String(index + 1).padStart(3, "0")}`,
      reason: "owner correction",
      summary: `Correction ${index + 1}`,
    }));
    const entries = Array.from({ length: 51 }, (_, index) => {
      const number = index + 1;
      return {
        observationId: `observation_${number}`,
        startLocal: "2026-03-01T22:00",
        endLocal: "2026-03-02T06:00",
        startLabel: `Start ${number}`,
        endLabel: `End ${number}`,
        zoneId: "America/New_York",
        classification: "principal",
        effectiveStartLocal: "2026-03-01T22:00",
        effectiveEndLocal: "2026-03-02T06:00",
        effectiveStartLabel: `Effective start ${number}`,
        effectiveEndLabel: `Effective end ${number}`,
        effectiveClassification: "principal",
        durationLabel: "8 hours 0 minutes",
        suppressed: false,
        sourceLabel: "Manual sleep log",
        provenanceLabel: "manual / user reported",
        history: index === 0 ? corrections : [],
      };
    });
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          ListSleepEntries: async () => ({
            status: "ready",
            empty: false,
            message: "51 local sleep entries stored on this device.",
            entries,
          }),
        },
      },
    };

    render(<SleepLogPanel />);

    expect(await screen.findByText("Entries 1-50 of 51")).toBeVisible();
    expect(screen.getByText("Effective start 1 to Effective end 1")).toBeVisible();
    expect(screen.queryByText("Effective start 51 to Effective end 51")).not.toBeInTheDocument();

    expect(screen.queryByText("Created 001")).not.toBeInTheDocument();
    const history = screen.getByText("Correction history (51)").closest("details");
    expect(history).not.toBeNull();
    history!.open = true;
    fireEvent(history!, new Event("toggle"));
    expect(screen.getByText("Created 001")).toBeVisible();
    expect(screen.queryByText("Created 051")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Next corrections" }));
    expect(screen.getByText("Created 051")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Next entries" }));
    expect(screen.getByText("Entries 51-51 of 51")).toBeVisible();
    expect(screen.getByText("Effective start 51 to Effective end 51")).toBeVisible();
  });

  it("carries the entry form and the log together", () => {
    render(<SleepLogPanel />);
    expect(screen.getByRole("heading", { name: "Add sleep entry" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Sleep log" })).toBeVisible();
    // Source configuration stayed on Data Sources.
    expect(screen.queryByRole("heading", { name: "Source status" })).toBeNull();
  });
});

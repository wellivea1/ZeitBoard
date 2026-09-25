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
    // One night per day from Thu, Jan 1, so each row names a different date.
    const day = (index: number, hour: string) => {
      const date = new Date(2026, 0, 1 + index);
      const pad = (value: number) => String(value).padStart(2, "0");
      return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${hour}`;
    };
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
        effectiveStartLocal: day(index, "22:00"),
        effectiveEndLocal: day(index + 1, "06:00"),
        effectiveStartLabel: `Effective start ${number}`,
        effectiveEndLabel: `Effective end ${number}`,
        effectiveClassification: "principal",
        reviewToken: "synthetic-review-token",
        needsReview: false,
        sourceWindowLabel: "Synthetic source window",
        activeEdits: [],
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
    expect(screen.getByText("Thu, Jan 1")).toBeVisible();
    expect(screen.queryByText("Fri, Feb 20")).not.toBeInTheDocument();

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
    expect(screen.getByText("Fri, Feb 20")).toBeVisible();
  });

  it("carries the entry form and the log together", () => {
    render(<SleepLogPanel />);
    // The form for a missed night folds away under the quick buttons.
    expect(screen.getByRole("form", { name: "Add sleep entry" })).toBeInTheDocument();
    expect(screen.getByText("Add a past night")).toBeVisible();
    expect(screen.getByRole("heading", { name: "Sleep log" })).toBeVisible();
    // Source configuration stayed on Data Sources.
    expect(screen.queryByRole("heading", { name: "Connected" })).toBeNull();
  });

  it("opens the past-night form when it is the address", () => {
    window.location.hash = "#/log/sleep";
    const { container } = render(<SleepLogPanel />);
    const fold = container.querySelector<HTMLDetailsElement>("details.sleep-add")!;
    expect(fold.open).toBe(false);

    // Home's "Add a past night", followed while Log is already open.
    window.location.hash = "#/log/sleep/add";
    fireEvent(window, new HashChangeEvent("hashchange"));
    expect(fold.open).toBe(true);
    expect(fold.contains(document.activeElement)).toBe(true);
    window.location.hash = "";
  });
});

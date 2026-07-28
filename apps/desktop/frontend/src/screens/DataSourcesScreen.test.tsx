import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { DataSourcesScreen } from "./DataSourcesScreen";

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("DataSourcesScreen structure", () => {
  it("leads with provenance in one ruled workspace", () => {
    const { container } = render(<DataSourcesScreen />);
    const workspace = screen.getByRole("region", { name: "Data source review" });
    const sourceHeading = screen.getByRole("heading", { name: "Source status" });
    const entryHeading = screen.getByRole("heading", { name: "Add sleep entry" });

    expect(workspace.querySelector(".panel")).toBeNull();
    expect(
      sourceHeading.compareDocumentPosition(entryHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);
    expect(container).toHaveTextContent(
      "Local ICS files and read-only CalDAV snapshots are managed in Calendar",
    );
    expect(container).not.toHaveTextContent("Out of scope for this local sleep-data slice");
  });

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

    render(<DataSourcesScreen />);

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
});

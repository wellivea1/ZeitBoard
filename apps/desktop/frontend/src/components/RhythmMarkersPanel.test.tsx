import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { RhythmMarkersData } from "../data/rhythmMarkers";
import { RhythmMarkersPanel } from "./RhythmMarkersPanel";

const readyData: RhythmMarkersData = {
  status: "ready",
  empty: false,
  message: "1 self-reported context marker. It does not establish cause.",
  fixtureMode: false,
  updatedLabel: "Updated Jul 22, 12:00 PM",
  markers: [
    {
      markerId: "marker_test_01",
      kind: "travel",
      kindLabel: "Travel / time-zone context",
      startAt: "2026-07-22T13:00:00Z",
      endAt: "2026-07-22T15:00:00Z",
      zoneId: "America/New_York",
      civilDate: "2026-07-22",
      hour: 9,
      startLabel: "Jul 22, 2026, 9:00 AM",
      endLabel: "Jul 22, 2026, 11:00 AM",
      rangeLabel: "Jul 22, 2026, 9:00 AM to Jul 22, 2026, 11:00 AM",
      note: "Private travel context",
      recordedLabel: "Jul 22, 2026, 12:00 PM",
    },
  ],
};

function renderPanel(
  data = readyData,
  onAdd = vi.fn(async () => undefined),
  onDelete = vi.fn(async () => undefined),
) {
  const onExport = vi.fn();
  render(
    <RhythmMarkersPanel
      data={data}
      busy={false}
      exporting={false}
      error=""
      announcement=""
      onAdd={onAdd}
      onDelete={onDelete}
      onExport={onExport}
    />,
  );
  return { onAdd, onDelete, onExport };
}

describe("RhythmMarkersPanel", () => {
  it("states the non-causal boundary and submits a dense append-only entry", async () => {
    const { onAdd } = renderPanel();
    expect(screen.getByText(/do not change the estimate, establish cause/i)).toBeVisible();
    expect(screen.getByText(/never edited in place/i)).toBeVisible();
    expect(screen.getByText("Owner export includes private notes.")).toBeVisible();

    fireEvent.change(screen.getByLabelText("Context type"), {
      target: { value: "illness" },
    });
    fireEvent.change(screen.getByLabelText("Started"), {
      target: { value: "2026-07-21T08:00" },
    });
    fireEvent.change(screen.getByLabelText("Ended (optional)"), {
      target: { value: "2026-07-21T10:00" },
    });
    fireEvent.change(screen.getByLabelText("IANA time zone"), {
      target: { value: "America/Chicago" },
    });
    fireEvent.change(screen.getByLabelText("Private note (optional)"), {
      target: { value: "  self reported context  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Append marker" }));

    await waitFor(() =>
      expect(onAdd).toHaveBeenCalledWith({
        kind: "illness",
        startLocal: "2026-07-21T08:00",
        endLocal: "2026-07-21T10:00",
        zoneId: "America/Chicago",
        note: "self reported context",
      }),
    );
  });

  it("keeps permanent erase distinct from suppression and requires typed DELETE", async () => {
    const { onDelete } = renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Erase" }));
    expect(screen.getByText(/distinct from suppressing an observation/i)).toBeVisible();
    const erase = screen.getByRole("button", { name: "Permanently erase" });
    expect(erase).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Type DELETE"), { target: { value: "DELETE" } });
    fireEvent.click(erase);
    await waitFor(() => expect(onDelete).toHaveBeenCalledWith("marker_test_01", "DELETE"));
  });

  it("does not substitute sample context when the desktop service is absent", () => {
    const unavailable: RhythmMarkersData = {
      status: "unavailable",
      empty: true,
      message: "Desktop service unavailable. Sample markers are not substituted.",
      markers: [],
      fixtureMode: false,
      updatedLabel: "Desktop service unavailable",
    };
    renderPanel(unavailable);
    expect(screen.getByText("This browser preview does not invent health context.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Append marker" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Export markers" })).toBeDisabled();
  });
});

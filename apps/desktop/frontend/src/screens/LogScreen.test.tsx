import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LogScreen } from "./LogScreen";

const marker = {
  markerId: "marker_screen_01",
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
};

const markerResponse = {
  status: "ready",
  empty: false,
  message: "1 self-reported context marker. It does not establish cause.",
  markers: [marker],
  fixtureMode: false,
  updatedLabel: "Updated Jul 22, 12:00 PM",
};

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

// Context markers moved from Rhythm to Log in slice U-H: recording that you
// travelled or were ill is logging, and it belongs beside the sleep and dose
// records rather than beside the charts that interpret them. The ownership rule
// came with them.
describe("LogScreen marker ownership", () => {
  it("keeps an authoritative mutation result without immediately loading all markers again", async () => {
    const getMarkers = vi.fn(async () => structuredClone(markerResponse));
    const addMarker = vi.fn(async () => structuredClone(markerResponse));
    (globalThis as { go?: unknown }).go = {
      main: { App: { GetRhythmMarkers: getMarkers, AddRhythmMarker: addMarker } },
    };

    render(<LogScreen tab="markers" onSelect={() => {}} />);
    expect(await screen.findByText("Private travel context")).toBeVisible();
    expect(getMarkers).toHaveBeenCalledTimes(1);

    fireEvent.change(screen.getByLabelText("Context type"), { target: { value: "illness" } });
    fireEvent.change(screen.getByLabelText("Started"), {
      target: { value: "2026-07-23T08:00" },
    });
    fireEvent.change(screen.getByLabelText("IANA time zone"), {
      target: { value: "America/New_York" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Append marker" }));

    await waitFor(() => expect(addMarker).toHaveBeenCalledTimes(1));
    expect(getMarkers).toHaveBeenCalledTimes(1);
  });
});

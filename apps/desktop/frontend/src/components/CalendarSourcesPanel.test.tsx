import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { CalendarSource } from "../data/calendar";
import { CalendarSourcesPanel } from "./CalendarSourcesPanel";

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("CalendarSourcesPanel", () => {
  it("requires typed confirmation before erasing an imported snapshot", async () => {
    const remove = vi.fn(async () => undefined);
    const onChanged = vi.fn();
    (globalThis as { go?: unknown }).go = {
      main: { App: { RemoveCalendarSource: remove } },
    };
    const sources: CalendarSource[] = [
      {
        sourceId: "calendar_source_imported",
        label: "Commitments",
        kind: "ics",
        readOnly: true,
        visibleEvents: 2,
        coverageLabel: "Jan 1 to Dec 31",
        coverageStart: "2026-01-01T00:00:00Z",
        coverageEnd: "2027-01-01T00:00:00Z",
      },
      {
        sourceId: "calendar_source_zeitboard",
        label: "ZeitBoard placements",
        kind: "zeitboard",
        readOnly: false,
        visibleEvents: 1,
        coverageLabel: "Local placements",
        coverageStart: "2026-01-01T00:00:00Z",
        coverageEnd: "2027-01-01T00:00:00Z",
      },
    ];
    render(<CalendarSourcesPanel sources={sources} available onChanged={onChanged} />);

    expect(screen.getAllByRole("button", { name: "Remove" })).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    const erase = screen.getByRole("button", { name: "Erase source" });
    expect(erase).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/Type REMOVE/), { target: { value: "REMOVE" } });
    fireEvent.click(erase);

    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
    expect(remove).toHaveBeenCalledWith({
      sourceId: "calendar_source_imported",
      confirmation: "REMOVE",
    });
  });
});

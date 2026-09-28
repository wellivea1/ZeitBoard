import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { medicationDataChangedEvent, type MedicationLog } from "../data/medications";
import { MedicationHistory } from "./MedicationHistory";

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

function medicationEvent(index: number): MedicationLog {
  return {
    eventId: `event_${index}`,
    medicationId: "medication_01",
    medicationLabel: "Recorded medication",
    doseAt: "2026-07-22T02:15:00Z",
    doseLocal: "2026-07-21T22:15",
    civilTime: `Event time ${index}`,
    zoneId: "America/New_York",
    status: "taken",
    scheduled: false,
    note: `Event note ${index}`,
    recordedLabel: `Recorded event ${index}`,
    wakeRelation: "8 hours after recorded wake",
    sleepRelation: "2 hours before predicted sleep",
    sleepRelationKind: "predicted",
    confidence: "Medium",
    excluded: false,
    correctionCount: 0,
  };
}

// The desktop's history, fifty doses a page, as GetMedicationHistoryPage reads it.
function desktopHistory(events: MedicationLog[]) {
  const read = vi.fn(async ({ page }: { page: number }) => {
    const last = Math.max(0, Math.ceil(events.length / 50) - 1);
    const shown = Math.min(page, last);
    return {
      status: events.length ? "ready" : "empty",
      message: `${events.length} recorded doses.`,
      total: events.length,
      page: shown,
      pageSize: 50,
      events: events.slice(shown * 50, shown * 50 + 50),
    };
  });
  (globalThis as { go?: unknown }).go = { main: { App: { GetMedicationHistoryPage: read } } };
  return read;
}

describe("MedicationHistory", () => {
  it("reads a page at a time while keeping every dose reachable", async () => {
    const read = desktopHistory(
      Array.from({ length: 51 }, (_, index) => medicationEvent(index + 1)),
    );
    render(
      <MedicationHistory
        busy={false}
        onCorrect={vi.fn(async () => undefined)}
        onDelete={vi.fn(async () => undefined)}
      />,
    );

    expect(await screen.findByText("Events 1-50 of 51")).toBeVisible();
    expect(screen.getByText("51 stored")).toBeVisible();
    expect(screen.getByText("Event note 1")).toBeVisible();
    expect(screen.queryByText("Event note 51")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Next events" }));
    expect(await screen.findByText("Events 51-51 of 51")).toBeVisible();
    expect(screen.getByText("Event note 51")).toBeVisible();
    expect(screen.queryByText("Event note 1")).not.toBeInTheDocument();
    expect(read).toHaveBeenLastCalledWith({ page: 1 });
  });

  it("reads its page again when medication data changes", async () => {
    const events = [medicationEvent(1)];
    desktopHistory(events);
    render(
      <MedicationHistory
        busy={false}
        onCorrect={vi.fn(async () => undefined)}
        onDelete={vi.fn(async () => undefined)}
      />,
    );
    expect(await screen.findByText("Event note 1")).toBeVisible();

    events.splice(0, 1);
    await act(async () => {
      window.dispatchEvent(new Event(medicationDataChangedEvent));
    });
    expect(await screen.findByText("No events recorded")).toBeVisible();
  });
});

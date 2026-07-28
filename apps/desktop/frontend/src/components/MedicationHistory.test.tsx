import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { MedicationLog } from "../data/medications";
import { MedicationHistory } from "./MedicationHistory";

function medicationEvent(index: number): MedicationLog {
  return {
    eventId: `event_${index}`,
    medicationId: "medication_01",
    medicationLabel: "Recorded medication",
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

describe("MedicationHistory", () => {
  it("renders a bounded event page while retaining navigation to all evidence", () => {
    const events = Array.from({ length: 51 }, (_, index) => medicationEvent(index + 1));
    render(
      <MedicationHistory
        events={events}
        busy={false}
        onCorrect={vi.fn(async () => undefined)}
        onDelete={vi.fn(async () => undefined)}
      />,
    );

    expect(screen.getByText("Events 1-50 of 51")).toBeVisible();
    expect(screen.getByText("Event note 1")).toBeVisible();
    expect(screen.queryByText("Event note 51")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Next events" }));
    expect(screen.getByText("Events 51-51 of 51")).toBeVisible();
    expect(screen.getByText("Event note 51")).toBeVisible();
    expect(screen.queryByText("Event note 1")).not.toBeInTheDocument();
  });
});

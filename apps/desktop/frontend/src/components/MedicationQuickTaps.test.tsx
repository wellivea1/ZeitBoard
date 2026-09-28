import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { MedicationDefinition, MedicationLog } from "../data/medications";
import { MedicationQuickTaps } from "./MedicationQuickTaps";

const medication = {
  medicationId: "med_synthetic",
  label: "Synthetic tablet",
  detailLabel: "",
  active: true,
  revision: 1,
  scheduleKind: "fixed_clock",
  schedule: {
    kind: "fixed_clock",
    zoneId: "Asia/Tokyo",
    civilTimes: ["21:00"],
    reminderEnabled: false,
    summary: "Daily at 21:00",
  },
} as unknown as MedicationDefinition;

function dose(
  eventId: string,
  status: "taken" | "skipped",
  doseAt: string,
  doseLocal: string,
  zoneId: string,
) {
  return {
    eventId,
    medicationId: "med_synthetic",
    status,
    doseAt,
    doseLocal,
    zoneId,
  } as unknown as MedicationLog;
}

describe("the one-tap dose row", () => {
  it("names a schedule's zone when this computer is elsewhere", () => {
    render(
      <MedicationQuickTaps medications={[medication]} available busy={false} onLog={vi.fn()} />,
    );
    expect(screen.getByText(/Usually at 9:00 PM Tokyo time/)).toBeVisible();
    expect(screen.queryByText(/Last (taken|skipped)/)).toBeNull();
  });

  // The desktop picks the newest dose, by its instant (the Go tests pin that);
  // the row words the one it is given.
  it("words the medication's newest dose", () => {
    const lastDose = dose(
      "dose_new_york",
      "skipped",
      "2026-09-27T04:46:00Z",
      "2026-09-27T00:46",
      "America/New_York",
    );
    render(
      <MedicationQuickTaps
        medications={[{ ...medication, eventCount: 2, lastDose }]}
        available
        busy={false}
        onLog={vi.fn()}
      />,
    );
    expect(screen.getByText(/Last skipped/)).toBeVisible();
    expect(screen.queryByText(/Last taken/)).toBeNull();
  });
});

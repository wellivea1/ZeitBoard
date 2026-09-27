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
      <MedicationQuickTaps
        medications={[medication]}
        events={[]}
        available
        busy={false}
        onLog={vi.fn()}
      />,
    );
    expect(screen.getByText(/Usually at 9:00 PM Tokyo time/)).toBeVisible();
  });

  it("takes the latest dose by its instant, not by each zone's own clock", () => {
    // Recorded on two devices in different zones: the later dose has the
    // earlier civil clock, so ordering by the civil strings picks the wrong one.
    const events = [
      dose("dose_utc", "taken", "2026-09-27T04:41:00Z", "2026-09-27T04:41", "UTC"),
      dose(
        "dose_new_york",
        "skipped",
        "2026-09-27T04:46:00Z",
        "2026-09-27T00:46",
        "America/New_York",
      ),
    ];
    render(
      <MedicationQuickTaps
        medications={[medication]}
        events={events}
        available
        busy={false}
        onLog={vi.fn()}
      />,
    );
    expect(screen.getByText(/Last skipped/)).toBeVisible();
    expect(screen.queryByText(/Last taken/)).toBeNull();
  });
});

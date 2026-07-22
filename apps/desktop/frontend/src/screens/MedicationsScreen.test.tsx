import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MedicationsScreen } from "./MedicationsScreen";

const response = {
  status: "ready",
  empty: false,
  message: "1 medication and 1 medication event stored only on this device.",
  estimateStatus: "estimated",
  estimateMessage: "Current rhythm estimate available for recent-event context.",
  medications: [
    {
      medicationId: "med_local_01",
      label: "Evening record",
      form: "tablet",
      detailLabel: "tablet",
      active: true,
      revision: 1,
      scheduleKind: "none",
      createdLabel: "Added Jul 21, 2026",
      eventCount: 1,
    },
  ],
  events: [
    {
      eventId: "dose_local_01",
      medicationId: "med_local_01",
      medicationLabel: "Evening record",
      doseLocal: "2026-07-21T22:15",
      civilTime: "Tue Jul 21, 10:15 PM EDT",
      zoneId: "America/New_York",
      status: "taken",
      scheduled: false,
      note: "Original factual note",
      recordedLabel: "Recorded Jul 21, 10:16 PM",
      wakeRelation: "8 h 15 min after recorded wake",
      sleepRelation: "1 h 45 min before predicted sleep",
      sleepRelationKind: "predicted",
      confidence: "Medium",
      excluded: false,
      correctionCount: 0,
    },
  ],
  fixtureMode: false,
  disclaimer:
    "Medication timing shown here is user-entered or derived context, not medical advice.",
  interactionDisclaimer:
    "ZeitBoard records what you enter. It does not check medication interactions; ask a pharmacist or clinician.",
  updatedLabel: "Updated Jul 22, 8:00 AM",
};

type GlobalWithGo = typeof globalThis & { go?: unknown };

afterEach(() => {
  delete (globalThis as GlobalWithGo).go;
});

describe("MedicationsScreen", () => {
  it("shows an honest disabled workspace without a desktop service", async () => {
    render(<MedicationsScreen />);

    expect(await screen.findByText("Desktop service unavailable")).toBeInTheDocument();
    expect(screen.queryByText(/sample preview/i)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Export medication data" })).toBeDisabled();
    expect(screen.getByText("No interaction checking")).toBeInTheDocument();
  });

  it("appends corrections and gates real event erasure behind typed confirmation", async () => {
    const current = structuredClone(response);
    const getMedications = vi.fn(async () => structuredClone(current));
    const correct = vi.fn(async (input: unknown) => {
      const correction = input as { note: string };
      current.events[0]!.note = correction.note;
      current.events[0]!.correctionCount = 1;
      return structuredClone(current);
    });
    const erase = vi.fn(async () => {
      current.events = [];
      current.medications[0]!.eventCount = 0;
      current.message = "1 medication and 0 medication events stored only on this device.";
      return structuredClone(current);
    });
    (globalThis as GlobalWithGo).go = {
      main: {
        App: {
          GetMedications: getMedications,
          CorrectMedicationEvent: correct,
          DeleteMedicationEvent: erase,
        },
      },
    };

    render(<MedicationsScreen />);

    expect(await screen.findByText("Original factual note")).toBeInTheDocument();
    expect(screen.getByText("1 h 45 min before predicted sleep")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Correct" }));

    const correctionForm = screen.getByRole("form", { name: "Correct Evening record event" });
    fireEvent.change(within(correctionForm).getByLabelText("Private note"), {
      target: { value: "Corrected factual note" },
    });
    fireEvent.click(within(correctionForm).getByRole("button", { name: "Append correction" }));

    await waitFor(() => expect(correct).toHaveBeenCalledTimes(1));
    expect(await screen.findByText("Corrected factual note")).toBeInTheDocument();
    expect(screen.getByText("1 correction")).toBeInTheDocument();

    const ledgerRow = screen.getByText("Corrected factual note").closest("article");
    expect(ledgerRow).not.toBeNull();
    fireEvent.click(within(ledgerRow as HTMLElement).getByRole("button", { name: "Erase" }));

    const eraseRegion = screen.getByRole("region", { name: "Erase medication event" });
    const eraseButton = within(eraseRegion).getByRole("button", { name: "Erase event" });
    expect(eraseButton).toBeDisabled();
    expect(erase).not.toHaveBeenCalled();

    fireEvent.change(within(eraseRegion).getByLabelText("Type DELETE"), {
      target: { value: "DELETE" },
    });
    expect(eraseButton).toBeEnabled();
    fireEvent.click(eraseButton);

    await waitFor(() =>
      expect(erase).toHaveBeenCalledWith({
        eventId: "dose_local_01",
        confirmation: "DELETE",
      }),
    );
    expect(await screen.findByText("No events recorded")).toBeInTheDocument();
  });
});

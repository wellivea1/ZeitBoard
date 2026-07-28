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
  reminderStatus: "disabled",
  reminderMessage: "Desktop reminders are off. Enable them only on a clock schedule you entered.",
  updatedLabel: "Updated Jul 22, 8:00 AM",
};

const scheduledResponse = {
  ...response,
  reminderStatus: "ready",
  reminderMessage: "Desktop reminders are active for 1 medication you configured.",
  medications: [
    {
      ...response.medications[0],
      revision: 2,
      scheduleKind: "fixed_clock",
      clinicianRule: "Use the clinician-provided written schedule",
      clinicianRuleAttribution: "Clinician guidance entered verbatim by you",
      schedule: {
        kind: "fixed_clock",
        zoneId: "UTC",
        civilTimes: ["09:00"],
        reminderEnabled: true,
        summary: "09:00 in UTC",
        forecast: {
          status: "no_overlap",
          message:
            "None of 1 covered scheduled occurrences fall inside current predicted sleep windows. 1 later occurrence is outside the current forecast horizon.",
          coveredCount: 1,
          collisionCount: 0,
          outsideHorizonCount: 1,
          coverageEndsAt: "2026-07-23T15:00:00Z",
          coverageLabel: "Jul 23, 3:00 PM UTC",
          occurrences: [
            {
              at: "2026-07-23T09:00:00Z",
              civilDate: "2026-07-23",
              civilTime: "09:00",
              civilLabel: "Thu Jul 23, 9:00 AM UTC",
              status: "outside_predicted_sleep",
              context: "Not inside a current predicted sleep window",
              confidence: "Medium",
              ambiguous: false,
            },
            {
              at: "2026-07-24T09:00:00Z",
              civilDate: "2026-07-24",
              civilTime: "09:00",
              civilLabel: "Fri Jul 24, 9:00 AM UTC",
              status: "outside_forecast",
              context: "Outside the current forecast horizon",
              confidence: "Unknown",
              ambiguous: false,
            },
          ],
          gaps: [],
        },
      },
    },
  ],
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
    expect(getMedications).toHaveBeenCalledTimes(1);
  });

  it("saves an explicit schedule and renders a dense neutral forecast", async () => {
    let current: unknown = structuredClone(response);
    const getMedications = vi.fn(async () => structuredClone(current));
    const updateSchedule = vi.fn(async () => {
      current = structuredClone(scheduledResponse);
      return structuredClone(current);
    });
    (globalThis as GlobalWithGo).go = {
      main: {
        App: {
          GetMedications: getMedications,
          UpdateMedicationSchedule: updateSchedule,
        },
      },
    };

    render(<MedicationsScreen />);

    expect(await screen.findByText("No medication schedules stored")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Schedule Evening record" }));
    const editor = screen.getByRole("region", { name: "Schedule: Evening record" });
    fireEvent.change(within(editor).getByLabelText("Schedule type"), {
      target: { value: "fixed_clock" },
    });
    fireEvent.change(within(editor).getByLabelText(/^Schedule time zone/), {
      target: { value: "UTC" },
    });
    const reminderOptIn = within(editor).getByLabelText(/Show this label through Windows/);
    expect(reminderOptIn.closest("label")).toHaveTextContent("including predicted sleep overlaps");
    fireEvent.click(reminderOptIn);
    fireEvent.change(within(editor).getByLabelText(/^Clinician guidance, entered by you/), {
      target: { value: "Use the clinician-provided written schedule" },
    });
    fireEvent.click(within(editor).getByRole("button", { name: "Save schedule revision" }));

    await waitFor(() => expect(updateSchedule).toHaveBeenCalledTimes(1));
    expect(updateSchedule).toHaveBeenCalledWith({
      medicationId: "med_local_01",
      revision: 1,
      kind: "fixed_clock",
      zoneId: "UTC",
      civilTimes: ["09:00"],
      daysOn: 0,
      daysOff: 0,
      cycleStartedOn: "",
      reminderEnabled: true,
      clinicianRule: "Use the clinician-provided written schedule",
    });
    expect(await screen.findByText("Schedule feasibility")).toBeInTheDocument();
    expect(screen.getByText("Use the clinician-provided written schedule")).toBeInTheDocument();
    expect(screen.getByText("Not inside a current predicted sleep window")).toBeInTheDocument();
    expect(screen.getByText("Outside the current forecast horizon")).toBeInTheDocument();
    expect(screen.queryByText(/good time|bad time|safe time/i)).not.toBeInTheDocument();
    expect(screen.getByRole("table")).toBeInTheDocument();
  });

  it("records an optional civil medication start for descriptive comparison", async () => {
    const current = structuredClone(response);
    Object.assign(current.medications[0]!, {
      startedAt: "2026-06-20T05:12:00Z",
      startedLocal: "2026-06-20T01:12",
      startedZoneId: "America/New_York",
      startedLabel: "Jun 20, 2026, 1:12 AM EDT",
    });
    const getMedications = vi.fn(async () => structuredClone(current));
    const updateMedication = vi.fn(async (input: unknown) => {
      const revision = input as { startedLocal: string; startedZoneId: string };
      Object.assign(current.medications[0]!, {
        revision: 2,
        startedAt: "2026-06-21T06:30:00Z",
        startedLocal: revision.startedLocal,
        startedZoneId: revision.startedZoneId,
        startedLabel: "Jun 21, 2026, 2:30 AM EDT",
      });
      return structuredClone(current);
    });
    (globalThis as GlobalWithGo).go = {
      main: {
        App: {
          GetMedications: getMedications,
          UpdateMedication: updateMedication,
        },
      },
    };

    render(<MedicationsScreen />);

    expect(await screen.findByText(/Start marker: Jun 20/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    const editor = screen.getByRole("region", { name: "Edit medication" });
    expect(within(editor).getByText(/does not establish a medication effect/)).toBeInTheDocument();
    fireEvent.change(within(editor).getByLabelText("Local date and time"), {
      target: { value: "2026-06-21T02:30" },
    });
    fireEvent.change(within(editor).getByLabelText("IANA time zone"), {
      target: { value: "America/New_York" },
    });
    fireEvent.click(within(editor).getByRole("button", { name: "Save revision" }));

    await waitFor(() => expect(updateMedication).toHaveBeenCalledTimes(1));
    expect(updateMedication).toHaveBeenCalledWith({
      medicationId: "med_local_01",
      revision: 1,
      label: "Evening record",
      form: "tablet",
      strengthLabel: "",
      active: true,
      startedLocal: "2026-06-21T02:30",
      startedZoneId: "America/New_York",
    });
    expect(await screen.findByText(/Start marker: Jun 21/)).toBeInTheDocument();
  });
});

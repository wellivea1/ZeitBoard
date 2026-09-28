import { describe, expect, it, vi } from "vitest";

import {
  addMedication,
  correctMedicationEvent,
  deleteMedication,
  deleteMedicationEvent,
  exportMedicationData,
  hasLocalMedicationService,
  loadMedications,
  loadMedicationHistoryPage,
  logMedicationEvent,
  normalizeMedicationHistoryPage,
  normalizeMedications,
  updateMedication,
  updateMedicationSchedule,
} from "./medications";

const medicationResponse = {
  status: "ready",
  empty: false,
  message: "1 medication and 1 recorded dose.",
  estimateStatus: "estimated",
  estimateMessage: "Current rhythm estimate available for recent-event context.",
  medications: [
    {
      medicationId: "med_local_01",
      label: "Evening record",
      form: "tablet",
      strengthLabel: "private strength",
      detailLabel: "tablet - private strength",
      active: true,
      revision: 1,
      scheduleKind: "none",
      createdLabel: "Added Jul 21, 2026",
      eventCount: 1,
      lastDose: {
        eventId: "dose_local_01",
        medicationId: "med_local_01",
        medicationLabel: "Evening record",
        doseAt: "2026-07-22T02:15:00Z",
        doseLocal: "2026-07-21T22:15",
        civilTime: "Tue Jul 21, 10:15 PM EDT",
        zoneId: "America/New_York",
        status: "taken",
        scheduled: false,
        note: "With water",
        recordedLabel: "Recorded Jul 21, 10:16 PM",
        wakeRelation: "8 h 15 min after recorded wake",
        sleepRelation: "1 h 45 min before predicted sleep",
        sleepRelationKind: "predicted",
        confidence: "Medium",
        excluded: false,
        correctionCount: 0,
      },
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
  ...medicationResponse,
  reminderStatus: "ready",
  reminderMessage: "Desktop reminders are active for 1 medication you configured.",
  medications: [
    {
      ...medicationResponse.medications[0],
      revision: 2,
      scheduleKind: "fixed_clock",
      clinicianRule: "Clinician instruction entered by the user",
      clinicianRuleAttribution: "Clinician guidance entered verbatim by you",
      schedule: {
        kind: "fixed_clock",
        zoneId: "UTC",
        civilTimes: ["09:00"],
        reminderEnabled: true,
        summary: "09:00 in UTC",
        forecast: {
          status: "collision",
          message:
            "1 of 1 covered scheduled occurrences fall inside current predicted sleep windows. 1 later occurrence is outside the current forecast horizon.",
          coveredCount: 1,
          collisionCount: 1,
          outsideHorizonCount: 1,
          coverageEndsAt: "2026-07-23T15:00:00Z",
          coverageLabel: "Jul 23, 3:00 PM UTC",
          occurrences: [
            {
              at: "2026-07-23T09:00:00Z",
              civilDate: "2026-07-23",
              civilTime: "09:00",
              civilLabel: "Thu Jul 23, 9:00 AM UTC",
              status: "inside_predicted_sleep",
              context: "Inside a current predicted sleep window",
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

const exportJSON = JSON.stringify({
  schema_version: "v2",
  generated_at: "2026-07-22T12:00:00Z",
  medication_set: {
    schema_version: "v2",
    generated_at: "2026-07-22T12:00:00Z",
    medications: [{}],
  },
  event_set: {
    schema_version: "v2",
    generated_at: "2026-07-22T12:00:00Z",
    events: [{}],
    corrections: [],
  },
});

describe("medication data adapter", () => {
  it("normalizes real local records and keeps excluded evidence in stored counts", () => {
    expect(normalizeMedications(medicationResponse)).toMatchObject({
      status: "ready",
      fixtureMode: false,
      medications: [
        {
          scheduleKind: "none",
          eventCount: 1,
          lastDose: { sleepRelationKind: "predicted", confidence: "Medium" },
        },
      ],
    });

    const excluded = structuredClone(medicationResponse);
    excluded.medications[0]!.lastDose.excluded = true;
    expect(normalizeMedications(excluded)).toBeDefined();
  });

  it("accepts only complete, valid medication start markers", () => {
    const started = structuredClone(medicationResponse);
    Object.assign(started.medications[0]!, {
      startedAt: "2026-06-20T05:12:00Z",
      startedLocal: "2026-06-20T01:12",
      startedZoneId: "America/New_York",
      startedLabel: "Started Jun 20, 2026, 1:12 AM EDT",
    });
    expect(normalizeMedications(started)?.medications[0]).toMatchObject({
      startedLocal: "2026-06-20T01:12",
      startedZoneId: "America/New_York",
    });

    const partial = structuredClone(started);
    delete (partial.medications[0] as Record<string, unknown>).startedZoneId;
    expect(normalizeMedications(partial)).toBeUndefined();

    const malformed = structuredClone(started);
    Object.assign(malformed.medications[0]!, { startedAt: "not-a-timestamp" });
    expect(normalizeMedications(malformed)).toBeUndefined();
  });

  it("rejects contradictory ownership, counts, identifiers, and civil times", () => {
    // A medication's newest dose must be its own.
    const unknownMedication = structuredClone(medicationResponse);
    unknownMedication.medications[0]!.lastDose.medicationId = "med_missing_01";
    expect(normalizeMedications(unknownMedication)).toBeUndefined();

    const labelMismatch = structuredClone(medicationResponse);
    labelMismatch.medications[0]!.lastDose.medicationLabel = "Different private label";
    expect(normalizeMedications(labelMismatch)).toBeUndefined();

    // And there exactly when it has doses.
    const countMismatch = structuredClone(medicationResponse);
    countMismatch.medications[0]!.eventCount = 0;
    expect(normalizeMedications(countMismatch)).toBeUndefined();

    const missingDose = structuredClone(medicationResponse);
    delete (missingDose.medications[0] as Record<string, unknown>).lastDose;
    expect(normalizeMedications(missingDose)).toBeUndefined();

    const duplicate = structuredClone(medicationResponse);
    duplicate.medications.push(structuredClone(duplicate.medications[0]!));
    expect(normalizeMedications(duplicate)).toBeUndefined();

    const impossibleTime = structuredClone(medicationResponse);
    impossibleTime.medications[0]!.lastDose.doseLocal = "2026-02-30T22:15";
    expect(normalizeMedications(impossibleTime)).toBeUndefined();
  });

  it("reads the dose history a page at a time", async () => {
    const dose = medicationResponse.medications[0]!.lastDose;
    const page = {
      status: "ready",
      message: "51 recorded doses.",
      total: 51,
      page: 1,
      pageSize: 50,
      events: [dose],
    };
    expect(normalizeMedicationHistoryPage(page)).toMatchObject({
      total: 51,
      page: 1,
      events: [{ eventId: "dose_local_01" }],
    });
    expect(normalizeMedicationHistoryPage({ ...page, status: "empty" })).toBeUndefined();
    expect(normalizeMedicationHistoryPage({ ...page, total: 0 })).toBeUndefined();
    expect(
      normalizeMedicationHistoryPage({ ...page, events: [{ ...dose, doseLocal: "soon" }] }),
    ).toBeUndefined();

    const read = vi.fn(async () => page);
    const root = { go: { main: { App: { GetMedicationHistoryPage: read } } } };
    await expect(loadMedicationHistoryPage(1, root)).resolves.toMatchObject({ total: 51 });
    expect(read).toHaveBeenCalledWith({ page: 1 });
    await expect(loadMedicationHistoryPage(0, {})).resolves.toMatchObject({
      status: "empty",
      events: [],
    });
  });

  it("reconciles schedule shape, horizon counts, context, and reminder state", () => {
    expect(normalizeMedications(scheduledResponse)).toMatchObject({
      reminderStatus: "ready",
      medications: [
        {
          scheduleKind: "fixed_clock",
          clinicianRuleAttribution: "Clinician guidance entered verbatim by you",
          schedule: {
            reminderEnabled: true,
            forecast: { coveredCount: 1, collisionCount: 1, outsideHorizonCount: 1 },
          },
        },
      ],
    });

    const badCount = structuredClone(scheduledResponse);
    badCount.medications[0]!.schedule.forecast.outsideHorizonCount = 0;
    expect(normalizeMedications(badCount)).toBeUndefined();

    const badContext = structuredClone(scheduledResponse);
    badContext.medications[0]!.schedule.forecast.occurrences[0]!.context = "Good time";
    expect(normalizeMedications(badContext)).toBeUndefined();

    const badKind = structuredClone(scheduledResponse);
    badKind.medications[0]!.scheduleKind = "cycling";
    expect(normalizeMedications(badKind)).toBeUndefined();

    const badReminder = structuredClone(scheduledResponse);
    badReminder.reminderStatus = "disabled";
    expect(normalizeMedications(badReminder)).toBeUndefined();
  });

  it("uses an honest no-fixture state when the desktop bridge is absent", async () => {
    expect(hasLocalMedicationService({})).toBe(false);
    await expect(loadMedications({})).resolves.toMatchObject({
      status: "unavailable",
      empty: true,
      fixtureMode: false,
      medications: [],
    });
    await expect(
      addMedication({ label: "Private", form: "", strengthLabel: "" }, {}),
    ).rejects.toThrow(/desktop service/);
  });

  it("passes mutations through named methods and requires DELETE for erasure", async () => {
    const methods = {
      GetMedications: vi.fn(async () => medicationResponse),
      AddMedication: vi.fn(async () => medicationResponse),
      UpdateMedication: vi.fn(async () => medicationResponse),
      UpdateMedicationSchedule: vi.fn(async () => medicationResponse),
      LogMedicationEvent: vi.fn(async () => medicationResponse),
      CorrectMedicationEvent: vi.fn(async () => medicationResponse),
      DeleteMedication: vi.fn(async () => medicationResponse),
      DeleteMedicationEvent: vi.fn(async () => medicationResponse),
    };
    const root = { go: { main: { App: methods } } };
    const definition = { label: "Private", form: "tablet", strengthLabel: "label" };
    const update = {
      ...definition,
      medicationId: "med_local_01",
      revision: 1,
      active: false,
    };
    const event = {
      medicationId: "med_local_01",
      doseAt: "2026-07-22T02:15:00Z",
      doseLocal: "2026-07-21T22:15",
      zoneId: "America/New_York",
      status: "taken" as const,
      scheduled: false,
      note: "",
    };
    const schedule = {
      medicationId: "med_local_01",
      revision: 1,
      kind: "fixed_clock" as const,
      zoneId: "UTC",
      civilTimes: ["09:00"],
      daysOn: 0,
      daysOff: 0,
      cycleStartedOn: "",
      reminderEnabled: true,
      clinicianRule: "",
    };

    await expect(loadMedications(root)).resolves.toMatchObject({ status: "ready" });
    await addMedication(definition, root);
    await updateMedication(update, root);
    await updateMedicationSchedule(schedule, root);
    await logMedicationEvent(event, root);
    await correctMedicationEvent({ ...event, eventId: "dose_local_01", excluded: true }, root);
    await deleteMedication("med_local_01", root);
    await deleteMedicationEvent("dose_local_01", root);

    expect(methods.AddMedication).toHaveBeenCalledWith(definition);
    expect(methods.UpdateMedication).toHaveBeenCalledWith(update);
    expect(methods.UpdateMedicationSchedule).toHaveBeenCalledWith(schedule);
    expect(methods.LogMedicationEvent).toHaveBeenCalledWith(event);
    expect(methods.DeleteMedication).toHaveBeenCalledWith({
      medicationId: "med_local_01",
      confirmation: "DELETE",
    });
    expect(methods.DeleteMedicationEvent).toHaveBeenCalledWith({
      eventId: "dose_local_01",
      confirmation: "DELETE",
    });
  });

  it("validates export identity, nested versions, and declared counts", async () => {
    const exportValue = {
      fileName: "zeitboard-medication-data-v2-20260722.json",
      json: exportJSON,
      generatedAt: "2026-07-22T12:00:00Z",
      generatedLabel: "Jul 22, 2026, 8:00 AM",
      medicationCount: 1,
      eventCount: 1,
    };
    const root = {
      go: { main: { App: { ExportMedicationData: vi.fn(async () => exportValue) } } },
    };
    await expect(exportMedicationData(root)).resolves.toEqual(exportValue);

    const badCount = structuredClone(exportValue);
    badCount.eventCount = 2;
    const invalidRoot = {
      go: { main: { App: { ExportMedicationData: async () => badCount } } },
    };
    await expect(exportMedicationData(invalidRoot)).rejects.toThrow(/declared counts/);

    const unsupported = structuredClone(exportValue);
    unsupported.json = exportJSON.replaceAll('"v2"', '"v1"');
    const unsupportedRoot = {
      go: { main: { App: { ExportMedicationData: async () => unsupported } } },
    };
    await expect(exportMedicationData(unsupportedRoot)).rejects.toThrow(/declared counts/);
  });
});

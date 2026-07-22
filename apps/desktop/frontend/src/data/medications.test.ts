import { describe, expect, it, vi } from "vitest";

import {
  addMedication,
  correctMedicationEvent,
  deleteMedication,
  deleteMedicationEvent,
  exportMedicationData,
  hasLocalMedicationService,
  loadMedications,
  logMedicationEvent,
  normalizeMedications,
  updateMedication,
} from "./medications";

const medicationResponse = {
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
      strengthLabel: "private strength",
      detailLabel: "tablet - private strength",
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
      note: "With water",
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

const exportJSON = JSON.stringify({
  schema_version: "v1",
  generated_at: "2026-07-22T12:00:00Z",
  medication_set: {
    schema_version: "v1",
    generated_at: "2026-07-22T12:00:00Z",
    medications: [{}],
  },
  event_set: {
    schema_version: "v1",
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
      medications: [{ scheduleKind: "none", eventCount: 1 }],
      events: [{ sleepRelationKind: "predicted", confidence: "Medium" }],
    });

    const excluded = structuredClone(medicationResponse);
    excluded.events[0]!.excluded = true;
    expect(normalizeMedications(excluded)).toBeDefined();
  });

  it("rejects contradictory ownership, counts, identifiers, and civil times", () => {
    const unknownMedication = structuredClone(medicationResponse);
    unknownMedication.events[0]!.medicationId = "med_missing_01";
    expect(normalizeMedications(unknownMedication)).toBeUndefined();

    const labelMismatch = structuredClone(medicationResponse);
    labelMismatch.events[0]!.medicationLabel = "Different private label";
    expect(normalizeMedications(labelMismatch)).toBeUndefined();

    const countMismatch = structuredClone(medicationResponse);
    countMismatch.medications[0]!.eventCount = 0;
    expect(normalizeMedications(countMismatch)).toBeUndefined();

    const duplicate = structuredClone(medicationResponse);
    duplicate.events.push(structuredClone(duplicate.events[0]!));
    duplicate.medications[0]!.eventCount = 2;
    expect(normalizeMedications(duplicate)).toBeUndefined();

    const impossibleTime = structuredClone(medicationResponse);
    impossibleTime.events[0]!.doseLocal = "2026-02-30T22:15";
    expect(normalizeMedications(impossibleTime)).toBeUndefined();
  });

  it("uses an honest no-fixture state when the desktop bridge is absent", async () => {
    expect(hasLocalMedicationService({})).toBe(false);
    await expect(loadMedications({})).resolves.toMatchObject({
      status: "unavailable",
      empty: true,
      fixtureMode: false,
      medications: [],
      events: [],
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
      doseLocal: "2026-07-21T22:15",
      zoneId: "America/New_York",
      status: "taken" as const,
      scheduled: false,
      note: "",
    };

    await expect(loadMedications(root)).resolves.toMatchObject({ status: "ready" });
    await addMedication(definition, root);
    await updateMedication(update, root);
    await logMedicationEvent(event, root);
    await correctMedicationEvent({ ...event, eventId: "dose_local_01", excluded: true }, root);
    await deleteMedication("med_local_01", root);
    await deleteMedicationEvent("dose_local_01", root);

    expect(methods.AddMedication).toHaveBeenCalledWith(definition);
    expect(methods.UpdateMedication).toHaveBeenCalledWith(update);
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
      fileName: "zeitboard-medication-data-20260722.json",
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
  });
});

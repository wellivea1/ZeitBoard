import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export const medicationDataChangedEvent = "zeitboard:medication-data-changed";
export const medicationDeleteConfirmation = "DELETE";

export type MedicationEventStatus = "taken" | "skipped";
export type MedicationEstimateStatus = "estimated" | "empty" | "refused" | "unavailable";
export type MedicationSleepRelationKind = "observed" | "predicted" | "unavailable";

export interface MedicationDefinition {
  medicationId: string;
  label: string;
  form?: string;
  strengthLabel?: string;
  detailLabel: string;
  active: boolean;
  revision: number;
  scheduleKind: "none" | "as_needed" | "fixed_clock" | "cycling";
  createdLabel: string;
  eventCount: number;
}

export interface MedicationLog {
  eventId: string;
  medicationId: string;
  medicationLabel: string;
  doseLocal: string;
  civilTime: string;
  zoneId: string;
  status: MedicationEventStatus;
  scheduled: boolean;
  note?: string;
  recordedLabel: string;
  wakeRelation: string;
  sleepRelation: string;
  sleepRelationKind: MedicationSleepRelationKind;
  confidence: "High" | "Medium" | "Low" | "Unknown";
  excluded: boolean;
  correctionCount: number;
}

export interface MedicationsData {
  status: "ready" | "empty" | "unavailable";
  empty: boolean;
  message: string;
  estimateStatus: MedicationEstimateStatus;
  estimateMessage: string;
  medications: MedicationDefinition[];
  events: MedicationLog[];
  fixtureMode: false;
  disclaimer: string;
  interactionDisclaimer: string;
  updatedLabel: string;
}

export interface MedicationInput {
  label: string;
  form: string;
  strengthLabel: string;
}

export interface MedicationUpdateInput extends MedicationInput {
  medicationId: string;
  revision: number;
  active: boolean;
}

export interface MedicationEventInput {
  medicationId: string;
  doseLocal: string;
  zoneId: string;
  status: MedicationEventStatus;
  scheduled: boolean;
  note: string;
}

export interface MedicationEventCorrectionInput extends Omit<MedicationEventInput, "medicationId"> {
  eventId: string;
  excluded: boolean;
}

export interface MedicationExport {
  fileName: string;
  json: string;
  generatedAt: string;
  generatedLabel: string;
  medicationCount: number;
  eventCount: number;
}

type UnknownRecord = Record<string, unknown>;

const identifierPattern = /^[a-z][a-z0-9_-]{2,63}$/;
const localDateTimePattern = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/;
const timestampPattern = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

const unavailableMedications: MedicationsData = {
  status: "unavailable",
  empty: true,
  message: "Medication logging requires the ZeitBoard desktop service.",
  estimateStatus: "unavailable",
  estimateMessage: "Rhythm context is unavailable in this browser preview.",
  medications: [],
  events: [],
  fixtureMode: false,
  disclaimer:
    "Medication timing shown here is user-entered or derived context, not medical advice.",
  interactionDisclaimer:
    "ZeitBoard records what you enter. It does not check medication interactions; ask a pharmacist or clinician.",
  updatedLabel: "Desktop service unavailable",
};

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function text(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() === value && value.length > 0
    ? value
    : undefined;
}

function optionalText(value: unknown): string | undefined {
  return value === undefined || value === "" ? undefined : text(value);
}

function identifier(value: unknown): string | undefined {
  const candidate = text(value);
  return candidate && identifierPattern.test(candidate) ? candidate : undefined;
}

function nonNegativeInteger(value: unknown): number | undefined {
  return typeof value === "number" && Number.isInteger(value) && value >= 0 ? value : undefined;
}

function positiveInteger(value: unknown): number | undefined {
  return typeof value === "number" && Number.isInteger(value) && value >= 1 ? value : undefined;
}

function timestamp(value: unknown): string | undefined {
  const candidate = text(value);
  return candidate && timestampPattern.test(candidate) && !Number.isNaN(Date.parse(candidate))
    ? candidate
    : undefined;
}

function localDateTime(value: unknown): string | undefined {
  const candidate = text(value);
  if (!candidate || !localDateTimePattern.test(candidate)) return undefined;
  const [date, clock] = candidate.split("T");
  const parsed = new Date(`${date}T${clock}:00Z`);
  return !Number.isNaN(parsed.valueOf()) && parsed.toISOString().slice(0, 16) === candidate
    ? candidate
    : undefined;
}

function normalizeMedication(value: unknown): MedicationDefinition | undefined {
  if (!isRecord(value)) return undefined;
  const medicationId = identifier(value.medicationId);
  const label = text(value.label);
  const form = optionalText(value.form);
  const strengthLabel = optionalText(value.strengthLabel);
  const detailLabel = text(value.detailLabel);
  const revision = positiveInteger(value.revision);
  const scheduleKind =
    value.scheduleKind === "none" ||
    value.scheduleKind === "as_needed" ||
    value.scheduleKind === "fixed_clock" ||
    value.scheduleKind === "cycling"
      ? value.scheduleKind
      : undefined;
  const createdLabel = text(value.createdLabel);
  const eventCount = nonNegativeInteger(value.eventCount);
  if (
    !medicationId ||
    !label ||
    !detailLabel ||
    typeof value.active !== "boolean" ||
    revision === undefined ||
    !scheduleKind ||
    !createdLabel ||
    eventCount === undefined
  ) {
    return undefined;
  }
  return {
    medicationId,
    label,
    ...(form ? { form } : {}),
    ...(strengthLabel ? { strengthLabel } : {}),
    detailLabel,
    active: value.active,
    revision,
    scheduleKind,
    createdLabel,
    eventCount,
  };
}

function normalizeLog(value: unknown): MedicationLog | undefined {
  if (!isRecord(value)) return undefined;
  const eventId = identifier(value.eventId);
  const medicationId = identifier(value.medicationId);
  const medicationLabel = text(value.medicationLabel);
  const doseLocal = localDateTime(value.doseLocal);
  const civilTime = text(value.civilTime);
  const zoneId = text(value.zoneId);
  const status = value.status === "taken" || value.status === "skipped" ? value.status : undefined;
  const note = optionalText(value.note);
  const recordedLabel = text(value.recordedLabel);
  const wakeRelation = text(value.wakeRelation);
  const sleepRelation = text(value.sleepRelation);
  const sleepRelationKind =
    value.sleepRelationKind === "observed" ||
    value.sleepRelationKind === "predicted" ||
    value.sleepRelationKind === "unavailable"
      ? value.sleepRelationKind
      : undefined;
  const confidence =
    value.confidence === "High" ||
    value.confidence === "Medium" ||
    value.confidence === "Low" ||
    value.confidence === "Unknown"
      ? value.confidence
      : undefined;
  const correctionCount = nonNegativeInteger(value.correctionCount);
  if (
    !eventId ||
    !medicationId ||
    !medicationLabel ||
    !doseLocal ||
    !civilTime ||
    !zoneId ||
    !status ||
    typeof value.scheduled !== "boolean" ||
    !recordedLabel ||
    !wakeRelation ||
    !sleepRelation ||
    !sleepRelationKind ||
    !confidence ||
    typeof value.excluded !== "boolean" ||
    correctionCount === undefined
  ) {
    return undefined;
  }
  return {
    eventId,
    medicationId,
    medicationLabel,
    doseLocal,
    civilTime,
    zoneId,
    status,
    scheduled: value.scheduled,
    ...(note ? { note } : {}),
    recordedLabel,
    wakeRelation,
    sleepRelation,
    sleepRelationKind,
    confidence,
    excluded: value.excluded,
    correctionCount,
  };
}

export function normalizeMedications(value: unknown): MedicationsData | undefined {
  if (!isRecord(value) || !Array.isArray(value.medications) || !Array.isArray(value.events)) {
    return undefined;
  }
  const status =
    value.status === "ready" || value.status === "empty" || value.status === "unavailable"
      ? value.status
      : undefined;
  const message = text(value.message);
  const estimateStatus =
    value.estimateStatus === "estimated" ||
    value.estimateStatus === "empty" ||
    value.estimateStatus === "refused" ||
    value.estimateStatus === "unavailable"
      ? value.estimateStatus
      : undefined;
  const estimateMessage = text(value.estimateMessage);
  const disclaimer = text(value.disclaimer);
  const interactionDisclaimer = text(value.interactionDisclaimer);
  const updatedLabel = text(value.updatedLabel);
  if (
    !status ||
    typeof value.empty !== "boolean" ||
    !message ||
    !estimateStatus ||
    !estimateMessage ||
    value.fixtureMode !== false ||
    !disclaimer ||
    !interactionDisclaimer ||
    !updatedLabel
  ) {
    return undefined;
  }
  const medications: MedicationDefinition[] = [];
  const medicationIds = new Set<string>();
  for (const item of value.medications) {
    const medication = normalizeMedication(item);
    if (!medication || medicationIds.has(medication.medicationId)) return undefined;
    medicationIds.add(medication.medicationId);
    medications.push(medication);
  }
  const labels = new Map(
    medications.map((medication) => [medication.medicationId, medication.label]),
  );
  const actualCounts = new Map<string, number>();
  const events: MedicationLog[] = [];
  const eventIds = new Set<string>();
  for (const item of value.events) {
    const event = normalizeLog(item);
    if (
      !event ||
      eventIds.has(event.eventId) ||
      labels.get(event.medicationId) !== event.medicationLabel
    ) {
      return undefined;
    }
    eventIds.add(event.eventId);
    actualCounts.set(event.medicationId, (actualCounts.get(event.medicationId) ?? 0) + 1);
    events.push(event);
  }
  if (
    medications.some(
      (medication) => (actualCounts.get(medication.medicationId) ?? 0) !== medication.eventCount,
    ) ||
    value.empty !== (medications.length === 0) ||
    (status === "ready" && value.empty) ||
    (status === "empty" && !value.empty) ||
    (status === "unavailable" && (!value.empty || events.length > 0)) ||
    (medications.length === 0 && events.length > 0)
  ) {
    return undefined;
  }
  return {
    status,
    empty: value.empty,
    message,
    estimateStatus,
    estimateMessage,
    medications,
    events,
    fixtureMode: false,
    disclaimer,
    interactionDisclaimer,
    updatedLabel,
  };
}

export function hasLocalMedicationService(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): boolean {
  return Boolean(findWailsMethod(root, ["GetMedications"]));
}

export async function loadMedications(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<MedicationsData> {
  const method = findWailsMethod(root, ["GetMedications"]);
  if (!method) return unavailableMedications;
  const result = normalizeMedications(await method());
  if (!result) throw new Error("Medication service returned an invalid response.");
  return result;
}

async function medicationMutation(
  methodName: string,
  input: unknown,
  root: WailsRoot,
): Promise<MedicationsData> {
  const method = findWailsMethod(root, [methodName]);
  if (!method) throw new Error("Medication logging requires the ZeitBoard desktop service.");
  const result = normalizeMedications(await method(input));
  if (!result) throw new Error("Medication service returned an invalid response.");
  return result;
}

export function addMedication(
  input: MedicationInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return medicationMutation("AddMedication", input, root);
}

export function updateMedication(
  input: MedicationUpdateInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return medicationMutation("UpdateMedication", input, root);
}

export function logMedicationEvent(
  input: MedicationEventInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return medicationMutation("LogMedicationEvent", input, root);
}

export function correctMedicationEvent(
  input: MedicationEventCorrectionInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return medicationMutation("CorrectMedicationEvent", input, root);
}

export function deleteMedication(
  medicationId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return medicationMutation(
    "DeleteMedication",
    { medicationId, confirmation: medicationDeleteConfirmation },
    root,
  );
}

export function deleteMedicationEvent(
  eventId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return medicationMutation(
    "DeleteMedicationEvent",
    { eventId, confirmation: medicationDeleteConfirmation },
    root,
  );
}

export async function exportMedicationData(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<MedicationExport> {
  const method = findWailsMethod(root, ["ExportMedicationData"]);
  if (!method) throw new Error("Medication export requires the ZeitBoard desktop service.");
  const value = await method();
  if (!isRecord(value)) throw new Error("Medication export returned an invalid response.");
  const fileName = text(value.fileName);
  const json = text(value.json);
  const generatedAt = timestamp(value.generatedAt);
  const generatedLabel = text(value.generatedLabel);
  const medicationCount = nonNegativeInteger(value.medicationCount);
  const eventCount = nonNegativeInteger(value.eventCount);
  if (
    !fileName ||
    !fileName.toLowerCase().endsWith(".json") ||
    !json ||
    !generatedAt ||
    !generatedLabel ||
    medicationCount === undefined ||
    eventCount === undefined
  ) {
    throw new Error("Medication export returned an invalid response.");
  }
  try {
    const parsed: unknown = JSON.parse(json);
    if (
      !isRecord(parsed) ||
      parsed.schema_version !== "v1" ||
      parsed.generated_at !== generatedAt
    ) {
      throw new Error();
    }
    const medicationSet = parsed.medication_set;
    const eventSet = parsed.event_set;
    if (
      !isRecord(medicationSet) ||
      medicationSet.schema_version !== "v1" ||
      medicationSet.generated_at !== generatedAt ||
      !Array.isArray(medicationSet.medications) ||
      medicationSet.medications.length !== medicationCount ||
      !isRecord(eventSet) ||
      eventSet.schema_version !== "v1" ||
      eventSet.generated_at !== generatedAt ||
      !Array.isArray(eventSet.events) ||
      eventSet.events.length !== eventCount ||
      !Array.isArray(eventSet.corrections)
    ) {
      throw new Error();
    }
  } catch {
    throw new Error("Medication export JSON did not match its declared counts.");
  }
  return { fileName, json, generatedAt, generatedLabel, medicationCount, eventCount };
}

export function downloadMedicationExport(value: MedicationExport): boolean {
  if (
    (typeof navigator !== "undefined" && navigator.userAgent.toLowerCase().includes("jsdom")) ||
    typeof Blob === "undefined" ||
    typeof URL === "undefined" ||
    typeof URL.createObjectURL !== "function" ||
    typeof document === "undefined"
  ) {
    return false;
  }
  const blob = new Blob([value.json], { type: "application/json;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = value.fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
  return true;
}

export function notifyMedicationDataChanged() {
  if (typeof window !== "undefined") window.dispatchEvent(new Event(medicationDataChangedEvent));
}

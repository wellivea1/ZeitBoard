import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export const medicationDataChangedEvent = "zeitboard:medication-data-changed";
export const medicationDeleteConfirmation = "DELETE";

export type MedicationEventStatus = "taken" | "skipped";
export type MedicationEstimateStatus = "estimated" | "empty" | "refused" | "unavailable";
export type MedicationSleepRelationKind = "observed" | "predicted" | "unavailable";
export type MedicationScheduleKind = "as_needed" | "fixed_clock" | "cycling";
export type MedicationForecastStatus =
  | "not_applicable"
  | "unavailable"
  | "no_overlap"
  | "collision";
export type MedicationOccurrenceStatus =
  | "inside_predicted_sleep"
  | "outside_predicted_sleep"
  | "outside_forecast";
export type MedicationReminderStatus = "disabled" | "ready" | "error" | "unavailable";

export interface MedicationScheduleOccurrence {
  at: string;
  civilDate: string;
  civilTime: string;
  civilLabel: string;
  status: MedicationOccurrenceStatus;
  context: string;
  confidence: "High" | "Medium" | "Low" | "Unknown";
  ambiguous: boolean;
  dstNote?: string;
}

export interface MedicationScheduleGap {
  civilDate: string;
  civilTime: string;
  civilLabel: string;
  message: string;
}

export interface MedicationScheduleForecast {
  status: MedicationForecastStatus;
  message: string;
  coveredCount: number;
  collisionCount: number;
  outsideHorizonCount: number;
  coverageEndsAt?: string;
  coverageLabel?: string;
  occurrences: MedicationScheduleOccurrence[];
  gaps: MedicationScheduleGap[];
}

export interface MedicationSchedule {
  kind: MedicationScheduleKind;
  zoneId?: string;
  civilTimes: string[];
  daysOn?: number;
  daysOff?: number;
  cycleStartedOn?: string;
  reminderEnabled: boolean;
  summary: string;
  forecast: MedicationScheduleForecast;
}

export interface MedicationDefinition {
  medicationId: string;
  label: string;
  form?: string;
  strengthLabel?: string;
  detailLabel: string;
  clinicianRule?: string;
  clinicianRuleAttribution?: string;
  active: boolean;
  revision: number;
  scheduleKind: "none" | MedicationScheduleKind;
  schedule?: MedicationSchedule;
  createdLabel: string;
  eventCount: number;
  startedAt?: string;
  startedLocal?: string;
  startedZoneId?: string;
  startedLabel?: string;
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
  reminderStatus: MedicationReminderStatus;
  reminderMessage: string;
  updatedLabel: string;
}

export interface MedicationInput {
  label: string;
  form: string;
  strengthLabel: string;
  startedLocal?: string;
  startedZoneId?: string;
}

export interface MedicationUpdateInput extends MedicationInput {
  medicationId: string;
  revision: number;
  active: boolean;
}

export interface MedicationScheduleInput {
  medicationId: string;
  revision: number;
  kind: "none" | MedicationScheduleKind;
  zoneId: string;
  civilTimes: string[];
  daysOn: number;
  daysOff: number;
  cycleStartedOn: string;
  reminderEnabled: boolean;
  clinicianRule: string;
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
const civilDatePattern = /^\d{4}-\d{2}-\d{2}$/;
const civilClockPattern = /^(?:[01]\d|2[0-3]):[0-5]\d$/;
const zonePattern = /^(?:UTC|[A-Za-z0-9._+-]+(?:\/[A-Za-z0-9._+-]+)+)$/;

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
  reminderStatus: "unavailable",
  reminderMessage: "Desktop reminders require the running ZeitBoard desktop service.",
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

function civilDate(value: unknown): string | undefined {
  const candidate = text(value);
  if (!candidate || !civilDatePattern.test(candidate)) return undefined;
  const parsed = new Date(`${candidate}T00:00:00Z`);
  return !Number.isNaN(parsed.valueOf()) && parsed.toISOString().slice(0, 10) === candidate
    ? candidate
    : undefined;
}

function civilClock(value: unknown): string | undefined {
  const candidate = text(value);
  return candidate && civilClockPattern.test(candidate) ? candidate : undefined;
}

function zoneId(value: unknown): string | undefined {
  const candidate = text(value);
  return candidate && zonePattern.test(candidate) ? candidate : undefined;
}

function normalizeScheduleOccurrence(value: unknown): MedicationScheduleOccurrence | undefined {
  if (!isRecord(value)) return undefined;
  const at = timestamp(value.at);
  const date = civilDate(value.civilDate);
  const clock = civilClock(value.civilTime);
  const civilLabel = text(value.civilLabel);
  const status =
    value.status === "inside_predicted_sleep" ||
    value.status === "outside_predicted_sleep" ||
    value.status === "outside_forecast"
      ? value.status
      : undefined;
  const context = text(value.context);
  const confidence =
    value.confidence === "High" ||
    value.confidence === "Medium" ||
    value.confidence === "Low" ||
    value.confidence === "Unknown"
      ? value.confidence
      : undefined;
  const dstNote = optionalText(value.dstNote);
  const expectedContext =
    status === "inside_predicted_sleep"
      ? "Inside a current predicted sleep window"
      : status === "outside_predicted_sleep"
        ? "Not inside a current predicted sleep window"
        : status === "outside_forecast"
          ? "Outside the current forecast horizon"
          : undefined;
  if (
    !at ||
    !date ||
    !clock ||
    !civilLabel ||
    !status ||
    !context ||
    context !== expectedContext ||
    !confidence ||
    typeof value.ambiguous !== "boolean" ||
    (status === "outside_forecast" && confidence !== "Unknown") ||
    value.ambiguous !== Boolean(dstNote)
  ) {
    return undefined;
  }
  return {
    at,
    civilDate: date,
    civilTime: clock,
    civilLabel,
    status,
    context,
    confidence,
    ambiguous: value.ambiguous,
    ...(dstNote ? { dstNote } : {}),
  };
}

function normalizeScheduleGap(value: unknown): MedicationScheduleGap | undefined {
  if (!isRecord(value)) return undefined;
  const date = civilDate(value.civilDate);
  const clock = civilClock(value.civilTime);
  const civilLabel = text(value.civilLabel);
  const message = text(value.message);
  if (!date || !clock || !civilLabel || !message) return undefined;
  return { civilDate: date, civilTime: clock, civilLabel, message };
}

function normalizeScheduleForecast(value: unknown): MedicationScheduleForecast | undefined {
  if (!isRecord(value) || !Array.isArray(value.occurrences) || !Array.isArray(value.gaps)) {
    return undefined;
  }
  const status =
    value.status === "not_applicable" ||
    value.status === "unavailable" ||
    value.status === "no_overlap" ||
    value.status === "collision"
      ? value.status
      : undefined;
  const message = text(value.message);
  const coveredCount = nonNegativeInteger(value.coveredCount);
  const collisionCount = nonNegativeInteger(value.collisionCount);
  const outsideHorizonCount = nonNegativeInteger(value.outsideHorizonCount);
  const coverageEndsAt =
    value.coverageEndsAt === undefined ? undefined : timestamp(value.coverageEndsAt);
  const coverageLabel = optionalText(value.coverageLabel);
  if (
    !status ||
    !message ||
    coveredCount === undefined ||
    collisionCount === undefined ||
    outsideHorizonCount === undefined ||
    (value.coverageEndsAt !== undefined && !coverageEndsAt) ||
    Boolean(coverageEndsAt) !== Boolean(coverageLabel)
  ) {
    return undefined;
  }
  const occurrences: MedicationScheduleOccurrence[] = [];
  let previousAt = -Infinity;
  const seen = new Set<string>();
  for (const item of value.occurrences) {
    const occurrence = normalizeScheduleOccurrence(item);
    if (!occurrence || seen.has(occurrence.at)) return undefined;
    const atValue = Date.parse(occurrence.at);
    if (atValue < previousAt) return undefined;
    previousAt = atValue;
    seen.add(occurrence.at);
    occurrences.push(occurrence);
  }
  const gaps: MedicationScheduleGap[] = [];
  for (const item of value.gaps) {
    const gap = normalizeScheduleGap(item);
    if (!gap) return undefined;
    gaps.push(gap);
  }
  const inside = occurrences.filter((item) => item.status === "inside_predicted_sleep").length;
  const outside = occurrences.filter((item) => item.status === "outside_predicted_sleep").length;
  const unknown = occurrences.filter((item) => item.status === "outside_forecast").length;
  if (
    coveredCount !== inside + outside ||
    collisionCount !== inside ||
    outsideHorizonCount !== unknown ||
    (status === "collision" && (inside === 0 || coveredCount === 0 || !coverageEndsAt)) ||
    (status === "no_overlap" && (inside !== 0 || coveredCount === 0 || !coverageEndsAt)) ||
    (status === "not_applicable" &&
      (occurrences.length !== 0 || gaps.length !== 0 || coveredCount !== 0 || coverageEndsAt)) ||
    (status === "unavailable" && coveredCount !== 0)
  ) {
    return undefined;
  }
  return {
    status,
    message,
    coveredCount,
    collisionCount,
    outsideHorizonCount,
    ...(coverageEndsAt && coverageLabel ? { coverageEndsAt, coverageLabel } : {}),
    occurrences,
    gaps,
  };
}

function normalizeSchedule(value: unknown): MedicationSchedule | undefined {
  if (!isRecord(value) || !Array.isArray(value.civilTimes)) return undefined;
  const kind =
    value.kind === "as_needed" || value.kind === "fixed_clock" || value.kind === "cycling"
      ? value.kind
      : undefined;
  const scheduleZone = value.zoneId === undefined ? undefined : zoneId(value.zoneId);
  const civilTimes = value.civilTimes.map(civilClock);
  const summary = text(value.summary);
  const forecast = normalizeScheduleForecast(value.forecast);
  const daysOn = value.daysOn === undefined ? undefined : positiveInteger(value.daysOn);
  const daysOff = value.daysOff === undefined ? undefined : positiveInteger(value.daysOff);
  const cycleStartedOn =
    value.cycleStartedOn === undefined ? undefined : civilDate(value.cycleStartedOn);
  if (
    !kind ||
    civilTimes.some((item) => !item) ||
    new Set(civilTimes).size !== civilTimes.length ||
    civilTimes.some((item, index) => index > 0 && item! < civilTimes[index - 1]!) ||
    !summary ||
    !forecast ||
    typeof value.reminderEnabled !== "boolean" ||
    (value.zoneId !== undefined && !scheduleZone) ||
    (value.daysOn !== undefined && !daysOn) ||
    (value.daysOff !== undefined && !daysOff) ||
    (value.cycleStartedOn !== undefined && !cycleStartedOn)
  ) {
    return undefined;
  }
  if (
    (kind === "as_needed" &&
      (scheduleZone ||
        civilTimes.length !== 0 ||
        daysOn ||
        daysOff ||
        cycleStartedOn ||
        value.reminderEnabled ||
        forecast.status !== "not_applicable")) ||
    (kind === "fixed_clock" &&
      (!scheduleZone ||
        civilTimes.length < 1 ||
        civilTimes.length > 8 ||
        daysOn ||
        daysOff ||
        cycleStartedOn)) ||
    (kind === "cycling" &&
      (!scheduleZone ||
        civilTimes.length < 1 ||
        civilTimes.length > 8 ||
        !daysOn ||
        !daysOff ||
        !cycleStartedOn ||
        daysOn > 365 ||
        daysOff > 365))
  ) {
    return undefined;
  }
  return {
    kind,
    ...(scheduleZone ? { zoneId: scheduleZone } : {}),
    civilTimes: civilTimes as string[],
    ...(daysOn ? { daysOn } : {}),
    ...(daysOff ? { daysOff } : {}),
    ...(cycleStartedOn ? { cycleStartedOn } : {}),
    reminderEnabled: value.reminderEnabled,
    summary,
    forecast,
  };
}

function normalizeMedication(value: unknown): MedicationDefinition | undefined {
  if (!isRecord(value)) return undefined;
  const medicationId = identifier(value.medicationId);
  const label = text(value.label);
  const form = optionalText(value.form);
  const strengthLabel = optionalText(value.strengthLabel);
  const detailLabel = text(value.detailLabel);
  const clinicianRule = optionalText(value.clinicianRule);
  const clinicianRuleAttribution = optionalText(value.clinicianRuleAttribution);
  const revision = positiveInteger(value.revision);
  const scheduleKind =
    value.scheduleKind === "none" ||
    value.scheduleKind === "as_needed" ||
    value.scheduleKind === "fixed_clock" ||
    value.scheduleKind === "cycling"
      ? value.scheduleKind
      : undefined;
  const schedule = value.schedule === undefined ? undefined : normalizeSchedule(value.schedule);
  const createdLabel = text(value.createdLabel);
  const eventCount = nonNegativeInteger(value.eventCount);
  const startedAt = value.startedAt === undefined ? undefined : timestamp(value.startedAt);
  const startedLocal =
    value.startedLocal === undefined ? undefined : localDateTime(value.startedLocal);
  const startedZoneId = value.startedZoneId === undefined ? undefined : zoneId(value.startedZoneId);
  const startedLabel = value.startedLabel === undefined ? undefined : text(value.startedLabel);
  const hasStartFields = [
    value.startedAt,
    value.startedLocal,
    value.startedZoneId,
    value.startedLabel,
  ].some((item) => item !== undefined);
  if (
    !medicationId ||
    !label ||
    !detailLabel ||
    typeof value.active !== "boolean" ||
    revision === undefined ||
    !scheduleKind ||
    (value.schedule !== undefined && !schedule) ||
    (scheduleKind === "none" && schedule !== undefined) ||
    (scheduleKind !== "none" && schedule?.kind !== scheduleKind) ||
    Boolean(clinicianRule) !== Boolean(clinicianRuleAttribution) ||
    (clinicianRule?.length ?? 0) > 500 ||
    !createdLabel ||
    eventCount === undefined ||
    (hasStartFields && !(startedAt && startedLocal && startedZoneId && startedLabel))
  ) {
    return undefined;
  }
  return {
    medicationId,
    label,
    ...(form ? { form } : {}),
    ...(strengthLabel ? { strengthLabel } : {}),
    detailLabel,
    ...(clinicianRule && clinicianRuleAttribution
      ? { clinicianRule, clinicianRuleAttribution }
      : {}),
    active: value.active,
    revision,
    scheduleKind,
    ...(schedule ? { schedule } : {}),
    createdLabel,
    eventCount,
    ...(startedAt && startedLocal && startedZoneId && startedLabel
      ? { startedAt, startedLocal, startedZoneId, startedLabel }
      : {}),
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
  const reminderStatus =
    value.reminderStatus === "disabled" ||
    value.reminderStatus === "ready" ||
    value.reminderStatus === "error" ||
    value.reminderStatus === "unavailable"
      ? value.reminderStatus
      : undefined;
  const reminderMessage = text(value.reminderMessage);
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
    !reminderStatus ||
    !reminderMessage ||
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
    (medications.length === 0 && events.length > 0) ||
    (status !== "unavailable" &&
      medications.some(
        (medication) => medication.active && medication.schedule?.reminderEnabled === true,
      ) !==
        (reminderStatus !== "disabled"))
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
    reminderStatus,
    reminderMessage,
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

export function updateMedicationSchedule(
  input: MedicationScheduleInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return medicationMutation("UpdateMedicationSchedule", input, root);
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
      parsed.schema_version !== "v2" ||
      parsed.generated_at !== generatedAt
    ) {
      throw new Error();
    }
    const medicationSet = parsed.medication_set;
    const eventSet = parsed.event_set;
    if (
      !isRecord(medicationSet) ||
      medicationSet.schema_version !== "v2" ||
      medicationSet.generated_at !== generatedAt ||
      !Array.isArray(medicationSet.medications) ||
      medicationSet.medications.length !== medicationCount ||
      !isRecord(eventSet) ||
      eventSet.schema_version !== "v2" ||
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

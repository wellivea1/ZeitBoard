import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export const medicationReportExportConfirmation = "EXPORT";

export interface MedicationClinicalReportInput {
  rangeMode: "custom" | "all";
  fromDate: string;
  toDate: string;
  zoneId: string;
  dayStartHour: 12 | 18;
  includeForecast: boolean;
  includeMedication: boolean;
  includeMedicationLabels: boolean;
  includeMedicationNotes: boolean;
  includeRhythmContext: boolean;
  includeRhythmContextNotes: boolean;
}

export type MedicationClinicalSleepKind =
  | "sleep_observed"
  | "sleep_inferred"
  | "sleep_nap"
  | "forecast";

export type MedicationClinicalAnnotationKind =
  | "medication_taken"
  | "medication_skipped"
  | "medication_start"
  | "context_travel"
  | "context_illness"
  | "context_disruption"
  | "context_forced_schedule";

export interface MedicationClinicalSleepSegment {
  kind: MedicationClinicalSleepKind;
  startPercent: number;
  widthPercent: number;
  startLabel: string;
  wakeLabel: string;
  durationLabel: string;
  source: string;
  confidence: string;
}

export interface MedicationClinicalAnnotation {
  kind: MedicationClinicalAnnotationKind;
  positionPercent: number;
  label: string;
  atLabel: string;
  detail?: string;
}

export interface MedicationClinicalActogramRow {
  civilDate: string;
  dayLabel: string;
  monthLabel?: string;
  weekend: boolean;
  noData: boolean;
  sleep: MedicationClinicalSleepSegment[];
  annotations: MedicationClinicalAnnotation[];
}

export interface MedicationClinicalLegend {
  kind: MedicationClinicalSleepKind | MedicationClinicalAnnotationKind;
  label: string;
}

export interface MedicationClinicalDriftPoint {
  id: string;
  day: string;
  civilDate: string;
  onsetHour: number;
  fitHour: number;
  bandLowHour: number;
  bandHighHour: number;
  onsetLabel: string;
  source: string;
  confidence: string;
}

export interface MedicationClinicalReport {
  status: "ready" | "partial" | "insufficient";
  message: string;
  generatedAt: string;
  generatedLabel: string;
  range: {
    mode: "custom" | "all";
    fromDate: string;
    toDate: string;
    label: string;
    dayStartHour: 12 | 18;
    dayStartLabel: string;
  };
  summary: {
    calendarRows: number;
    observedSleepSegments: number;
    noDataRows: number;
    medicationEvents: number;
    recordedScheduled: number;
    recordedTaken: number;
    recordedSkipped: number;
    excludedEvents: number;
    rhythmContextMarkers: number;
  };
  redactions: string[];
  actogram: {
    axisLabels: string[];
    rows: MedicationClinicalActogramRow[];
    legend: MedicationClinicalLegend[];
    summary: string;
  };
  drift: {
    status: string;
    slopeLabel: string;
    confidence: string;
    summary: string;
    yMinHour: number;
    yMaxHour: number;
    points: MedicationClinicalDriftPoint[];
  };
  adherence: Array<{
    medicationLabel: string;
    recordedScheduled: number;
    taken: number;
    skipped: number;
    asNeeded: number;
    summary: string;
  }>;
  events: Array<{
    medicationLabel: string;
    civilTime: string;
    status: "taken" | "skipped";
    scheduleContext: "Recorded scheduled event" | "As-needed / not marked scheduled";
    wakeContext: string;
    sleepContext: string;
    confidence: string;
    note?: string;
  }>;
  associations: Array<{
    medicationLabel: string;
    startedLabel: string;
    status: string;
    message: string;
    before: MedicationClinicalAssociationSegment;
    after: MedicationClinicalAssociationSegment;
    context: Array<{
      kindLabel: string;
      rangeLabel: string;
      timingLabel: string;
      note?: string;
    }>;
  }>;
  provenance: string[];
  notice: string;
}

export interface MedicationClinicalAssociationSegment {
  episodeCount: number;
  rangeLabel: string;
  slopeLabel: string;
  confidence: string;
}

export interface MedicationClinicalReportExport {
  fileName: string;
  html: string;
  generatedAt: string;
  generatedLabel: string;
  rowCount: number;
  eventCount: number;
  redactions: string[];
}

type UnknownRecord = Record<string, unknown>;

const datePattern = /^\d{4}-\d{2}-\d{2}$/;
const timestampPattern = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;
const exportFilePattern =
  /^zeitboard-clinician-report-\d{4}-\d{2}-\d{2}-to-\d{4}-\d{2}-\d{2}\.html$/;
const sleepKinds = new Set<MedicationClinicalSleepKind>([
  "sleep_observed",
  "sleep_inferred",
  "sleep_nap",
  "forecast",
]);
const annotationKinds = new Set<MedicationClinicalAnnotationKind>([
  "medication_taken",
  "medication_skipped",
  "medication_start",
  "context_travel",
  "context_illness",
  "context_disruption",
  "context_forced_schedule",
]);

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

function integer(value: unknown): number | undefined {
  return typeof value === "number" && Number.isInteger(value) && value >= 0 ? value : undefined;
}

function finite(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function date(value: unknown): string | undefined {
  const candidate = text(value);
  if (!candidate || !datePattern.test(candidate)) return undefined;
  const parsed = new Date(`${candidate}T00:00:00Z`);
  return !Number.isNaN(parsed.valueOf()) && parsed.toISOString().slice(0, 10) === candidate
    ? candidate
    : undefined;
}

function timestamp(value: unknown): string | undefined {
  const candidate = text(value);
  return candidate && timestampPattern.test(candidate) && !Number.isNaN(Date.parse(candidate))
    ? candidate
    : undefined;
}

function stringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const values = value.map(text);
  return values.every(Boolean) ? (values as string[]) : undefined;
}

function normalizeSleep(value: unknown): MedicationClinicalSleepSegment | undefined {
  if (!isRecord(value) || !sleepKinds.has(value.kind as MedicationClinicalSleepKind)) {
    return undefined;
  }
  const startPercent = finite(value.startPercent);
  const widthPercent = finite(value.widthPercent);
  const startLabel = text(value.startLabel);
  const wakeLabel = text(value.wakeLabel);
  const durationLabel = text(value.durationLabel);
  const source = text(value.source);
  const confidence = text(value.confidence);
  if (
    startPercent === undefined ||
    startPercent < 0 ||
    startPercent > 100 ||
    widthPercent === undefined ||
    widthPercent <= 0 ||
    widthPercent > 100 ||
    startPercent + widthPercent > 100.01 ||
    !startLabel ||
    !wakeLabel ||
    !durationLabel ||
    !source ||
    !confidence
  ) {
    return undefined;
  }
  return {
    kind: value.kind as MedicationClinicalSleepKind,
    startPercent,
    widthPercent,
    startLabel,
    wakeLabel,
    durationLabel,
    source,
    confidence,
  };
}

function normalizeAnnotation(value: unknown): MedicationClinicalAnnotation | undefined {
  if (!isRecord(value) || !annotationKinds.has(value.kind as MedicationClinicalAnnotationKind)) {
    return undefined;
  }
  const positionPercent = finite(value.positionPercent);
  const label = text(value.label);
  const atLabel = text(value.atLabel);
  const detail = optionalText(value.detail);
  if (
    positionPercent === undefined ||
    positionPercent < 0 ||
    positionPercent > 100 ||
    !label ||
    !atLabel
  ) {
    return undefined;
  }
  return {
    kind: value.kind as MedicationClinicalAnnotationKind,
    positionPercent,
    label,
    atLabel,
    ...(detail ? { detail } : {}),
  };
}

function normalizeRow(value: unknown): MedicationClinicalActogramRow | undefined {
  if (!isRecord(value) || !Array.isArray(value.sleep) || !Array.isArray(value.annotations)) {
    return undefined;
  }
  const civilDate = date(value.civilDate);
  const dayLabel = text(value.dayLabel);
  const monthLabel = optionalText(value.monthLabel);
  const sleep = value.sleep.map(normalizeSleep);
  const annotations = value.annotations.map(normalizeAnnotation);
  if (
    !civilDate ||
    !dayLabel ||
    typeof value.weekend !== "boolean" ||
    typeof value.noData !== "boolean" ||
    sleep.some((item) => !item) ||
    annotations.some((item) => !item) ||
    value.noData !== sleep.every((item) => item?.kind === "forecast")
  ) {
    return undefined;
  }
  return {
    civilDate,
    dayLabel,
    ...(monthLabel ? { monthLabel } : {}),
    weekend: value.weekend,
    noData: value.noData,
    sleep: sleep as MedicationClinicalSleepSegment[],
    annotations: annotations as MedicationClinicalAnnotation[],
  };
}

function normalizeDriftPoint(value: unknown): MedicationClinicalDriftPoint | undefined {
  if (!isRecord(value)) return undefined;
  const id = text(value.id);
  const day = text(value.day);
  const civilDate = date(value.civilDate);
  const onsetHour = finite(value.onsetHour);
  const fitHour = finite(value.fitHour);
  const bandLowHour = finite(value.bandLowHour);
  const bandHighHour = finite(value.bandHighHour);
  const onsetLabel = text(value.onsetLabel);
  const source = text(value.source);
  const confidence = text(value.confidence);
  if (
    !id ||
    !day ||
    !civilDate ||
    onsetHour === undefined ||
    fitHour === undefined ||
    bandLowHour === undefined ||
    bandHighHour === undefined ||
    !onsetLabel ||
    !source ||
    !confidence
  ) {
    return undefined;
  }
  return {
    id,
    day,
    civilDate,
    onsetHour,
    fitHour,
    bandLowHour,
    bandHighHour,
    onsetLabel,
    source,
    confidence,
  };
}

function normalizeAssociationSegment(
  value: unknown,
): MedicationClinicalAssociationSegment | undefined {
  if (!isRecord(value)) return undefined;
  const episodeCount = integer(value.episodeCount);
  const rangeLabel = text(value.rangeLabel);
  const slopeLabel = text(value.slopeLabel);
  const confidence = text(value.confidence);
  return episodeCount !== undefined && rangeLabel && slopeLabel && confidence
    ? { episodeCount, rangeLabel, slopeLabel, confidence }
    : undefined;
}

export function normalizeMedicationClinicalReport(
  value: unknown,
): MedicationClinicalReport | undefined {
  if (!isRecord(value)) return undefined;
  const status =
    value.status === "ready" || value.status === "partial" || value.status === "insufficient"
      ? value.status
      : undefined;
  const message = text(value.message);
  const generatedAt = timestamp(value.generatedAt);
  const generatedLabel = text(value.generatedLabel);
  const redactions = stringArray(value.redactions);
  const provenance = stringArray(value.provenance);
  const notice = text(value.notice);
  if (
    !status ||
    !message ||
    !generatedAt ||
    !generatedLabel ||
    !redactions ||
    redactions.length < 2 ||
    !provenance ||
    !notice ||
    !isRecord(value.range) ||
    !isRecord(value.summary) ||
    !isRecord(value.actogram) ||
    !isRecord(value.drift) ||
    !Array.isArray(value.adherence) ||
    !Array.isArray(value.events) ||
    !Array.isArray(value.associations)
  ) {
    return undefined;
  }
  const rangeMode =
    value.range.mode === "custom" || value.range.mode === "all" ? value.range.mode : undefined;
  const fromDate = date(value.range.fromDate);
  const toDate = date(value.range.toDate);
  const rangeLabel = text(value.range.label);
  const dayStartHour =
    value.range.dayStartHour === 12 || value.range.dayStartHour === 18
      ? value.range.dayStartHour
      : undefined;
  const dayStartLabel = text(value.range.dayStartLabel);
  const summary = value.summary;
  const calendarRows = integer(summary.calendarRows);
  const observedSleepSegments = integer(summary.observedSleepSegments);
  const noDataRows = integer(summary.noDataRows);
  const medicationEvents = integer(summary.medicationEvents);
  const recordedScheduled = integer(summary.recordedScheduled);
  const recordedTaken = integer(summary.recordedTaken);
  const recordedSkipped = integer(summary.recordedSkipped);
  const excludedEvents = integer(summary.excludedEvents);
  const rhythmContextMarkers = integer(summary.rhythmContextMarkers);
  if (
    !rangeMode ||
    !fromDate ||
    !toDate ||
    !rangeLabel ||
    !dayStartHour ||
    !dayStartLabel ||
    calendarRows === undefined ||
    observedSleepSegments === undefined ||
    noDataRows === undefined ||
    medicationEvents === undefined ||
    recordedScheduled === undefined ||
    recordedTaken === undefined ||
    recordedSkipped === undefined ||
    excludedEvents === undefined ||
    rhythmContextMarkers === undefined ||
    !Array.isArray(value.actogram.axisLabels) ||
    !Array.isArray(value.actogram.rows) ||
    !Array.isArray(value.actogram.legend)
  ) {
    return undefined;
  }
  const axisLabels = stringArray(value.actogram.axisLabels);
  const rows = value.actogram.rows.map(normalizeRow);
  const legend = value.actogram.legend.map((item): MedicationClinicalLegend | undefined => {
    if (!isRecord(item)) return undefined;
    const kind = item.kind;
    const label = text(item.label);
    return label &&
      (sleepKinds.has(kind as MedicationClinicalSleepKind) ||
        annotationKinds.has(kind as MedicationClinicalAnnotationKind))
      ? { kind: kind as MedicationClinicalLegend["kind"], label }
      : undefined;
  });
  const actogramSummary = text(value.actogram.summary);
  const driftStatus = text(value.drift.status);
  const driftSlope = text(value.drift.slopeLabel);
  const driftConfidence = text(value.drift.confidence);
  const driftSummary = text(value.drift.summary);
  const yMinHour = finite(value.drift.yMinHour);
  const yMaxHour = finite(value.drift.yMaxHour);
  const points = Array.isArray(value.drift.points)
    ? value.drift.points.map(normalizeDriftPoint)
    : [];
  if (
    !axisLabels ||
    axisLabels.length !== 5 ||
    rows.some((item) => !item) ||
    legend.some((item) => !item) ||
    !actogramSummary ||
    !driftStatus ||
    !driftSlope ||
    !driftConfidence ||
    !driftSummary ||
    yMinHour === undefined ||
    yMaxHour === undefined ||
    points.some((item) => !item) ||
    (points.length > 0 && yMaxHour <= yMinHour)
  ) {
    return undefined;
  }
  const adherence = value.adherence.map((item) => {
    if (!isRecord(item)) return undefined;
    const medicationLabel = text(item.medicationLabel);
    const recordedScheduled = integer(item.recordedScheduled);
    const taken = integer(item.taken);
    const skipped = integer(item.skipped);
    const asNeeded = integer(item.asNeeded);
    const summary = text(item.summary);
    return medicationLabel &&
      recordedScheduled !== undefined &&
      taken !== undefined &&
      skipped !== undefined &&
      asNeeded !== undefined &&
      summary
      ? { medicationLabel, recordedScheduled, taken, skipped, asNeeded, summary }
      : undefined;
  });
  const events = value.events.map((item) => {
    if (!isRecord(item)) return undefined;
    const medicationLabel = text(item.medicationLabel);
    const civilTime = text(item.civilTime);
    const status = item.status === "taken" || item.status === "skipped" ? item.status : undefined;
    const scheduleContext =
      item.scheduleContext === "Recorded scheduled event" ||
      item.scheduleContext === "As-needed / not marked scheduled"
        ? item.scheduleContext
        : undefined;
    const wakeContext = text(item.wakeContext);
    const sleepContext = text(item.sleepContext);
    const confidence = text(item.confidence);
    const note = optionalText(item.note);
    return medicationLabel &&
      civilTime &&
      status &&
      scheduleContext &&
      wakeContext &&
      sleepContext &&
      confidence
      ? {
          medicationLabel,
          civilTime,
          status,
          scheduleContext,
          wakeContext,
          sleepContext,
          confidence,
          ...(note ? { note } : {}),
        }
      : undefined;
  });
  const associations = value.associations.map((item) => {
    if (!isRecord(item) || !Array.isArray(item.context)) return undefined;
    const medicationLabel = text(item.medicationLabel);
    const startedLabel = text(item.startedLabel);
    const associationStatus = text(item.status);
    const associationMessage = text(item.message);
    const before = normalizeAssociationSegment(item.before);
    const after = normalizeAssociationSegment(item.after);
    const context = item.context.map((entry) => {
      if (!isRecord(entry)) return undefined;
      const kindLabel = text(entry.kindLabel);
      const entryRange = text(entry.rangeLabel);
      const timingLabel = text(entry.timingLabel);
      const note = optionalText(entry.note);
      return kindLabel && entryRange && timingLabel
        ? { kindLabel, rangeLabel: entryRange, timingLabel, ...(note ? { note } : {}) }
        : undefined;
    });
    return medicationLabel &&
      startedLabel &&
      associationStatus &&
      associationMessage &&
      before &&
      after &&
      context.every(Boolean)
      ? {
          medicationLabel,
          startedLabel,
          status: associationStatus,
          message: associationMessage,
          before,
          after,
          context: context as NonNullable<(typeof context)[number]>[],
        }
      : undefined;
  });
  if (
    rows.length !== calendarRows ||
    events.length !== medicationEvents ||
    fromDate > toDate ||
    adherence.some((item) => !item) ||
    events.some((item) => !item) ||
    associations.some((item) => !item)
  ) {
    return undefined;
  }
  const normalizedRows = rows as MedicationClinicalActogramRow[];
  const normalizedLegend = legend as MedicationClinicalLegend[];
  const normalizedAdherence = adherence as MedicationClinicalReport["adherence"];
  const normalizedEvents = events as MedicationClinicalReport["events"];
  const normalizedAssociations = associations as MedicationClinicalReport["associations"];
  const normalizedPoints = points as MedicationClinicalDriftPoint[];
  const fromTime = Date.parse(`${fromDate}T00:00:00Z`);
  const expectedCalendarRows = (Date.parse(`${toDate}T00:00:00Z`) - fromTime) / 86_400_000 + 1;
  const presentKinds = new Set<MedicationClinicalLegend["kind"]>();
  let countedSleepSegments = 0;
  let countedNoDataRows = 0;
  let countedMedicationEvents = 0;
  let countedContextMarkers = 0;
  for (const row of normalizedRows) {
    if (row.noData) countedNoDataRows += 1;
    for (const segment of row.sleep) {
      presentKinds.add(segment.kind);
      if (segment.kind !== "forecast") countedSleepSegments += 1;
    }
    for (const annotation of row.annotations) {
      presentKinds.add(annotation.kind);
      if (annotation.kind === "medication_taken" || annotation.kind === "medication_skipped") {
        countedMedicationEvents += 1;
      }
      if (annotation.kind.startsWith("context_")) countedContextMarkers += 1;
    }
  }
  const legendKinds = normalizedLegend.map((item) => item.kind);
  const uniqueLegendKinds = new Set(legendKinds);
  const adherenceScheduled = normalizedAdherence.reduce(
    (total, item) => total + item.recordedScheduled,
    0,
  );
  const adherenceTaken = normalizedAdherence.reduce((total, item) => total + item.taken, 0);
  const adherenceSkipped = normalizedAdherence.reduce((total, item) => total + item.skipped, 0);
  const adherenceAsNeeded = normalizedAdherence.reduce((total, item) => total + item.asNeeded, 0);
  if (
    calendarRows !== expectedCalendarRows ||
    normalizedRows.some(
      (row, index) =>
        row.civilDate !== new Date(fromTime + index * 86_400_000).toISOString().slice(0, 10),
    ) ||
    countedSleepSegments !== observedSleepSegments ||
    countedNoDataRows !== noDataRows ||
    countedMedicationEvents !== medicationEvents ||
    countedContextMarkers !== rhythmContextMarkers ||
    uniqueLegendKinds.size !== legendKinds.length ||
    uniqueLegendKinds.size !== presentKinds.size ||
    [...presentKinds].some((kind) => !uniqueLegendKinds.has(kind)) ||
    adherenceScheduled !== recordedScheduled ||
    adherenceTaken !== recordedTaken ||
    adherenceSkipped !== recordedSkipped ||
    normalizedAdherence.some((item) => item.recordedScheduled !== item.taken + item.skipped) ||
    adherenceScheduled + adherenceAsNeeded !== medicationEvents ||
    normalizedPoints.some(
      (point, index) =>
        point.bandLowHour > point.bandHighHour ||
        point.bandLowHour < yMinHour ||
        point.bandHighHour > yMaxHour ||
        point.onsetHour < yMinHour ||
        point.onsetHour > yMaxHour ||
        point.fitHour < yMinHour ||
        point.fitHour > yMaxHour ||
        (index > 0 && point.civilDate < normalizedPoints[index - 1]!.civilDate),
    ) ||
    new Set(normalizedPoints.map((point) => point.id)).size !== normalizedPoints.length ||
    new Set(redactions).size !== redactions.length ||
    !redactions.includes("Personal diagnostic information omitted") ||
    !redactions.includes("Calendar and location information omitted") ||
    !redactions.includes("Clinician-entered medication guidance omitted")
  ) {
    return undefined;
  }
  return {
    status,
    message,
    generatedAt,
    generatedLabel,
    range: {
      mode: rangeMode,
      fromDate,
      toDate,
      label: rangeLabel,
      dayStartHour,
      dayStartLabel,
    },
    summary: {
      calendarRows,
      observedSleepSegments,
      noDataRows,
      medicationEvents,
      recordedScheduled,
      recordedTaken,
      recordedSkipped,
      excludedEvents,
      rhythmContextMarkers,
    },
    redactions,
    actogram: {
      axisLabels,
      rows: normalizedRows,
      legend: normalizedLegend,
      summary: actogramSummary,
    },
    drift: {
      status: driftStatus,
      slopeLabel: driftSlope,
      confidence: driftConfidence,
      summary: driftSummary,
      yMinHour,
      yMaxHour,
      points: normalizedPoints,
    },
    adherence: normalizedAdherence,
    events: normalizedEvents,
    associations: normalizedAssociations,
    provenance,
    notice,
  };
}

export function hasLocalMedicationReportService(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): boolean {
  return Boolean(
    findWailsMethod(root, ["GetMedicationClinicianReport"]) &&
    findWailsMethod(root, ["ExportMedicationClinicianReport"]),
  );
}

export async function loadMedicationClinicalReport(
  input: MedicationClinicalReportInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<MedicationClinicalReport> {
  const method = findWailsMethod(root, ["GetMedicationClinicianReport"]);
  if (!method) throw new Error("Clinician reports require the ZeitBoard desktop service.");
  const report = normalizeMedicationClinicalReport(await method(input));
  if (!report) throw new Error("Clinician report service returned an invalid response.");
  return report;
}

export async function exportMedicationClinicalReport(
  report: MedicationClinicalReportInput,
  confirmation: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<MedicationClinicalReportExport> {
  const method = findWailsMethod(root, ["ExportMedicationClinicianReport"]);
  if (!method) throw new Error("Clinician reports require the ZeitBoard desktop service.");
  const value = await method({ report, confirmation });
  if (!isRecord(value)) throw new Error("Clinician report export returned an invalid response.");
  const fileName = text(value.fileName);
  const html = text(value.html);
  const generatedAt = timestamp(value.generatedAt);
  const generatedLabel = text(value.generatedLabel);
  const rowCount = integer(value.rowCount);
  const eventCount = integer(value.eventCount);
  const redactions = stringArray(value.redactions);
  if (
    !fileName ||
    !exportFilePattern.test(fileName) ||
    !html ||
    !html.startsWith("<!doctype html>") ||
    /<script\b/i.test(html) ||
    !html.includes("Content-Security-Policy\" content=\"default-src 'none'") ||
    !generatedAt ||
    !generatedLabel ||
    rowCount === undefined ||
    eventCount === undefined ||
    !redactions ||
    !redactions.includes("Personal diagnostic information omitted") ||
    !redactions.includes("Calendar and location information omitted") ||
    !redactions.includes("Clinician-entered medication guidance omitted")
  ) {
    throw new Error("Clinician report export returned an invalid response.");
  }
  return { fileName, html, generatedAt, generatedLabel, rowCount, eventCount, redactions };
}

export function downloadMedicationClinicalReport(value: MedicationClinicalReportExport): boolean {
  if (
    (typeof navigator !== "undefined" && navigator.userAgent.toLowerCase().includes("jsdom")) ||
    typeof Blob === "undefined" ||
    typeof URL === "undefined" ||
    typeof URL.createObjectURL !== "function" ||
    typeof document === "undefined"
  ) {
    return false;
  }
  const blob = new Blob([value.html], { type: "text/html;charset=utf-8" });
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

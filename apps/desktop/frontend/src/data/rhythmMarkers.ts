import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export const rhythmMarkersChangedEvent = "zeitboard:rhythm-markers-changed";
export const rhythmMarkerDeleteConfirmation = "DELETE";

export type RhythmMarkerKind = "travel" | "illness" | "disruption" | "forced_schedule";
export type RhythmMarkerStatus = "ready" | "empty" | "unavailable";

export interface RhythmMarker {
  markerId: string;
  kind: RhythmMarkerKind;
  kindLabel: string;
  startAt: string;
  endAt?: string;
  zoneId: string;
  civilDate: string;
  hour: number;
  startLabel: string;
  endLabel?: string;
  rangeLabel: string;
  note?: string;
  recordedLabel: string;
}

export interface RhythmMarkersData {
  status: RhythmMarkerStatus;
  empty: boolean;
  message: string;
  markers: RhythmMarker[];
  fixtureMode: boolean;
  updatedLabel: string;
}

export interface RhythmMarkerInput {
  kind: RhythmMarkerKind;
  startLocal: string;
  endLocal: string;
  zoneId: string;
  note: string;
}

export interface RhythmMarkerExport {
  fileName: string;
  json: string;
  generatedAt: string;
  generatedLabel: string;
  markerCount: number;
}

type UnknownRecord = Record<string, unknown>;

const identifierPattern = /^[a-z][a-z0-9_-]{2,63}$/;
const timestampPattern = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;
const civilDatePattern = /^\d{4}-\d{2}-\d{2}$/;
const zonePattern = /^(?:UTC|[A-Za-z0-9._+-]+(?:\/[A-Za-z0-9._+-]+)+)$/;

export const rhythmMarkerKindLabels: Record<RhythmMarkerKind, string> = {
  travel: "Travel / time-zone context",
  illness: "Illness / health disruption",
  disruption: "Sleep disruption / awakening",
  forced_schedule: "Forced schedule / obligation",
};

export const unavailableRhythmMarkers: RhythmMarkersData = {
  status: "unavailable",
  empty: true,
  message:
    "Context markers require the ZeitBoard desktop service. Sample markers are not substituted.",
  markers: [],
  fixtureMode: false,
  updatedLabel: "Desktop service unavailable",
};

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasExactKeys(
  value: UnknownRecord,
  required: readonly string[],
  optional: readonly string[] = [],
) {
  const allowed = new Set([...required, ...optional]);
  return (
    required.every((key) => Object.prototype.hasOwnProperty.call(value, key)) &&
    Object.keys(value).every((key) => allowed.has(key))
  );
}

function text(value: unknown, maximum = Number.POSITIVE_INFINITY): string | undefined {
  return typeof value === "string" && value.length > 0 && value.length <= maximum
    ? value
    : undefined;
}

function optionalCanonicalText(value: unknown, maximum: number): string | undefined {
  if (value === undefined) return "";
  return typeof value === "string" &&
    value.length > 0 &&
    value.length <= maximum &&
    value.trim() === value
    ? value
    : undefined;
}

function timestamp(value: unknown): string | undefined {
  if (
    typeof value !== "string" ||
    !timestampPattern.test(value) ||
    Number.isNaN(Date.parse(value))
  ) {
    return undefined;
  }
  return value;
}

function kind(value: unknown): RhythmMarkerKind | undefined {
  return value === "travel" ||
    value === "illness" ||
    value === "disruption" ||
    value === "forced_schedule"
    ? value
    : undefined;
}

function normalizeMarker(value: unknown): RhythmMarker | undefined {
  if (
    !isRecord(value) ||
    !hasExactKeys(
      value,
      [
        "markerId",
        "kind",
        "kindLabel",
        "startAt",
        "zoneId",
        "civilDate",
        "hour",
        "startLabel",
        "rangeLabel",
        "recordedLabel",
      ],
      ["endAt", "endLabel", "note"],
    )
  ) {
    return undefined;
  }
  const markerId = text(value.markerId);
  const markerKind = kind(value.kind);
  const kindLabel = text(value.kindLabel);
  const startAt = timestamp(value.startAt);
  const endAt = value.endAt === undefined ? undefined : timestamp(value.endAt);
  const zoneId = text(value.zoneId, 64);
  const civilDate = text(value.civilDate);
  const hour =
    typeof value.hour === "number" && Number.isFinite(value.hour) ? value.hour : undefined;
  const startLabel = text(value.startLabel);
  const endLabel = value.endLabel === undefined ? undefined : text(value.endLabel);
  const rangeLabel = text(value.rangeLabel);
  const note = optionalCanonicalText(value.note, 500);
  const recordedLabel = text(value.recordedLabel);
  if (
    !markerId ||
    !identifierPattern.test(markerId) ||
    !markerKind ||
    kindLabel !== rhythmMarkerKindLabels[markerKind] ||
    !startAt ||
    (value.endAt !== undefined && !endAt) ||
    !zoneId ||
    !zonePattern.test(zoneId) ||
    !civilDate ||
    !civilDatePattern.test(civilDate) ||
    hour === undefined ||
    hour < 0 ||
    hour >= 24 ||
    !startLabel ||
    !rangeLabel ||
    note === undefined ||
    !recordedLabel ||
    Boolean(endAt) !== Boolean(endLabel) ||
    (endAt !== undefined && Date.parse(endAt) <= Date.parse(startAt))
  ) {
    return undefined;
  }
  return {
    markerId,
    kind: markerKind,
    kindLabel,
    startAt,
    ...(endAt ? { endAt } : {}),
    zoneId,
    civilDate,
    hour,
    startLabel,
    ...(endLabel ? { endLabel } : {}),
    rangeLabel,
    ...(note ? { note } : {}),
    recordedLabel,
  };
}

export function normalizeRhythmMarkers(value: unknown): RhythmMarkersData | undefined {
  if (
    !isRecord(value) ||
    !hasExactKeys(value, [
      "status",
      "empty",
      "message",
      "markers",
      "fixtureMode",
      "updatedLabel",
    ]) ||
    (value.status !== "ready" && value.status !== "empty") ||
    typeof value.empty !== "boolean" ||
    !text(value.message) ||
    !Array.isArray(value.markers) ||
    value.fixtureMode !== false ||
    !text(value.updatedLabel)
  ) {
    return undefined;
  }
  const markers: RhythmMarker[] = [];
  const ids = new Set<string>();
  for (const raw of value.markers) {
    const marker = normalizeMarker(raw);
    if (!marker || ids.has(marker.markerId)) return undefined;
    ids.add(marker.markerId);
    markers.push(marker);
  }
  const isEmpty = markers.length === 0;
  if (value.empty !== isEmpty || value.status !== (isEmpty ? "empty" : "ready")) return undefined;
  return {
    status: value.status,
    empty: value.empty,
    message: value.message as string,
    markers,
    fixtureMode: false,
    updatedLabel: value.updatedLabel as string,
  };
}

export function hasLocalRhythmMarkerService(root: WailsRoot = globalThis as unknown as WailsRoot) {
  return Boolean(findWailsMethod(root, ["GetRhythmMarkers"]));
}

export async function loadRhythmMarkers(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<RhythmMarkersData> {
  const method = findWailsMethod(root, ["GetRhythmMarkers"]);
  if (!method) return unavailableRhythmMarkers;
  const result = normalizeRhythmMarkers(await method());
  if (!result) throw new Error("Rhythm marker service returned an invalid response.");
  return result;
}

async function markerMutation(
  methodName: "AddRhythmMarker" | "DeleteRhythmMarker",
  input: unknown,
  root: WailsRoot,
) {
  const method = findWailsMethod(root, [methodName]);
  if (!method) throw new Error("Rhythm markers require the ZeitBoard desktop service.");
  const result = normalizeRhythmMarkers(await method(input));
  if (!result) throw new Error("Rhythm marker service returned an invalid response.");
  return result;
}

export function addRhythmMarker(
  input: RhythmMarkerInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return markerMutation("AddRhythmMarker", input, root);
}

export function deleteRhythmMarker(
  markerId: string,
  confirmation: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return markerMutation("DeleteRhythmMarker", { markerId, confirmation }, root);
}

function validContractMarker(value: unknown) {
  if (
    !isRecord(value) ||
    !hasExactKeys(
      value,
      ["marker_id", "kind", "start_at", "zone_id", "provenance"],
      ["end_at", "note"],
    )
  ) {
    return false;
  }
  const markerKind = kind(value.kind);
  const startAt = timestamp(value.start_at);
  const endAt = value.end_at === undefined ? undefined : timestamp(value.end_at);
  const note = optionalCanonicalText(value.note, 500);
  if (
    typeof value.marker_id !== "string" ||
    !identifierPattern.test(value.marker_id) ||
    !markerKind ||
    !startAt ||
    (value.end_at !== undefined && !endAt) ||
    typeof value.zone_id !== "string" ||
    !zonePattern.test(value.zone_id) ||
    note === undefined ||
    (endAt !== undefined && Date.parse(endAt) <= Date.parse(startAt)) ||
    !isRecord(value.provenance) ||
    !hasExactKeys(value.provenance, ["acquisition_method", "evidence_status", "recorded_at"]) ||
    value.provenance.acquisition_method !== "manual" ||
    value.provenance.evidence_status !== "user_reported" ||
    !timestamp(value.provenance.recorded_at)
  ) {
    return false;
  }
  return true;
}

export async function exportRhythmMarkers(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<RhythmMarkerExport> {
  const method = findWailsMethod(root, ["ExportRhythmMarkers"]);
  if (!method) throw new Error("Rhythm marker export requires the ZeitBoard desktop service.");
  const value = await method();
  if (
    !isRecord(value) ||
    !hasExactKeys(value, ["fileName", "json", "generatedAt", "generatedLabel", "markerCount"])
  ) {
    throw new Error("Rhythm marker export returned an invalid response.");
  }
  const fileName = text(value.fileName);
  const json = text(value.json);
  const generatedAt = timestamp(value.generatedAt);
  const generatedLabel = text(value.generatedLabel);
  const markerCount =
    typeof value.markerCount === "number" &&
    Number.isInteger(value.markerCount) &&
    value.markerCount >= 0
      ? value.markerCount
      : undefined;
  if (
    !fileName?.toLowerCase().endsWith(".json") ||
    !json ||
    !generatedAt ||
    !generatedLabel ||
    markerCount === undefined
  ) {
    throw new Error("Rhythm marker export returned an invalid response.");
  }
  try {
    const parsed: unknown = JSON.parse(json);
    if (
      !isRecord(parsed) ||
      !hasExactKeys(parsed, ["schema_version", "generated_at", "markers"]) ||
      parsed.schema_version !== "v1" ||
      parsed.generated_at !== generatedAt ||
      !Array.isArray(parsed.markers) ||
      parsed.markers.length !== markerCount ||
      !parsed.markers.every(validContractMarker)
    ) {
      throw new Error();
    }
  } catch {
    throw new Error("Rhythm marker export JSON did not match the v1 contract and declared count.");
  }
  return { fileName, json, generatedAt, generatedLabel, markerCount };
}

export function downloadRhythmMarkerExport(value: RhythmMarkerExport) {
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

export function notifyRhythmMarkersChanged() {
  if (typeof window !== "undefined") window.dispatchEvent(new Event(rhythmMarkersChangedEvent));
}

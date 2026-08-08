import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// The 48-72 hour operational view (ADR-0034). Everything here is computed in
// the shared Go core; this module only validates the shape that crosses the
// bridge and supplies the read-only preview a browser build shows instead.

export type OutlookStatus = "available" | "withheld" | "refused" | "unavailable";

/**
 * Three states, not two. The estimator's sleep and waking envelopes overlap on
 * purpose, and an instant inside both is one where the model genuinely does not
 * know which side of the boundary it is on. Drawing a sharp line there would be
 * a confident claim against a measured P90 onset error of over five hours.
 */
export type Presence = "awake" | "asleep" | "uncertain" | "unknown";

export interface OutlookSegment {
  presence: Presence;
  observed: boolean;
  rangeLabel: string;
  dayLabel: string;
  durationLabel: string;
  offsetHours: number;
  durationHours: number;
}

export interface OutlookDayMark {
  label: string;
  offsetHours: number;
}

/** `partial` means the only overlap sits on an uncertain boundary. */
export type OfficeStatus = "reachable" | "partial" | "unreachable";

export interface OutlookOfficeWindow {
  dayLabel: string;
  hoursLabel: string;
  status: OfficeStatus;
  reachableLabel?: string;
  detail: string;
  offsetHours: number;
  durationHours: number;
}

export interface OutlookCommitment {
  title: string;
  whenLabel: string;
  conflict: string;
  conflictLabel?: string;
}

export interface OutlookOpportunity {
  taskId: string;
  title: string;
  whenLabel?: string;
  unplacedLabel?: string;
  needsApproval: boolean;
}

export interface OutlookFreshness {
  state: string;
  reason?: string;
  explanation: string;
  ageLabel?: string;
  trusted: boolean;
}

export interface OutlookData {
  status: OutlookStatus;
  refusal?: { code: string; message: string };
  freshness: OutlookFreshness;
  horizonLabel: string;
  horizonHours: number;
  days: OutlookDayMark[];
  segments: OutlookSegment[];
  nextSleepLabel?: string;
  nextWakeLabel?: string;
  officeHoursLabel: string;
  officeWindows: OutlookOfficeWindow[];
  commitments: OutlookCommitment[];
  opportunities: OutlookOpportunity[];
  awakeLabel: string;
  uncertainLabel: string;
  withheldMessage?: string;
  disclaimer: string;
}

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function num(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

const PRESENCES: readonly Presence[] = ["awake", "asleep", "uncertain", "unknown"];
const OFFICE_STATUSES: readonly OfficeStatus[] = ["reachable", "partial", "unreachable"];

export const outlookUnavailable: OutlookData = {
  status: "unavailable",
  freshness: {
    state: "withheld",
    explanation: "This browser preview cannot read your records.",
    trusted: false,
  },
  horizonLabel: "Next 72 hours",
  horizonHours: 72,
  days: [],
  segments: [],
  officeHoursLabel: "Typical office hours, Monday to Friday 9:00 AM to 5:00 PM",
  officeWindows: [],
  commitments: [],
  opportunities: [],
  awakeLabel: "0 minutes",
  uncertainLabel: "0 minutes",
  withheldMessage: "Open the ZeitBoard desktop app to see the next three days.",
  disclaimer:
    "Estimates describe observed sleep-wake timing and uncertainty. This application does not provide medical advice.",
};

function normalizeSegment(value: unknown): OutlookSegment | undefined {
  if (!isRecord(value)) return undefined;
  const presence = PRESENCES.find((candidate) => candidate === value.presence);
  const rangeLabel = str(value.rangeLabel);
  const dayLabel = str(value.dayLabel);
  const durationLabel = str(value.durationLabel);
  const offsetHours = num(value.offsetHours);
  const durationHours = num(value.durationHours);
  if (
    !presence ||
    !rangeLabel ||
    !dayLabel ||
    !durationLabel ||
    offsetHours === undefined ||
    durationHours === undefined
  ) {
    return undefined;
  }
  return {
    presence,
    observed: value.observed === true,
    rangeLabel,
    dayLabel,
    durationLabel,
    offsetHours,
    durationHours,
  };
}

function normalizeOffice(value: unknown): OutlookOfficeWindow | undefined {
  if (!isRecord(value)) return undefined;
  const status = OFFICE_STATUSES.find((candidate) => candidate === value.status);
  const dayLabel = str(value.dayLabel);
  const hoursLabel = str(value.hoursLabel);
  const detail = str(value.detail);
  const offsetHours = num(value.offsetHours);
  const durationHours = num(value.durationHours);
  if (
    !status ||
    !dayLabel ||
    !hoursLabel ||
    !detail ||
    offsetHours === undefined ||
    durationHours === undefined
  ) {
    return undefined;
  }
  const reachableLabel = str(value.reachableLabel);
  return {
    status,
    dayLabel,
    hoursLabel,
    detail,
    offsetHours,
    durationHours,
    ...(reachableLabel ? { reachableLabel } : {}),
  };
}

function normalizeCommitment(value: unknown): OutlookCommitment | undefined {
  if (!isRecord(value)) return undefined;
  const title = str(value.title);
  const whenLabel = str(value.whenLabel);
  const conflict = str(value.conflict);
  if (!title || !whenLabel || !conflict) return undefined;
  const conflictLabel = str(value.conflictLabel);
  return { title, whenLabel, conflict, ...(conflictLabel ? { conflictLabel } : {}) };
}

function normalizeOpportunity(value: unknown): OutlookOpportunity | undefined {
  if (!isRecord(value)) return undefined;
  const taskId = str(value.taskId);
  if (!taskId) return undefined;
  const whenLabel = str(value.whenLabel);
  const unplacedLabel = str(value.unplacedLabel);
  return {
    taskId,
    title: str(value.title) ?? "Untitled task",
    needsApproval: value.needsApproval === true,
    ...(whenLabel ? { whenLabel } : {}),
    ...(unplacedLabel ? { unplacedLabel } : {}),
  };
}

function normalizeFreshness(value: unknown): OutlookFreshness | undefined {
  if (!isRecord(value)) return undefined;
  const state = str(value.state);
  const explanation = str(value.explanation);
  if (!state || !explanation) return undefined;
  const reason = str(value.reason);
  const ageLabel = str(value.ageLabel);
  return {
    state,
    explanation,
    trusted: value.trusted === true,
    ...(reason ? { reason } : {}),
    ...(ageLabel ? { ageLabel } : {}),
  };
}

export function normalizeOutlook(value: unknown): OutlookData | undefined {
  if (!isRecord(value)) return undefined;
  const status = ["available", "withheld", "refused", "unavailable"].find(
    (candidate) => candidate === value.status,
  ) as OutlookStatus | undefined;
  const freshness = normalizeFreshness(value.freshness);
  const horizonLabel = str(value.horizonLabel);
  const horizonHours = num(value.horizonHours);
  const disclaimer = str(value.disclaimer);
  if (!status || !freshness || !horizonLabel || horizonHours === undefined || !disclaimer) {
    return undefined;
  }

  const segments: OutlookSegment[] = [];
  for (const item of Array.isArray(value.segments) ? value.segments : []) {
    const segment = normalizeSegment(item);
    if (!segment) return undefined;
    segments.push(segment);
  }
  const officeWindows: OutlookOfficeWindow[] = [];
  for (const item of Array.isArray(value.officeWindows) ? value.officeWindows : []) {
    const window = normalizeOffice(item);
    if (!window) return undefined;
    officeWindows.push(window);
  }
  const commitments: OutlookCommitment[] = [];
  for (const item of Array.isArray(value.commitments) ? value.commitments : []) {
    const commitment = normalizeCommitment(item);
    if (!commitment) return undefined;
    commitments.push(commitment);
  }
  const opportunities: OutlookOpportunity[] = [];
  for (const item of Array.isArray(value.opportunities) ? value.opportunities : []) {
    const opportunity = normalizeOpportunity(item);
    if (!opportunity) return undefined;
    opportunities.push(opportunity);
  }
  const days: OutlookDayMark[] = [];
  for (const item of Array.isArray(value.days) ? value.days : []) {
    if (!isRecord(item)) return undefined;
    const label = str(item.label);
    const offsetHours = num(item.offsetHours);
    if (!label || offsetHours === undefined) return undefined;
    days.push({ label, offsetHours });
  }

  const refusalValue = isRecord(value.refusal) ? value.refusal : undefined;
  const refusalCode = refusalValue ? str(refusalValue.code) : undefined;
  const refusalMessage = refusalValue ? str(refusalValue.message) : undefined;

  return {
    status,
    freshness,
    horizonLabel,
    horizonHours,
    days,
    segments,
    officeHoursLabel: str(value.officeHoursLabel) ?? outlookUnavailable.officeHoursLabel,
    officeWindows,
    commitments,
    opportunities,
    awakeLabel: str(value.awakeLabel) ?? "0 minutes",
    uncertainLabel: str(value.uncertainLabel) ?? "0 minutes",
    disclaimer,
    ...(refusalCode && refusalMessage
      ? { refusal: { code: refusalCode, message: refusalMessage } }
      : {}),
    ...(str(value.nextSleepLabel) ? { nextSleepLabel: str(value.nextSleepLabel) as string } : {}),
    ...(str(value.nextWakeLabel) ? { nextWakeLabel: str(value.nextWakeLabel) as string } : {}),
    ...(str(value.withheldMessage)
      ? { withheldMessage: str(value.withheldMessage) as string }
      : {}),
  };
}

/**
 * Loads the view, falling back to `fallback` when the desktop bridge is absent
 * or answers with a shape this build does not recognise.
 *
 * A half-understood payload is treated as no payload. A timeline is read as a
 * plan, and a plan assembled from the fields that happened to parse is worse
 * than an honest blank.
 */
export async function loadOutlook(
  root: WailsRoot = globalThis as unknown as WailsRoot,
  fallback: OutlookData = outlookUnavailable,
): Promise<OutlookData> {
  const method = findWailsMethod(root, ["GetOutlook"]);
  if (!method) return fallback;
  try {
    const normalized = normalizeOutlook(await method());
    if (normalized) return normalized;
  } catch {
    // fall through to the fallback
  }
  return fallback;
}

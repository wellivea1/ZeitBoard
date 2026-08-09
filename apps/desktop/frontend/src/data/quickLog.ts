import { notifySleepDataChanged } from "./sleepDataEvents";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// One-tap sleep logging. The judgement lives in core/quicklog; this module only
// validates the shape that crosses the bridge and keeps the closed outcome set
// closed — an outcome this build does not recognise must not be rendered as a
// success, because the difference between "recorded" and "needs an answer" is
// the difference between a night in the log and a night lost.

export type QuickLogOutcome =
  | "record"
  | "pending"
  | "discarded"
  | "confirm_onset"
  | "confirm_short"
  | "confirm_long"
  | "confirm_stale"
  | "reject";

export interface QuickLogState {
  status: "ok" | "unavailable";
  message?: string;
  pending: boolean;
  pendingLabel?: string;
  pendingSince?: string;
  pendingStale: boolean;
}

export interface QuickLogResult {
  outcome: QuickLogOutcome;
  reason: string;
  recorded: boolean;
  entry?: string;
  suggestedStartLocal?: string;
  suggestedEndLocal?: string;

  /** True when the prefilled start came from the estimator, not from a tap. */
  suggestionIsPrediction: boolean;
  state: QuickLogState;
}

export interface ConfirmQuickSleepInput {
  startLocal: string;
  endLocal: string;
  zoneId?: string;
  classification?: "principal" | "nap";
}

const OUTCOMES: readonly QuickLogOutcome[] = [
  "record",
  "pending",
  "discarded",
  "confirm_onset",
  "confirm_short",
  "confirm_long",
  "confirm_stale",
  "reject",
];

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

export const quickLogUnavailable: QuickLogState = {
  status: "unavailable",
  message: "Open the ZeitBoard desktop app to log sleep with one tap.",
  pending: false,
  pendingStale: false,
};

export function normalizeQuickLogState(value: unknown): QuickLogState | undefined {
  if (!isRecord(value)) return undefined;
  const status = value.status === "ok" || value.status === "unavailable" ? value.status : undefined;
  if (!status) return undefined;
  const message = str(value.message);
  const pendingLabel = str(value.pendingLabel);
  const pendingSince = str(value.pendingSince);
  return {
    status,
    pending: value.pending === true,
    pendingStale: value.pendingStale === true,
    ...(message ? { message } : {}),
    ...(pendingLabel ? { pendingLabel } : {}),
    ...(pendingSince ? { pendingSince } : {}),
  };
}

export function normalizeQuickLogResult(value: unknown): QuickLogResult | undefined {
  if (!isRecord(value)) return undefined;
  const outcome = OUTCOMES.find((candidate) => candidate === value.outcome);
  const reason = str(value.reason);
  const state = normalizeQuickLogState(value.state);
  if (!outcome || !reason || !state) return undefined;
  const entry = str(value.entry);
  const suggestedStartLocal = str(value.suggestedStartLocal);
  const suggestedEndLocal = str(value.suggestedEndLocal);
  return {
    outcome,
    reason,
    recorded: value.recorded === true,
    suggestionIsPrediction: value.suggestionIsPrediction === true,
    state,
    ...(entry ? { entry } : {}),
    ...(suggestedStartLocal ? { suggestedStartLocal } : {}),
    ...(suggestedEndLocal ? { suggestedEndLocal } : {}),
  };
}

export async function loadQuickLogState(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<QuickLogState> {
  const method = findWailsMethod(root, ["GetQuickLogState"]);
  if (!method) return quickLogUnavailable;
  try {
    const normalized = normalizeQuickLogState(await method());
    if (normalized) return normalized;
  } catch {
    // fall through
  }
  return quickLogUnavailable;
}

async function tap(
  names: readonly string[],
  input: unknown,
  root: WailsRoot,
): Promise<QuickLogResult> {
  const method = findWailsMethod(root, names);
  if (!method) throw new Error("One-tap logging needs the ZeitBoard desktop app.");
  const normalized = normalizeQuickLogResult(await method(input));
  if (!normalized) throw new Error("That did not work.");
  // A recorded night changes the estimate, the outlook, and everything drawn
  // from them.
  if (normalized.recorded) notifySleepDataChanged();
  return normalized;
}

export function beginQuickSleep(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<QuickLogResult> {
  return tap(["BeginQuickSleep"], undefined, root);
}

export function completeQuickSleep(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<QuickLogResult> {
  return tap(["CompleteQuickSleep"], undefined, root);
}

export function discardQuickSleep(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<QuickLogResult> {
  return tap(["DiscardQuickSleep"], undefined, root);
}

export function confirmQuickSleep(
  input: ConfirmQuickSleepInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<QuickLogResult> {
  return tap(["ConfirmQuickSleep"], input, root);
}

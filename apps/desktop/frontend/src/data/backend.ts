import { overviewFixture } from "./fixture";
import type { OverviewData, OverviewResult } from "./overview";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

type UnknownRecord = Record<string, unknown>;

const methodNames = ["GetOverview", "Overview"] as const;

function isOverviewData(value: unknown): value is OverviewData {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<OverviewData>;

  return (
    typeof candidate.fixtureMode === "boolean" &&
    ["estimated", "empty", "refused", "unavailable"].includes(candidate.status ?? "") &&
    typeof candidate.empty === "boolean" &&
    typeof candidate.state === "string" &&
    typeof candidate.stateDetail === "string" &&
    typeof candidate.timeSinceWake === "string" &&
    typeof candidate.nextSleepWindow?.label === "string" &&
    typeof candidate.nextSleepWindow.uncertainty === "string" &&
    typeof candidate.drift?.label === "string" &&
    typeof candidate.drift.direction === "string" &&
    ["Low", "Medium", "High"].includes(candidate.confidence?.level ?? "") &&
    typeof candidate.confidence?.reason === "string" &&
    typeof candidate.usefulTaskWindow?.label === "string" &&
    typeof candidate.usefulTaskWindow.detail === "string" &&
    typeof candidate.sharingStatus?.active === "boolean" &&
    typeof candidate.sharingStatus.label === "string" &&
    typeof candidate.sharingStatus.detail === "string" &&
    typeof candidate.updatedLabel === "string"
  );
}

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value : undefined;
}

function normalizeConfidence(value: unknown): OverviewData["confidence"]["level"] | undefined {
  const normalized = asString(value)?.toLowerCase();
  if (normalized === "low") return "Low";
  if (normalized === "medium" || normalized === "moderate") return "Medium";
  if (normalized === "high") return "High";
  return undefined;
}

function normalizeStatus(value: unknown): OverviewData["status"] {
  if (value === "empty" || value === "refused" || value === "unavailable") return value;
  return "estimated";
}

function normalizeSource(value: unknown, fixtureMode = false): OverviewResult["source"] {
  if (fixtureMode) return "fixture";
  return value === "synced" ? "synced" : "local";
}

function normalizeRefusal(value: unknown): OverviewData["refusal"] | undefined {
  if (!isRecord(value)) return undefined;
  const code = asString(value.code);
  const message = asString(value.message);
  return code && message ? { code, message } : undefined;
}

function normalizeWailsOverview(value: unknown): OverviewData | undefined {
  if (isOverviewData(value)) return value;
  if (!isRecord(value)) return undefined;

  const state = asString(value.currentEstimatedState);
  const timeSinceWake = asString(value.timeSinceWake);
  const sleepWindow = asString(value.predictedNextSleepWindow);
  const drift = asString(value.driftEstimate);
  const confidenceLevel = normalizeConfidence(value.confidence);
  const usefulTaskWindow = asString(value.nextUsefulTaskWindow);
  const sharingStatus = asString(value.sharingStatus);
  const status = normalizeStatus(value.status);
  const fixtureMode = value.fixtureMode === true;
  const source = normalizeSource(value.estimateSource, fixtureMode);

  if (
    !state ||
    !timeSinceWake ||
    !sleepWindow ||
    !drift ||
    !confidenceLevel ||
    !usefulTaskWindow ||
    !sharingStatus
  ) {
    return undefined;
  }

  const confidenceReasons = Array.isArray(value.confidenceReasons)
    ? value.confidenceReasons.filter((reason): reason is string => typeof reason === "string")
    : [];

  return {
    fixtureMode,
    status,
    empty: value.empty === true,
    ...(normalizeRefusal(value.refusal) ? { refusal: normalizeRefusal(value.refusal) } : {}),
    state,
    stateDetail:
      source === "fixture"
        ? "Sample projection for interface review"
        : source === "synced"
          ? "Synced server estimate from the enrolled backend"
          : status === "estimated"
            ? "Local estimate based on recent sleep-wake observations"
            : "Local estimate is waiting for enough manually entered sleep data",
    timeSinceWake,
    nextSleepWindow: {
      label: sleepWindow,
      uncertainty: "Predicted range includes estimation uncertainty",
    },
    drift: {
      label: drift,
      direction: drift.trim().startsWith("-")
        ? "Earlier per observed cycle"
        : "Later per observed cycle",
    },
    confidence: {
      level: confidenceLevel,
      reason: confidenceReasons.join(" ") || "Confidence supplied by the local estimation service",
    },
    freshness: normalizeFreshness(value.freshness),
    usefulTaskWindow: {
      label: usefulTaskWindow,
      detail: "Proposal from current task constraints and estimated availability",
    },
    sharingStatus: {
      active: !sharingStatus.toLowerCase().includes("no active"),
      label: sharingStatus,
      detail: "Trusted views contain only explicitly allowlisted fields",
    },
    updatedLabel: asString(value.updatedLabel) ?? "Updated from the local service just now",
  };
}

// normalizeFreshness defaults to a withheld verdict rather than a trusting one.
// A build whose service predates the freshness policy must not be read as
// "evidence is current"; absence of a verdict is not a positive verdict.
function normalizeFreshness(value: unknown): OverviewData["freshness"] {
  const fallback: OverviewData["freshness"] = {
    state: "withheld",
    reason: "no_evidence",
    explanation: "This build cannot confirm how recent the underlying records are.",
    ageLabel: "",
    trusted: false,
  };
  if (typeof value !== "object" || value === null) return fallback;
  const record = value as Record<string, unknown>;
  const state = record.state;
  if (state !== "current" && state !== "stale" && state !== "withheld") return fallback;
  return {
    state,
    reason: asString(record.reason) ?? "",
    explanation: asString(record.explanation) ?? fallback.explanation,
    ageLabel: asString(record.ageLabel) ?? "",
    trusted: record.trusted === true,
  };
}

export async function loadOverview(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<OverviewResult> {
  const method = findWailsMethod(root, methodNames);
  if (!method) return { data: overviewFixture, source: "fixture" };

  try {
    const result = await method();
    const overview = normalizeWailsOverview(result);
    if (overview) {
      const source = isRecord(result)
        ? normalizeSource(result.estimateSource, overview.fixtureMode)
        : normalizeSource(undefined, overview.fixtureMode);
      return { data: overview, source };
    }
  } catch {
    // Fixture mode keeps the desktop shell usable before the Wails service is ready.
  }

  return { data: overviewFixture, source: "fixture" };
}

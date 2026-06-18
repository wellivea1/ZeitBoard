import {
  rhythmActogramFixture,
  rhythmDriftFixture,
  type RhythmDriftPointFixture,
  type RhythmSleepBandFixture,
} from "./phaseTwo";
import type { ConfidenceLevel } from "./overview";

export type RhythmSource = "backend" | "fixture";

export interface RhythmActogram {
  summary: string;
  observedRows: RhythmSleepBandFixture[];
  forecastRows: RhythmSleepBandFixture[];
  now: { label: string; day: string; hour: number };
}

export interface RhythmDrift {
  title: string;
  slopeLabel: string;
  confidence: ConfidenceLevel;
  summary: string;
  yMinHour: number;
  yMaxHour: number;
  points: RhythmDriftPointFixture[];
}

export interface RhythmData {
  fixtureMode: boolean;
  status: "estimated" | "empty" | "refused" | "unavailable";
  message?: string;
  refusal?: {
    code: string;
    message: string;
  };
  actogram: RhythmActogram;
  drift: RhythmDrift;
}

export interface RhythmResult {
  data: RhythmData;
  source: RhythmSource;
}

// The fixture is repackaged from the shared phaseTwo data so the offline shell
// renders the same shape the backend supplies, and the two never diverge.
export const rhythmFixture: RhythmData = {
  fixtureMode: true,
  status: "estimated",
  actogram: {
    summary: rhythmActogramFixture.summary,
    observedRows: rhythmActogramFixture.observedRows,
    forecastRows: rhythmActogramFixture.forecastRows,
    now: rhythmActogramFixture.now,
  },
  drift: {
    title: rhythmDriftFixture.title,
    slopeLabel: rhythmDriftFixture.slopeLabel,
    confidence: rhythmDriftFixture.confidence,
    summary: rhythmDriftFixture.summary,
    yMinHour: rhythmDriftFixture.yMinHour,
    yMaxHour: rhythmDriftFixture.yMaxHour,
    points: rhythmDriftFixture.points,
  },
};

type RhythmMethod = () => Promise<unknown>;
type UnknownRecord = Record<string, unknown>;

interface WailsRoot {
  go?: Record<string, Record<string, Record<string, unknown>>>;
}

const methodNames = ["GetRhythm", "Rhythm"] as const;

function findRhythmMethod(root: WailsRoot): RhythmMethod | undefined {
  const packages = root.go;
  if (!packages) return undefined;

  for (const packageValue of Object.values(packages)) {
    for (const serviceValue of Object.values(packageValue)) {
      for (const methodName of methodNames) {
        const candidate = serviceValue[methodName];
        if (typeof candidate === "function") {
          return (candidate as RhythmMethod).bind(serviceValue);
        }
      }
    }
  }

  return undefined;
}

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function num(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function confidence(value: unknown): ConfidenceLevel | undefined {
  const normalized = str(value)?.toLowerCase();
  if (normalized === "low") return "Low";
  if (normalized === "medium" || normalized === "moderate") return "Medium";
  if (normalized === "high") return "High";
  return undefined;
}

function status(value: unknown): RhythmData["status"] {
  if (value === "empty" || value === "refused" || value === "unavailable") return value;
  return "estimated";
}

function refusal(value: unknown): RhythmData["refusal"] | undefined {
  if (!isRecord(value)) return undefined;
  const code = str(value.code);
  const message = str(value.message);
  return code && message ? { code, message } : undefined;
}

function band(value: unknown): RhythmSleepBandFixture | undefined {
  if (!isRecord(value)) return undefined;
  const id = str(value.id);
  const day = str(value.day);
  const startHour = num(value.startHour);
  const durationHours = num(value.durationHours);
  const kind = str(value.kind);
  const startLabel = str(value.startLabel);
  const wakeLabel = str(value.wakeLabel);
  const durationLabel = str(value.durationLabel);
  const source = str(value.source);
  const level = confidence(value.confidence);
  if (
    !id ||
    !day ||
    startHour === undefined ||
    durationHours === undefined ||
    !kind ||
    !startLabel ||
    !wakeLabel ||
    !durationLabel ||
    !source ||
    !level
  ) {
    return undefined;
  }
  return {
    id,
    day,
    startHour,
    durationHours,
    kind: kind as RhythmSleepBandFixture["kind"],
    startLabel,
    wakeLabel,
    durationLabel,
    source,
    confidence: level,
  };
}

function point(value: unknown): RhythmDriftPointFixture | undefined {
  if (!isRecord(value)) return undefined;
  const id = str(value.id);
  const day = str(value.day);
  const onsetHour = num(value.onsetHour);
  const fitHour = num(value.fitHour);
  const bandLowHour = num(value.bandLowHour);
  const bandHighHour = num(value.bandHighHour);
  const onsetLabel = str(value.onsetLabel);
  const source = str(value.source);
  const level = confidence(value.confidence);
  if (
    !id ||
    !day ||
    onsetHour === undefined ||
    fitHour === undefined ||
    bandLowHour === undefined ||
    bandHighHour === undefined ||
    !onsetLabel ||
    !source ||
    !level
  ) {
    return undefined;
  }
  return { id, day, onsetHour, fitHour, bandLowHour, bandHighHour, onsetLabel, source, confidence: level };
}

function mapAll<T>(value: unknown, map: (item: unknown) => T | undefined): T[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const mapped: T[] = [];
  for (const item of value) {
    const next = map(item);
    if (!next) return undefined;
    mapped.push(next);
  }
  return mapped;
}

function normalizeNow(value: unknown): RhythmActogram["now"] | undefined {
  if (!isRecord(value)) return undefined;
  const label = str(value.label);
  const day = str(value.day);
  const hour = num(value.hour);
  if (!label || !day || hour === undefined) return undefined;
  return { label, day, hour };
}

export function normalizeRhythm(value: unknown): RhythmData | undefined {
  if (!isRecord(value)) return undefined;

  const rhythmStatus = status(value.status);
  const observedRows = mapAll(value.observedRows, band);
  const forecastRows = mapAll(value.forecastRows, band);
  const points = mapAll(value.driftPoints, point);
  const now = normalizeNow(value.now);
  const actogramSummary = str(value.actogramSummary);
  const driftTitle = str(value.driftTitle);
  const slopeLabel = str(value.slopeLabel);
  const driftConfidence = confidence(value.driftConfidence);
  const driftSummary = str(value.driftSummary);
  const yMinHour = num(value.yMinHour);
  const yMaxHour = num(value.yMaxHour);

  if (
    !observedRows ||
    !forecastRows ||
    !points ||
    !now ||
    !actogramSummary ||
    !driftTitle ||
    !slopeLabel ||
    !driftConfidence ||
    !driftSummary ||
    yMinHour === undefined ||
    yMaxHour === undefined ||
    yMaxHour <= yMinHour
  ) {
    return undefined;
  }
  if (rhythmStatus === "estimated" && (observedRows.length === 0 || points.length === 0)) {
    return undefined;
  }

  return {
    fixtureMode: value.fixtureMode === true,
    status: rhythmStatus,
    message: rhythmStatus === "estimated" ? undefined : driftSummary,
    ...(refusal(value.refusal) ? { refusal: refusal(value.refusal) } : {}),
    actogram: { summary: actogramSummary, observedRows, forecastRows, now },
    drift: {
      title: driftTitle,
      slopeLabel,
      confidence: driftConfidence,
      summary: driftSummary,
      yMinHour,
      yMaxHour,
      points,
    },
  };
}

export async function loadRhythm(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<RhythmResult> {
  const method = findRhythmMethod(root);
  if (!method) return { data: rhythmFixture, source: "fixture" };

  try {
    const result = await method();
    const rhythm = normalizeRhythm(result);
    if (rhythm) return { data: rhythm, source: "backend" };
  } catch {
    // Fixture mode keeps the Rhythm screen usable before the Wails service is ready.
  }

  return { data: rhythmFixture, source: "fixture" };
}

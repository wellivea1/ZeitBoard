import { findWailsMethod, type WailsRoot } from "../data/wailsBridge";

// Rhythm-linked appearance switching (ADR-0021, ui-refactor-plan U-D): an
// opt-in local display rule — engage a night preset N hours before the
// PREDICTED sleep onset and release it at the predicted wake. When the
// estimator refuses, the rule honestly falls back to fixed civil times the
// user set (or stays inactive). Display switching is reversible and local;
// it never touches the approval queue.

export type NightPreset = "amber" | "black" | "dark";

export interface NightRule {
  enabled: boolean;
  preset: NightPreset;
  leadHours: number;
  fallbackStartLocal: string; // "HH:MM", used when no estimate exists
  fallbackEndLocal: string;
}

export interface AppearanceClock {
  status: string;
  sleepStartAt?: string;
  wakeAt?: string;
}

export interface NightWindow {
  active: boolean;
  source: "forecast" | "civil" | null;
}

const STORAGE_KEY = "zeitboard-night-rule";

export const defaultNightRule: NightRule = {
  enabled: false,
  preset: "amber",
  leadHours: 2,
  fallbackStartLocal: "",
  fallbackEndLocal: "",
};

export function getStoredNightRule(): NightRule {
  try {
    const raw = typeof window !== "undefined" ? localStorage.getItem(STORAGE_KEY) : null;
    if (!raw) return defaultNightRule;
    const parsed = JSON.parse(raw) as Partial<NightRule>;
    return {
      enabled: parsed.enabled === true,
      preset:
        parsed.preset === "black" || parsed.preset === "dark" || parsed.preset === "amber"
          ? parsed.preset
          : "amber",
      leadHours:
        typeof parsed.leadHours === "number" && parsed.leadHours >= 0 && parsed.leadHours <= 12
          ? parsed.leadHours
          : 2,
      fallbackStartLocal: typeof parsed.fallbackStartLocal === "string" ? parsed.fallbackStartLocal : "",
      fallbackEndLocal: typeof parsed.fallbackEndLocal === "string" ? parsed.fallbackEndLocal : "",
    };
  } catch {
    return defaultNightRule;
  }
}

export function storeNightRule(rule: NightRule): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(rule));
  } catch {
    // Restricted storage must not break appearance controls.
  }
}

function minutesOfDay(value: string): number | undefined {
  const match = /^(\d{1,2}):(\d{2})$/.exec(value);
  if (!match) return undefined;
  const hours = Number(match[1]);
  const minutes = Number(match[2]);
  if (hours > 23 || minutes > 59) return undefined;
  return hours * 60 + minutes;
}

// evaluateNightWindow is pure so the rule is testable: forecast first,
// honest civil fallback second, inactive otherwise.
export function evaluateNightWindow(rule: NightRule, clock: AppearanceClock, now: Date): NightWindow {
  if (!rule.enabled) return { active: false, source: null };

  if (clock.status === "estimated" && clock.sleepStartAt && clock.wakeAt) {
    const sleepStart = Date.parse(clock.sleepStartAt);
    const wake = Date.parse(clock.wakeAt);
    if (Number.isFinite(sleepStart) && Number.isFinite(wake) && wake > sleepStart) {
      const engageAt = sleepStart - rule.leadHours * 3_600_000;
      const active = now.getTime() >= engageAt && now.getTime() < wake;
      return { active, source: "forecast" };
    }
  }

  const start = minutesOfDay(rule.fallbackStartLocal);
  const end = minutesOfDay(rule.fallbackEndLocal);
  if (start === undefined || end === undefined || start === end) {
    return { active: false, source: null };
  }
  const minute = now.getHours() * 60 + now.getMinutes();
  const active = start < end ? minute >= start && minute < end : minute >= start || minute < end;
  return { active, source: "civil" };
}

export async function loadAppearanceClock(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<AppearanceClock> {
  const method = findWailsMethod(root, ["GetAppearanceClock"]);
  if (!method) return { status: "unavailable" };
  try {
    const result = (await method()) as AppearanceClock | null;
    if (result && typeof result.status === "string") {
      return {
        status: result.status,
        ...(typeof result.sleepStartAt === "string" && result.sleepStartAt
          ? { sleepStartAt: result.sleepStartAt }
          : {}),
        ...(typeof result.wakeAt === "string" && result.wakeAt ? { wakeAt: result.wakeAt } : {}),
      };
    }
  } catch {
    // fall through
  }
  return { status: "unavailable" };
}

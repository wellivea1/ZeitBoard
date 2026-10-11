import { localeClock, type Clock } from "../utils/windowClock";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// The clock setting (ADR-0057): which clock times are written on, and why.
// The desktop resolves "system" against Windows' regional format, so every
// label it writes and every time the window writes agree.

export type ClockPreference = "system" | Clock;

export interface ClockSetting {
  preference: ClockPreference;
  /** The clock in use. */
  clock: Clock;
  /** The computer's own clock. */
  system: Clock;
  revision: number;
  /** True when a stale edit lost to what was already stored. */
  conflict: boolean;
  message?: string;
}

const PREFERENCES: readonly ClockPreference[] = ["system", "12", "24"];
const CLOCKS: readonly Clock[] = ["12", "24"];

/** Outside the desktop the browser's own clock is the only one there is. */
export function clockUnavailable(): ClockSetting {
  const clock = localeClock();
  return { preference: "system", clock, system: clock, revision: 0, conflict: false };
}

export function normalizeClockSetting(value: unknown): ClockSetting | undefined {
  if (typeof value !== "object" || value === null) return undefined;
  const record = value as Record<string, unknown>;
  const preference = PREFERENCES.find((candidate) => candidate === record.preference);
  const clock = CLOCKS.find((candidate) => candidate === record.clock);
  const system = CLOCKS.find((candidate) => candidate === record.system);
  if (!preference || !clock || !system) return undefined;
  const message = typeof record.message === "string" && record.message ? record.message : undefined;
  return {
    preference,
    clock,
    system,
    revision: typeof record.revision === "number" ? record.revision : 0,
    conflict: record.conflict === true,
    ...(message ? { message } : {}),
  };
}

export async function loadClockSetting(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ClockSetting | undefined> {
  const method = findWailsMethod(root, ["GetClockSetting"]);
  if (!method) return clockUnavailable();
  try {
    return normalizeClockSetting(await method());
  } catch {
    // The window keeps the clock it has; the next read may answer.
    return undefined;
  }
}

export async function saveClockSetting(
  preference: ClockPreference,
  baseRevision: number,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ClockSetting> {
  const method = findWailsMethod(root, ["SaveClockSetting"]);
  if (!method) throw new Error("The clock is set in the ZeitBoard desktop app.");
  const saved = normalizeClockSetting(await method({ preference, baseRevision }));
  if (!saved) throw new Error("The clock could not be saved.");
  return saved;
}

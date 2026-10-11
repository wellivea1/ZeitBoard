import { useSyncExternalStore } from "react";

// The clock the window writes times on (ADR-0057). The desktop decides it:
// Windows' own clock unless the owner chose one in Settings › Display. The
// last one is remembered on this computer, so the first page drawn already
// uses it while the desktop is asked again.

export type Clock = "12" | "24";

const storageKey = "zeitboard.clock";
const listeners = new Set<() => void>();

function isClock(value: unknown): value is Clock {
  return value === "12" || value === "24";
}

/** The clock the browser's own locale writes, for when nothing better is known. */
export function localeClock(): Clock {
  const cycle = new Intl.DateTimeFormat(undefined, { hour: "numeric" }).resolvedOptions().hourCycle;
  return cycle === "h23" || cycle === "h24" ? "24" : "12";
}

function remembered(): Clock {
  try {
    const value = window.localStorage.getItem(storageKey);
    if (isClock(value)) return value;
  } catch {
    // Storage is unavailable; the locale's clock stands in.
  }
  return localeClock();
}

let current: Clock = remembered();

export function windowClock(): Clock {
  return current;
}

export function setWindowClock(clock: Clock) {
  try {
    window.localStorage.setItem(storageKey, clock);
  } catch {
    // Remembered for this session only.
  }
  if (clock === current) return;
  current = clock;
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** The window's clock, for a component that redraws when it changes. */
export function useWindowClock(): Clock {
  return useSyncExternalStore(subscribe, windowClock, windowClock);
}

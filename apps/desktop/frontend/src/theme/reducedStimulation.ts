const STORAGE_KEY = "zeitboard-reduced";

export type ReducedStimulation = boolean;

export function getStoredReducedStimulation(): boolean {
  try {
    const value = typeof window !== "undefined" ? localStorage.getItem(STORAGE_KEY) : null;
    return value === "true";
  } catch {
    return false;
  }
}

export function storeReducedStimulation(enabled: boolean): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(STORAGE_KEY, enabled ? "true" : "false");
  } catch {
    // Restricted storage should not make appearance controls unusable.
  }
}

export function applyReducedStimulationAttribute(enabled: boolean): void {
  if (typeof document === "undefined") return;
  if (enabled) {
    document.documentElement.setAttribute("data-reduced", "true");
  } else {
    document.documentElement.removeAttribute("data-reduced");
  }
}

export function loadAndApplyReducedStimulation(): boolean {
  const enabled = getStoredReducedStimulation();
  applyReducedStimulationAttribute(enabled);
  return enabled;
}

import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export interface StartupSettings {
  available: boolean;
  registered: boolean;
  matchesCurrent: boolean;
  startHidden: boolean;
  trayAvailable: boolean;
  message: string;
}

async function call(name: string, input?: unknown) {
  const method = findWailsMethod(globalThis as WailsRoot, [name]);
  if (!method)
    throw new Error("Open the ZeitBoard desktop app to manage startup and background work.");
  return method(input);
}

function status(value: unknown): StartupSettings {
  if (typeof value !== "object" || value === null)
    throw new Error("Startup status is unavailable.");
  const row = value as Record<string, unknown>;
  if (
    ["available", "registered", "matchesCurrent", "startHidden", "trayAvailable"].some(
      (key) => typeof row[key] !== "boolean",
    ) ||
    typeof row.message !== "string"
  ) {
    throw new Error("Startup status is invalid. Refresh to try again.");
  }
  return row as unknown as StartupSettings;
}

export async function loadStartupSettings() {
  return status(await call("GetStartupSettings"));
}
export async function saveStartupSettings(enabled: boolean, startHidden: boolean) {
  return status(await call("SetStartupSettings", { enabled, startHidden }));
}
export async function hideDesktopWindow() {
  await call("HideWindow");
}
export async function quitDesktop() {
  await call("QuitApp");
}

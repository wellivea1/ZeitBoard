import type { NightRule } from "../theme/nightMode";
import type { ThemePreference } from "../theme/theme";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export const appearanceCommandEvent = "zeitboard:appearance-command";

export interface LocalAppearanceState {
  theme: ThemePreference;
  reducedStimulation: boolean;
  nightRule: NightRule;
}

export interface LocalAppearanceEnvelope {
  state: LocalAppearanceState;
  revision: number;
  conflict: boolean;
}

export interface LocalAgentStatus {
  schemaVersion: "v1";
  mode: "desktop_local";
  running: boolean;
  endpoint?: string;
  message: string;
  backendProposalsAvailable: boolean;
  localStoreAvailable: boolean;
  appearanceStatus: "ready" | "error";
}

interface RuntimeRoot extends WailsRoot {
  runtime?: {
    EventsOn?: (eventName: string, callback: (...data: unknown[]) => void) => unknown;
  };
}

const themes: ThemePreference[] = ["auto", "light", "dark", "black", "amber", "contrast"];
const nightPresets: NightRule["preset"][] = ["amber", "black", "dark"];
const civilClock = /^$|^(?:[01]\d|2[0-3]):[0-5]\d$/;

const browserStatus: LocalAgentStatus = {
  schemaVersion: "v1",
  mode: "desktop_local",
  running: false,
  message: "The desktop-local agent is available only in the installed desktop app.",
  backendProposalsAvailable: false,
  localStoreAvailable: false,
  appearanceStatus: "ready",
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function normalizeNightRule(value: unknown): NightRule | undefined {
  if (!isRecord(value) || typeof value.enabled !== "boolean") return undefined;
  if (!nightPresets.includes(value.preset as NightRule["preset"])) return undefined;
  if (
    typeof value.leadHours !== "number" ||
    !Number.isFinite(value.leadHours) ||
    value.leadHours < 0 ||
    value.leadHours > 12
  ) {
    return undefined;
  }
  if (
    typeof value.fallbackStartLocal !== "string" ||
    typeof value.fallbackEndLocal !== "string" ||
    !civilClock.test(value.fallbackStartLocal) ||
    !civilClock.test(value.fallbackEndLocal)
  ) {
    return undefined;
  }
  return {
    enabled: value.enabled,
    preset: value.preset as NightRule["preset"],
    leadHours: value.leadHours,
    fallbackStartLocal: value.fallbackStartLocal,
    fallbackEndLocal: value.fallbackEndLocal,
  };
}

export function normalizeLocalAppearance(value: unknown): LocalAppearanceState | undefined {
  if (!isRecord(value) || !themes.includes(value.theme as ThemePreference)) return undefined;
  if (typeof value.reducedStimulation !== "boolean") return undefined;
  const nightRule = normalizeNightRule(value.nightRule);
  if (!nightRule) return undefined;
  return {
    theme: value.theme as ThemePreference,
    reducedStimulation: value.reducedStimulation,
    nightRule,
  };
}

export function normalizeAppearanceEnvelope(value: unknown): LocalAppearanceEnvelope | undefined {
  if (!isRecord(value) || !Number.isSafeInteger(value.revision) || Number(value.revision) < 0) {
    return undefined;
  }
  const state = normalizeLocalAppearance(value.state);
  if (!state) return undefined;
  return {
    state,
    revision: Number(value.revision),
    conflict: value.conflict === true,
  };
}

export function normalizeLocalAgentStatus(value: unknown): LocalAgentStatus | undefined {
  if (!isRecord(value) || value.schemaVersion !== "v1" || value.mode !== "desktop_local") {
    return undefined;
  }
  if (
    typeof value.running !== "boolean" ||
    typeof value.message !== "string" ||
    typeof value.backendProposalsAvailable !== "boolean" ||
    typeof value.localStoreAvailable !== "boolean" ||
    (value.appearanceStatus !== "ready" && value.appearanceStatus !== "error")
  ) {
    return undefined;
  }
  return {
    schemaVersion: "v1",
    mode: "desktop_local",
    running: value.running,
    ...(typeof value.endpoint === "string" && value.endpoint ? { endpoint: value.endpoint } : {}),
    message: value.message,
    backendProposalsAvailable: value.backendProposalsAvailable,
    localStoreAvailable: value.localStoreAvailable,
    appearanceStatus: value.appearanceStatus,
  };
}

export async function loadLocalAgentStatus(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<LocalAgentStatus> {
  const method = findWailsMethod(root, ["GetLocalAgentStatus"]);
  if (!method) return browserStatus;
  const status = normalizeLocalAgentStatus(await method());
  if (!status) throw new Error("The desktop returned an invalid local-agent status.");
  return status;
}

export async function loadLocalAppearanceState(
  local: LocalAppearanceState,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<LocalAppearanceEnvelope> {
  const method = findWailsMethod(root, ["LoadLocalAppearanceState"]);
  if (!method) return { state: local, revision: 0, conflict: false };
  const envelope = normalizeAppearanceEnvelope(await method(local));
  if (!envelope) throw new Error("The desktop returned invalid appearance settings.");
  return envelope;
}

export async function saveLocalAppearanceState(
  state: LocalAppearanceState,
  baseRevision: number,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<LocalAppearanceEnvelope> {
  const method = findWailsMethod(root, ["SaveLocalAppearanceState"]);
  if (!method) return { state, revision: baseRevision, conflict: false };
  const envelope = normalizeAppearanceEnvelope(await method({ state, baseRevision }));
  if (!envelope) throw new Error("The desktop returned invalid appearance settings.");
  return envelope;
}

export function listenForAppearanceCommands(
  callback: (envelope: LocalAppearanceEnvelope) => void,
  root: RuntimeRoot = globalThis as unknown as RuntimeRoot,
): () => void {
  const eventsOn = root.runtime?.EventsOn;
  if (typeof eventsOn !== "function") return () => {};
  const dispose = eventsOn(appearanceCommandEvent, (value: unknown) => {
    const envelope = normalizeAppearanceEnvelope(value);
    if (envelope) callback(envelope);
  });
  return typeof dispose === "function" ? (dispose as () => void) : () => {};
}

import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// After the first forecast: the optional steps that change what Home can say,
// and which are done. The desktop says which are done; the words and the links
// belong to the window.

export type SetupStepId = "phone" | "reaching" | "calendar" | "startup";

export interface SetupStep {
  id: SetupStepId;
  done: boolean;
}

export interface SetupStepWords {
  title: string;
  body: string;
  link: { href: string; label: string };
}

export const setupStepWords: Record<SetupStepId, SetupStepWords> = {
  phone: {
    title: "Nights from your phone",
    body: "On Android, the companion reads Health Connect and sends each night through your own ZeitBoard server, so nights arrive without typing them.",
    link: { href: "#/settings/sync", label: "Set up sync" },
  },
  reaching: {
    title: "Who you need to reach",
    body: "Home marks when a clinic, a pharmacy or family are available while you are likely awake. It starts from a working week that is only a guess.",
    link: { href: "#/settings/reaching", label: "Set their hours" },
  },
  calendar: {
    title: "Your calendar",
    body: "Add a calendar file or a subscription. Its events are drawn against the forecast, and any that fall in likely sleep are marked.",
    link: { href: "#/data-sources", label: "Add a calendar" },
  },
  startup: {
    title: "Start with Windows",
    body: "ZeitBoard waits in the tray from sign-in, so reminders and sync carry on without the window open.",
    link: { href: "#/settings/computer", label: "Start at sign-in" },
  },
};

const STEP_IDS = Object.keys(setupStepWords) as SetupStepId[];

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

export function normalizeSetupGuide(value: unknown): SetupStep[] | undefined {
  if (!isRecord(value) || !Array.isArray(value.steps)) return undefined;
  const steps: SetupStep[] = [];
  for (const item of value.steps) {
    if (!isRecord(item)) return undefined;
    const id = STEP_IDS.find((candidate) => candidate === item.id);
    // A step this window has no words for is left out rather than shown blank.
    if (id && !steps.some((step) => step.id === id)) steps.push({ id, done: item.done === true });
  }
  return steps;
}

/** The steps, or none outside the desktop app or when they cannot be read. */
export async function loadSetupGuide(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SetupStep[]> {
  const method = findWailsMethod(root, ["GetSetupGuide"]);
  if (!method) return [];
  try {
    return normalizeSetupGuide(await method()) ?? [];
  } catch {
    return [];
  }
}

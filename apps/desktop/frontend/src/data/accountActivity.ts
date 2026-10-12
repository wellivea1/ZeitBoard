import { count, normalizeNightsFigure, type NightsFigure } from "./activityNights";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// Activity from a Google export (ADR-0059). The desktop reads only when each
// activity happened, keeps the minutes when asked, and compares the nights
// their gaps suggest with the recorded ones. Suggestions reach nothing else.

export interface AccountActivity {
  imported: boolean;
  minutes: number;
  importedLabel: string;
  /** The nights the kept activity suggests, once there is some. */
  figure?: NightsFigure;
}

export interface ActivityExportPreview {
  token: string;
  fileName: string;
  entries: number;
  unusable: number;
  minutes: number;
  nights: number;
  spanLabel: string;
}

export const accountActivityChangedEvent = "zeitboard:account-activity-changed";

export function notifyAccountActivityChanged() {
  window.dispatchEvent(new Event(accountActivityChangedEvent));
}

export const noAccountActivity: AccountActivity = {
  imported: false,
  minutes: 0,
  importedLabel: "",
};

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

export function normalizeAccountActivity(value: unknown): AccountActivity | undefined {
  if (!isRecord(value) || typeof value.imported !== "boolean") return undefined;
  if (!value.imported) return noAccountActivity;
  const minutes = count(value.minutes);
  const figure = normalizeNightsFigure(value.figure);
  if (minutes === undefined || !figure) return undefined;
  return {
    imported: true,
    minutes,
    importedLabel: typeof value.importedLabel === "string" ? value.importedLabel : "",
    figure,
  };
}

export function normalizeActivityExportPreview(value: unknown): ActivityExportPreview | undefined {
  if (!isRecord(value) || typeof value.token !== "string" || !value.token) return undefined;
  const entries = count(value.entries);
  const unusable = count(value.unusable);
  const minutes = count(value.minutes);
  const nights = count(value.nights);
  if (entries === undefined || unusable === undefined || minutes === undefined) return undefined;
  if (nights === undefined || minutes === 0) return undefined;
  return {
    token: value.token,
    fileName: typeof value.fileName === "string" ? value.fileName : "",
    entries,
    unusable,
    minutes,
    nights,
    spanLabel: typeof value.spanLabel === "string" ? value.spanLabel : "",
  };
}

/** Whether this window can open an export: only the desktop app can. */
export function canImportActivityExport(root: WailsRoot = globalThis as unknown as WailsRoot) {
  return findWailsMethod(root, ["PreviewActivityExport"]) !== undefined;
}

async function call(name: string, input: unknown, root: WailsRoot): Promise<unknown> {
  const method = findWailsMethod(root, [name]);
  if (!method) throw new Error("Importing activity needs the ZeitBoard desktop app.");
  return method(input);
}

function kept(value: unknown): AccountActivity {
  const activity = normalizeAccountActivity(value);
  if (!activity) throw new Error("The imported activity could not be read.");
  return activity;
}

/** The kept activity; none outside the desktop app. */
export async function loadAccountActivity(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<AccountActivity> {
  if (!findWailsMethod(root, ["GetAccountActivity"])) return noAccountActivity;
  return kept(await call("GetAccountActivity", undefined, root));
}

/** Opens an export and says what is in it, or nothing when none was chosen. */
export async function previewActivityExport(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ActivityExportPreview | undefined> {
  const value = await call("PreviewActivityExport", undefined, root);
  if (isRecord(value) && value.canceled === true) return undefined;
  const preview = normalizeActivityExportPreview(value);
  if (!preview) throw new Error("The export could not be read.");
  return preview;
}

export async function importActivityExport(
  token: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<AccountActivity> {
  return kept(await call("ImportActivityExport", { token }, root));
}

export async function removeAccountActivity(
  confirmation: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<AccountActivity> {
  return kept(await call("RemoveAccountActivity", { confirmation }, root));
}

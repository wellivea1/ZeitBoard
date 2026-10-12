import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// Activity from a Google export (ADR-0059). The desktop reads only when each
// activity happened, keeps the minutes when asked, and compares the nights
// their gaps suggest with the recorded ones. Suggestions reach nothing else.

/** Part of a night cut at midnight: [day, start minute, end minute], day 0
 * being the first date drawn. */
export type NightSegment = readonly [day: number, start: number, end: number];

export interface ActivityAgreement {
  /** Nights recorded while the activity was kept. */
  recorded: number;
  /** Of those, the nights a suggestion overlapped. */
  matched: number;
  /** How far apart their edges typically were, or empty when none matched. */
  summary: string;
}

export interface AccountActivity {
  imported: boolean;
  minutes: number;
  nights: number;
  spanLabel: string;
  importedLabel: string;
  zoneId: string;
  firstDate: string;
  days: number;
  suggested: NightSegment[];
  recorded: NightSegment[];
  agreement?: ActivityAgreement;
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
  nights: 0,
  spanLabel: "",
  importedLabel: "",
  zoneId: "",
  firstDate: "",
  days: 0,
  suggested: [],
  recorded: [],
};

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function count(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : undefined;
}

function segments(value: unknown, days: number): NightSegment[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const result: NightSegment[] = [];
  for (const item of value) {
    if (!Array.isArray(item) || item.length !== 3) return undefined;
    const [day, start, end] = item.map(count);
    if (day === undefined || start === undefined || end === undefined) return undefined;
    if (day >= days || end <= start || end > 24 * 60) return undefined;
    result.push([day, start, end]);
  }
  return result;
}

function agreement(value: unknown): ActivityAgreement | undefined {
  if (!isRecord(value)) return undefined;
  const recorded = count(value.recorded);
  const matched = count(value.matched);
  if (recorded === undefined || matched === undefined || matched > recorded) return undefined;
  return {
    recorded,
    matched,
    summary: typeof value.summary === "string" ? value.summary : "",
  };
}

export function normalizeAccountActivity(value: unknown): AccountActivity | undefined {
  if (!isRecord(value) || typeof value.imported !== "boolean") return undefined;
  if (!value.imported) return noAccountActivity;
  const minutes = count(value.minutes);
  const nights = count(value.nights);
  const days = count(value.days);
  if (minutes === undefined || nights === undefined || days === undefined || days === 0) {
    return undefined;
  }
  const suggested = segments(value.suggested, days);
  const recorded = segments(value.recorded, days);
  if (!suggested || !recorded || typeof value.firstDate !== "string") return undefined;
  const compared = agreement(value.agreement);
  return {
    imported: true,
    minutes,
    nights,
    spanLabel: typeof value.spanLabel === "string" ? value.spanLabel : "",
    importedLabel: typeof value.importedLabel === "string" ? value.importedLabel : "",
    zoneId: typeof value.zoneId === "string" ? value.zoneId : "",
    firstDate: value.firstDate,
    days,
    suggested,
    recorded,
    ...(compared ? { agreement: compared } : {}),
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

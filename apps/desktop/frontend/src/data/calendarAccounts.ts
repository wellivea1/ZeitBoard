import {
  accountProvider,
  normalizeCalendarImport,
  type CalendarAccountProvider,
  type CalendarImportReport,
} from "./calendar";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// Calendars from a Google or Microsoft account (ADR-0064), read through the
// owner's own app registration. The window never holds a credential: the
// desktop keeps the sign-in and gives back only whose account it is and the
// calendars it holds.

export const providerNames: Record<CalendarAccountProvider, string> = {
  google: "Google",
  microsoft: "Microsoft",
};

/** The desktop opened a sign-in in the browser, which can now be cancelled. */
export const calendarSignInOpenedEvent = "zeitboard:calendar-sign-in-opened";

export interface CalendarAccount {
  accountId: string;
  provider: CalendarAccountProvider;
  label: string;
  connectedLabel: string;
  /** How many of its calendars ZeitBoard keeps. */
  calendars: number;
  /** Accepted times are written to one of its calendars (ADR-0065). */
  writing: boolean;
  /** Set when the provider ended the sign-in; signing in again restores it. */
  problem?: string;
}

/** One calendar an account holds; sourceId is set once it is brought in. */
export interface AccountCalendarChoice {
  calendarId: string;
  name: string;
  primary: boolean;
  sourceId?: string;
}

export interface CalendarAccountCalendars {
  account: CalendarAccount;
  calendars: AccountCalendarChoice[];
}

export interface CalendarAccountInput {
  provider: CalendarAccountProvider;
  clientId: string;
  clientSecret: string;
}

export interface AccountCalendarInput {
  accountId: string;
  calendarId: string;
  label: string;
  zoneId: string;
}

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function text(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value : undefined;
}

export function normalizeCalendarAccount(value: unknown): CalendarAccount | undefined {
  if (!isRecord(value)) return undefined;
  const accountId = text(value.accountId);
  const provider = accountProvider(value.provider);
  const label = text(value.label);
  const calendars = value.calendars;
  if (!accountId || !provider || !label) return undefined;
  if (typeof calendars !== "number" || !Number.isInteger(calendars) || calendars < 0) {
    return undefined;
  }
  const problem = text(value.problem);
  return {
    accountId,
    provider,
    label,
    connectedLabel: text(value.connectedLabel) ?? "",
    calendars,
    writing: value.writing === true,
    ...(problem ? { problem } : {}),
  };
}

function normalizeChoice(value: unknown): AccountCalendarChoice | undefined {
  if (!isRecord(value)) return undefined;
  const calendarId = text(value.calendarId);
  const name = text(value.name);
  if (!calendarId || !name || typeof value.primary !== "boolean") return undefined;
  const sourceId = text(value.sourceId);
  return { calendarId, name, primary: value.primary, ...(sourceId ? { sourceId } : {}) };
}

export function normalizeAccountCalendars(value: unknown): CalendarAccountCalendars | undefined {
  if (!isRecord(value) || !Array.isArray(value.calendars)) return undefined;
  const account = normalizeCalendarAccount(value.account);
  const calendars = value.calendars.map(normalizeChoice);
  if (!account || calendars.some((calendar) => !calendar)) return undefined;
  return { account, calendars: calendars as AccountCalendarChoice[] };
}

/** Whether this window can sign in to an account: only the desktop app can. */
export function canConnectCalendarAccounts(root: WailsRoot = globalThis as unknown as WailsRoot) {
  return findWailsMethod(root, ["ConnectCalendarAccount"]) !== undefined;
}

async function call(name: string, root: WailsRoot, input?: unknown): Promise<unknown> {
  const method = findWailsMethod(root, [name]);
  if (!method) throw new Error("Calendar accounts need the ZeitBoard desktop app.");
  return method(input);
}

async function accountCalendars(
  name: string,
  input: unknown,
  root: WailsRoot,
): Promise<CalendarAccountCalendars> {
  const result = normalizeAccountCalendars(await call(name, root, input));
  if (!result) throw new Error("The account's calendars could not be read.");
  return result;
}

/**
 * Opens the provider's sign-in in the browser and waits, for up to five
 * minutes, for it to come back; then lists the calendars the account holds.
 */
export function connectCalendarAccount(
  input: CalendarAccountInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return accountCalendars("ConnectCalendarAccount", input, root);
}

/** Signs in again with the registration an account was kept with. */
export function reconnectCalendarAccount(
  accountId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return accountCalendars("ReconnectCalendarAccount", { accountId }, root);
}

export async function cancelCalendarSignIn(root: WailsRoot = globalThis as unknown as WailsRoot) {
  await call("CancelCalendarSignIn", root);
}

export async function loadCalendarAccounts(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<CalendarAccount[]> {
  const method = findWailsMethod(root, ["GetCalendarAccounts"]);
  if (!method) return [];
  const value = await method();
  const accounts = Array.isArray(value) ? value.map(normalizeCalendarAccount) : undefined;
  if (!accounts || accounts.some((account) => !account)) {
    throw new Error("The signed-in accounts could not be read.");
  }
  return accounts as CalendarAccount[];
}

export function listAccountCalendars(
  accountId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return accountCalendars("ListAccountCalendars", { accountId }, root);
}

async function importCall(
  name: string,
  input: AccountCalendarInput,
  root: WailsRoot,
): Promise<CalendarImportReport> {
  const report = normalizeCalendarImport(await call(name, root, input));
  if (!report) throw new Error("The calendar could not be read.");
  return report;
}

export function previewAccountCalendar(
  input: AccountCalendarInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return importCall("PreviewAccountCalendar", input, root);
}

/** Brings a calendar in from an account, to be read again every hour. */
export function addAccountCalendar(
  input: AccountCalendarInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return importCall("AddAccountCalendar", input, root);
}

/** Removes an account and its calendars; resolves to what became of its grant. */
export async function removeCalendarAccount(
  accountId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<string> {
  const value = await call("RemoveCalendarAccount", root, { accountId, confirmation: "REMOVE" });
  if (!isRecord(value) || !text(value.message))
    throw new Error("The account could not be removed.");
  return value.message as string;
}

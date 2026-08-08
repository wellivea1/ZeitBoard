import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// Owner-side share links (roadmap slice 12a). The portal runs on the user's own
// server, so everything here goes through backend sync rather than the local
// store — and a desktop with sync off genuinely has no links, which is a
// different statement from "sharing is not built".

export type ShareLinksStatus = "off" | "unavailable" | "ok" | "error";

export interface ShareGrants {
  wakingWindows: boolean;
  allowRequests: boolean;
  allowMessages: boolean;
}

export interface ShareAccessEntry {
  event: string;
  label: string;
  count: number;
  lastLabel?: string;
}

export interface ShareLink {
  profileId: string;
  label: string;
  state: string;
  stateLabel: string;
  createdLabel: string;
  expiresLabel: string;
  grants: ShareGrants;
  grantSummary: string;
  access: ShareAccessEntry[];
}

export interface ShareLinksData {
  status: ShareLinksStatus;
  message?: string;
  disclosure: string;
  links: ShareLink[];
  minPasscodeLength: number;
  maxDays: number;
}

export interface CreateShareLinkInput {
  label: string;
  passcode: string;
  expiresInDays: number;
  grants: ShareGrants;
}

/**
 * The created link's address is returned exactly once. The server keeps only a
 * hash of it, so no later call can produce it again — which is the whole reason
 * a stolen database yields no working links, and the reason the screen has to
 * say so at the moment it is shown.
 */
export interface CreatedShareLink {
  status: "ok" | "error";
  message?: string;
  linkUrl?: string;
  expiresLabel?: string;
  disclosure?: string;
  links: ShareLinksData;
}

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function int(value: unknown): number | undefined {
  return typeof value === "number" && Number.isInteger(value) ? value : undefined;
}

const STATUSES: readonly ShareLinksStatus[] = ["off", "unavailable", "ok", "error"];

export const shareLinksUnavailable: ShareLinksData = {
  status: "off",
  message: "Open the ZeitBoard desktop app to manage share links.",
  disclosure: "",
  links: [],
  minPasscodeLength: 6,
  maxDays: 90,
};

function normalizeGrants(value: unknown): ShareGrants {
  const record = isRecord(value) ? value : {};
  return {
    wakingWindows: record.wakingWindows === true,
    allowRequests: record.allowRequests === true,
    allowMessages: record.allowMessages === true,
  };
}

function normalizeLink(value: unknown): ShareLink | undefined {
  if (!isRecord(value)) return undefined;
  const profileId = str(value.profileId);
  const label = str(value.label);
  const state = str(value.state);
  const stateLabel = str(value.stateLabel);
  const createdLabel = str(value.createdLabel);
  const grantSummary = str(value.grantSummary);
  if (!profileId || !label || !state || !stateLabel || !createdLabel || !grantSummary) {
    return undefined;
  }
  const access: ShareAccessEntry[] = [];
  for (const item of Array.isArray(value.access) ? value.access : []) {
    if (!isRecord(item)) return undefined;
    const event = str(item.event);
    const entryLabel = str(item.label);
    const count = int(item.count);
    if (!event || !entryLabel || count === undefined) return undefined;
    const lastLabel = str(item.lastLabel);
    access.push({ event, label: entryLabel, count, ...(lastLabel ? { lastLabel } : {}) });
  }
  return {
    profileId,
    label,
    state,
    stateLabel,
    createdLabel,
    expiresLabel: str(value.expiresLabel) ?? "",
    grants: normalizeGrants(value.grants),
    grantSummary,
    access,
  };
}

export function normalizeShareLinks(value: unknown): ShareLinksData | undefined {
  if (!isRecord(value)) return undefined;
  const status = STATUSES.find((candidate) => candidate === value.status);
  // The list is required, not optional. Defaulting a missing one to empty would
  // render "nothing is being shared" over a server that is in fact sharing —
  // the one direction this screen must never be wrong in.
  if (!status || !Array.isArray(value.links)) return undefined;
  const links: ShareLink[] = [];
  for (const item of value.links) {
    const link = normalizeLink(item);
    if (!link) return undefined;
    links.push(link);
  }
  const message = str(value.message);
  return {
    status,
    disclosure: str(value.disclosure) ?? "",
    links,
    minPasscodeLength: int(value.minPasscodeLength) ?? 6,
    maxDays: int(value.maxDays) ?? 90,
    ...(message ? { message } : {}),
  };
}

export function normalizeCreatedShareLink(value: unknown): CreatedShareLink | undefined {
  if (!isRecord(value)) return undefined;
  const status = value.status === "ok" || value.status === "error" ? value.status : undefined;
  const links = normalizeShareLinks(value.links);
  if (!status || !links) return undefined;
  const message = str(value.message);
  const linkUrl = str(value.linkUrl);
  const expiresLabel = str(value.expiresLabel);
  const disclosure = str(value.disclosure);
  return {
    status,
    links,
    ...(message ? { message } : {}),
    ...(linkUrl ? { linkUrl } : {}),
    ...(expiresLabel ? { expiresLabel } : {}),
    ...(disclosure ? { disclosure } : {}),
  };
}

export async function loadShareLinks(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ShareLinksData> {
  const method = findWailsMethod(root, ["GetBackendShareLinks"]);
  if (!method) return shareLinksUnavailable;
  try {
    const normalized = normalizeShareLinks(await method());
    if (normalized) return normalized;
  } catch {
    // fall through to the read-only state
  }
  return shareLinksUnavailable;
}

export async function createShareLink(
  input: CreateShareLinkInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<CreatedShareLink> {
  const method = findWailsMethod(root, ["CreateBackendShareLink"]);
  if (!method) throw new Error("Share links need the ZeitBoard desktop app.");
  const normalized = normalizeCreatedShareLink(await method(input));
  if (!normalized) throw new Error("The link could not be created.");
  return normalized;
}

async function act(
  names: readonly string[],
  input: { profileId: string; confirmation?: string },
  root: WailsRoot,
): Promise<ShareLinksData> {
  const method = findWailsMethod(root, names);
  if (!method) throw new Error("Share links need the ZeitBoard desktop app.");
  const normalized = normalizeShareLinks(await method(input));
  if (!normalized) throw new Error("The link list could not be read back.");
  return normalized;
}

export function revokeShareLink(
  profileId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ShareLinksData> {
  return act(["RevokeBackendShareLink"], { profileId }, root);
}

export function eraseShareLink(
  profileId: string,
  confirmation: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ShareLinksData> {
  return act(["EraseBackendShareLink"], { profileId, confirmation }, root);
}

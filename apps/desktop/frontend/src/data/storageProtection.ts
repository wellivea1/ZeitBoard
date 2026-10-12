import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// What actually protects the files on this computer, read back from the
// operating system rather than assumed from how they were created.
//
// The distinction this module has to keep visible: an owner-only permission is
// not encryption. It stops another account on the same machine; it does not
// stop anyone who reads the disk from somewhere else. Saying "protected" and
// leaving the reader to guess which is what put a false claim in privacy.md.

export interface StorageProtectionFile {
  name: string;
  ownerOnly: boolean;
  inherited: boolean;
  /** Windows encrypts it (ADR-0063), as read back from Windows. */
  encrypted: boolean;
  note?: string;
}

/** The choice to have Windows encrypt ZeitBoard's folder, and how far it went. */
export interface StorageEncryption {
  supported: boolean;
  chosen: boolean;
  encryptedFiles: number;
  files: number;
  /** The files do not match the choice yet; the next start puts it right. */
  pending: boolean;
  problem?: string;
}

export interface StorageProtection {
  state: "ok" | "at_risk" | "unknown";
  headline: string;
  detail: string;
  files: StorageProtectionFile[];
  encryption: StorageEncryption;
}

const noEncryption: StorageEncryption = {
  supported: false,
  chosen: false,
  encryptedFiles: 0,
  files: 0,
  pending: false,
};

export const storageProtectionUnavailable: StorageProtection = {
  state: "unknown",
  headline: "Open the ZeitBoard desktop app to check local file permissions.",
  detail: "This browser preview has no local files to inspect.",
  files: [],
  encryption: noEncryption,
};

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function files(value: unknown): StorageProtectionFile[] {
  if (!Array.isArray(value)) return [];
  const parsed: StorageProtectionFile[] = [];
  for (const entry of value) {
    if (!isRecord(entry)) continue;
    const name = str(entry.name);
    if (!name) continue;
    const note = str(entry.note);
    parsed.push({
      name,
      // Absent reads as not protected. An unknown permission is not a good one.
      ownerOnly: entry.ownerOnly === true,
      inherited: entry.inherited === true,
      encrypted: entry.encrypted === true,
      ...(note ? { note } : {}),
    });
  }
  return parsed;
}

function count(value: unknown): number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : 0;
}

function encryption(value: unknown): StorageEncryption {
  if (!isRecord(value)) return noEncryption;
  const problem = str(value.problem);
  const files = count(value.files);
  return {
    supported: value.supported === true,
    chosen: value.chosen === true,
    // More encrypted than there are files cannot be, so it reads as none.
    encryptedFiles: count(value.encryptedFiles) <= files ? count(value.encryptedFiles) : 0,
    files,
    pending: value.pending === true,
    ...(problem ? { problem } : {}),
  };
}

export function normalizeStorageProtection(value: unknown): StorageProtection | undefined {
  if (!isRecord(value)) return undefined;
  const state =
    value.state === "ok" || value.state === "at_risk" || value.state === "unknown"
      ? value.state
      : undefined;
  const headline = str(value.headline);
  const detail = str(value.detail);
  if (!state || !headline || !detail) return undefined;
  return {
    state,
    headline,
    detail,
    files: files(value.files),
    encryption: encryption(value.encryption),
  };
}

export async function loadStorageProtection(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<StorageProtection> {
  const method = findWailsMethod(root, ["GetStorageProtection"]);
  if (!method) return storageProtectionUnavailable;
  try {
    const normalized = normalizeStorageProtection(await method());
    if (normalized) return normalized;
  } catch {
    // fall through
  }
  return storageProtectionUnavailable;
}

/** Saves the choice to have Windows encrypt ZeitBoard's files (ADR-0063). */
export async function setStorageEncryption(
  encrypt: boolean,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<StorageProtection> {
  const method = findWailsMethod(root, ["SetStorageEncryption"]);
  if (!method) throw new Error("Encrypting files needs the ZeitBoard desktop app.");
  const report = normalizeStorageProtection(await method({ encrypt }));
  if (!report) throw new Error("ZeitBoard's files could not be checked.");
  return report;
}

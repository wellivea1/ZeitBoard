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
  note?: string;
}

export interface StorageProtection {
  state: "ok" | "at_risk" | "unknown";
  headline: string;
  detail: string;
  files: StorageProtectionFile[];
}

export const storageProtectionUnavailable: StorageProtection = {
  state: "unknown",
  headline: "Open the ZeitBoard desktop app to check local file permissions.",
  detail: "This browser preview has no local files to inspect.",
  files: [],
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
      ...(note ? { note } : {}),
    });
  }
  return parsed;
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
  return { state, headline, detail, files: files(value.files) };
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

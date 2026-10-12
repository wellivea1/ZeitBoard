import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// ZeitBoard packaged for Claude (ADR-0061): the desktop writes a Claude
// Desktop extension and a skill where the owner chooses. Neither holds data.

export interface SavedPackage {
  /** The file written, or empty when nothing was saved. */
  fileName: string;
  saved: boolean;
}

type PackageMethod = "SaveClaudeExtension" | "SaveClaudeSkill";

export function canSaveClaudePackages(root: WailsRoot = globalThis as unknown as WailsRoot) {
  return findWailsMethod(root, ["SaveClaudeExtension"]) !== undefined;
}

async function save(name: PackageMethod, root: WailsRoot): Promise<SavedPackage> {
  const method = findWailsMethod(root, [name]);
  if (!method) throw new Error("Saving it needs the ZeitBoard desktop app.");
  const value = await method();
  if (typeof value !== "object" || value === null) throw new Error("It could not be saved.");
  const record = value as Record<string, unknown>;
  if (record.saved !== true) return { fileName: "", saved: false };
  if (typeof record.fileName !== "string" || !record.fileName) {
    throw new Error("It could not be saved.");
  }
  return { fileName: record.fileName, saved: true };
}

export function saveClaudeExtension(root: WailsRoot = globalThis as unknown as WailsRoot) {
  return save("SaveClaudeExtension", root);
}

export function saveClaudeSkill(root: WailsRoot = globalThis as unknown as WailsRoot) {
  return save("SaveClaudeSkill", root);
}

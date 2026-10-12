import { afterEach, describe, expect, it, vi } from "vitest";
import {
  loadAccountActivity,
  noAccountActivity,
  normalizeAccountActivity,
  previewActivityExport,
  removeAccountActivity,
} from "./accountActivity";

const figure = {
  nights: 12,
  spanLabel: "Aug 31, 2026 to Sep 13, 2026",
  zoneId: "UTC",
  firstDate: "2026-08-31",
  days: 14,
  suggested: [
    [0, 1410, 1440],
    [1, 0, 500],
  ],
  recorded: [[7, 300, 780]],
  agreement: { recorded: 6, matched: 6, summary: "On those nights, suggestions typically began…" },
};

const kept = { imported: true, minutes: 4210, importedLabel: "Sep 20, 2026", figure };

afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

describe("account activity", () => {
  it("reads kept activity and the nights it suggests", () => {
    const activity = normalizeAccountActivity(kept);
    expect(activity?.minutes).toBe(4210);
    expect(activity?.figure?.suggested).toEqual([
      [0, 1410, 1440],
      [1, 0, 500],
    ]);
    expect(activity?.figure?.agreement).toEqual(figure.agreement);
  });

  it("refuses kept activity without a figure it could draw", () => {
    expect(normalizeAccountActivity({ ...kept, figure: undefined })).toBeUndefined();
    expect(
      normalizeAccountActivity({ ...kept, figure: { ...figure, recorded: [[14, 0, 60]] } }),
    ).toBeUndefined();
  });

  it("is none outside the desktop app", async () => {
    await expect(loadAccountActivity()).resolves.toBe(noAccountActivity);
  });

  it("reads a closed dialog as no choice", async () => {
    (globalThis as unknown as { go?: unknown }).go = {
      main: { App: { PreviewActivityExport: async () => ({ canceled: true }) } },
    };
    await expect(previewActivityExport()).resolves.toBeUndefined();
  });

  it("removes with the word typed back", async () => {
    const remove = vi.fn(async () => ({ imported: false }));
    (globalThis as unknown as { go?: unknown }).go = {
      main: { App: { RemoveAccountActivity: remove } },
    };
    await expect(removeAccountActivity("REMOVE")).resolves.toBe(noAccountActivity);
    expect(remove).toHaveBeenCalledWith({ confirmation: "REMOVE" });
  });
});

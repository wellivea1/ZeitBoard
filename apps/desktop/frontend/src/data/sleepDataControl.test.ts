import { describe, expect, it, vi } from "vitest";

import { saveSleepDataExport } from "./sleepDataControl";

describe("saveSleepDataExport", () => {
  it("uses the native bounded-summary path without requesting export JSON", async () => {
    const save = vi.fn(async () => ({
      fileName: "sleep.json",
      generatedLabel: "Jul 27, 2026, 9:00 PM",
      observationCount: 4,
      correctionCount: 2,
      preview: '{"schema_version":"v1"}',
      previewTruncated: true,
      saved: true,
      canceled: false,
    }));
    const legacy = vi.fn();
    const result = await saveSleepDataExport({
      go: { main: { App: { SaveSleepDataExport: save, ExportSleepData: legacy } } },
    });

    expect(result.saved).toBe(true);
    expect(result.preview).toHaveLength(23);
    expect(save).toHaveBeenCalledTimes(1);
    expect(legacy).not.toHaveBeenCalled();
  });

  it("rejects contradictory native save state", async () => {
    await expect(
      saveSleepDataExport({
        go: {
          main: {
            App: {
              SaveSleepDataExport: async () => ({
                fileName: "sleep.json",
                generatedLabel: "now",
                observationCount: 0,
                correctionCount: 0,
                preview: "{}",
                previewTruncated: false,
                saved: true,
                canceled: true,
              }),
            },
          },
        },
      }),
    ).rejects.toThrow(/invalid summary/i);
  });
});

import { describe, expect, it, vi } from "vitest";

import {
  downloadMedicationClinicalReport,
  exportMedicationClinicalReport,
  hasLocalMedicationReportService,
  printMedicationClinicalReport,
  loadMedicationClinicalReport,
  normalizeMedicationClinicalReport,
} from "./medicationReport";
import {
  asDoublePlot,
  medicationClinicalDoublePlotFixture,
  medicationClinicalReportExportFixture,
  medicationClinicalReportFixture,
  medicationClinicalReportInput,
} from "../test/medicationReportFixture";

function reportRoot(report = medicationClinicalReportFixture()) {
  return {
    go: {
      main: {
        App: {
          GetMedicationClinicianReport: vi.fn(async () => structuredClone(report)),
          ExportMedicationClinicianReport: vi.fn(async () =>
            structuredClone(medicationClinicalReportExportFixture()),
          ),
        },
      },
    },
  };
}

describe("medication clinician report adapter", () => {
  it("normalizes a coherent real-data report and exact-present legend", () => {
    const normalized = normalizeMedicationClinicalReport(medicationClinicalReportFixture());

    expect(normalized).toMatchObject({
      status: "partial",
      range: { dayStartHour: 18 },
      summary: {
        calendarRows: 32,
        observedSleepSegments: 31,
        recordedScheduled: 2,
        recordedTaken: 1,
        recordedSkipped: 1,
      },
      adherence: [{ medicationLabel: "Medication 1", asNeeded: 0 }],
    });
    expect(normalized?.actogram.legend.map((item) => item.kind)).toEqual([
      "sleep_observed",
      "medication_taken",
      "medication_skipped",
      "medication_start",
      "context_forced_schedule",
    ]);
  });

  it("rejects contradictory counts, legends, ranges, zones, and required redactions", () => {
    const countMismatch = medicationClinicalReportFixture();
    countMismatch.summary.observedSleepSegments = 30;
    expect(normalizeMedicationClinicalReport(countMismatch)).toBeUndefined();

    const adherenceMismatch = medicationClinicalReportFixture();
    adherenceMismatch.adherence[0]!.taken = 2;
    expect(normalizeMedicationClinicalReport(adherenceMismatch)).toBeUndefined();

    const legendMismatch = medicationClinicalReportFixture();
    legendMismatch.actogram.legend.pop();
    expect(normalizeMedicationClinicalReport(legendMismatch)).toBeUndefined();

    const missingRedaction = medicationClinicalReportFixture();
    missingRedaction.redactions = missingRedaction.redactions.filter(
      (item) => item !== "Personal diagnostic information omitted",
    );
    expect(normalizeMedicationClinicalReport(missingRedaction)).toBeUndefined();

    const unordered = medicationClinicalReportFixture();
    unordered.actogram.rows[1]!.civilDate = unordered.actogram.rows[0]!.civilDate;
    expect(normalizeMedicationClinicalReport(unordered)).toBeUndefined();

    const overflowingSegment = medicationClinicalReportFixture();
    overflowingSegment.actogram.rows[0]!.sleep[0]!.startPercent = 90;
    overflowingSegment.actogram.rows[0]!.sleep[0]!.widthPercent = 20;
    expect(normalizeMedicationClinicalReport(overflowingSegment)).toBeUndefined();

    const contextMismatch = medicationClinicalReportFixture();
    contextMismatch.summary.rhythmContextMarkers = 2;
    expect(normalizeMedicationClinicalReport(contextMismatch)).toBeUndefined();

    const driftOutsideBounds = medicationClinicalReportFixture();
    driftOutsideBounds.drift.points[0]!.onsetHour = 30;
    expect(normalizeMedicationClinicalReport(driftOutsideBounds)).toBeUndefined();
  });

  it("reads a double plot, counting each day once", () => {
    const normalized = normalizeMedicationClinicalReport(medicationClinicalDoublePlotFixture(3));
    expect(normalized).toMatchObject({
      range: { orientation: "48h" },
      actogram: { orientation: "48h" },
      summary: { observedSleepSegments: 2, medicationEvents: 2, rhythmContextMarkers: 1 },
    });
    expect(normalized?.actogram.axisLabels).toHaveLength(9);
    // Row one's right half is row two's night, marked as drawn again.
    expect(normalized?.actogram.rows[0]?.sleep.map((segment) => segment.repeat ?? false)).toEqual([
      false,
      true,
    ]);

    // A day with no sleep may still show the next day's on its right half.
    const withGap = medicationClinicalReportFixture(3);
    withGap.actogram.rows[0]!.sleep = [];
    withGap.actogram.rows[0]!.noData = true;
    withGap.summary.observedSleepSegments -= 1;
    withGap.summary.noDataRows += 1;
    expect(normalizeMedicationClinicalReport(asDoublePlot(withGap))).toBeDefined();

    const wrongAxis = medicationClinicalDoublePlotFixture(3);
    wrongAxis.actogram.axisLabels = wrongAxis.actogram.axisLabels.slice(0, 5);
    expect(normalizeMedicationClinicalReport(wrongAxis)).toBeUndefined();

    const mixed = medicationClinicalDoublePlotFixture(3);
    mixed.range.orientation = "24h";
    expect(normalizeMedicationClinicalReport(mixed)).toBeUndefined();
  });

  it("prints from a frame that runs nothing, and removes the frame once printed", () => {
    const value = medicationClinicalReportExportFixture();
    expect(printMedicationClinicalReport(value)).toBe(true);
    const frame = document.querySelector<HTMLIFrameElement>("iframe[data-report-print]")!;
    expect(frame.getAttribute("sandbox")).toBe("allow-same-origin allow-modals");
    expect(frame.getAttribute("aria-hidden")).toBe("true");
    expect(frame.srcdoc).toBe(value.html);

    const print = vi.fn();
    Object.defineProperty(frame.contentWindow!, "print", { configurable: true, value: print });
    Object.defineProperty(frame.contentWindow!, "focus", { configurable: true, value: vi.fn() });
    frame.dispatchEvent(new Event("load"));
    expect(print).toHaveBeenCalledTimes(1);

    // A second request replaces a frame still waiting, rather than stacking.
    expect(printMedicationClinicalReport(value)).toBe(true);
    expect(document.querySelectorAll("iframe[data-report-print]")).toHaveLength(1);
    const next = document.querySelector<HTMLIFrameElement>("iframe[data-report-print]")!;
    Object.defineProperty(next.contentWindow!, "print", { configurable: true, value: vi.fn() });
    Object.defineProperty(next.contentWindow!, "focus", { configurable: true, value: vi.fn() });
    next.dispatchEvent(new Event("load"));
    next.contentWindow!.dispatchEvent(new Event("afterprint"));
    expect(document.querySelector("iframe[data-report-print]")).toBeNull();
  });

  it("loads and exports only through the local desktop bridge", async () => {
    const root = reportRoot();

    expect(hasLocalMedicationReportService(root)).toBe(true);
    await expect(
      loadMedicationClinicalReport(medicationClinicalReportInput, root),
    ).resolves.toMatchObject({ summary: { calendarRows: 32 } });
    await expect(
      exportMedicationClinicalReport(medicationClinicalReportInput, "EXPORT", root),
    ).resolves.toMatchObject({ rowCount: 32, eventCount: 2 });
    expect(root.go.main.App.GetMedicationClinicianReport).toHaveBeenCalledWith(
      medicationClinicalReportInput,
    );
    expect(root.go.main.App.ExportMedicationClinicianReport).toHaveBeenCalledWith({
      report: medicationClinicalReportInput,
      confirmation: "EXPORT",
    });
    expect(downloadMedicationClinicalReport(medicationClinicalReportExportFixture())).toBe(false);
  });

  it("fails closed when the service or export payload is incomplete", async () => {
    expect(hasLocalMedicationReportService({})).toBe(false);
    await expect(loadMedicationClinicalReport(medicationClinicalReportInput, {})).rejects.toThrow(
      /desktop service/,
    );

    const root = reportRoot();
    root.go.main.App.ExportMedicationClinicianReport.mockResolvedValue({
      ...medicationClinicalReportExportFixture(),
      html: "<!doctype html><script>unsafe()</script>",
    });
    await expect(
      exportMedicationClinicalReport(medicationClinicalReportInput, "EXPORT", root),
    ).rejects.toThrow(/invalid response/);
  });
});

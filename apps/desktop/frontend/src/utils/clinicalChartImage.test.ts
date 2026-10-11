import { describe, expect, it, vi } from "vitest";

import {
  medicationClinicalDoublePlotFixture,
  medicationClinicalReportFixture,
} from "../test/medicationReportFixture";
import {
  clinicalChartFileName,
  clinicalChartLayout,
  maxChartRows,
  paintClinicalChart,
  type ChartContext,
  type ChartShape,
} from "./clinicalChartImage";

// A fixed-width stand-in for a canvas's measureText.
const measure = (text: string) => text.length * 7;

const texts = (shapes: ChartShape[]) =>
  shapes.filter((shape): shape is Extract<ChartShape, { type: "text" }> => shape.type === "text");

describe("the clinical chart as an image", () => {
  it("draws each row's sleep and timing where the preview does, in the printed colours", () => {
    const report = medicationClinicalReportFixture(4);
    const layout = clinicalChartLayout(report, measure);
    const trackX = 40 + 112;
    const trackWidth = 1280 - 40 - trackX;

    // The observed night, a sixth of the way in and a third of the day long.
    const sleep = layout.shapes.filter(
      (shape) => shape.type === "rect" && shape.fill === "#315e91" && shape.h === 8,
    );
    expect(sleep).toHaveLength(3); // the fourth row has no data
    expect(sleep[0]).toMatchObject({
      x: trackX + 0.1667 * trackWidth,
      w: 0.3333 * trackWidth,
    });
    // Each recorded mark keeps its own look: taken, started, skipped (dashed), context.
    const markAt = (percent: number) =>
      layout.shapes.find(
        (shape) =>
          shape.type === "line" &&
          shape.x1 === trackX + (percent / 100) * trackWidth &&
          shape.y2 - shape.y1 === 14,
      );
    expect(markAt(25)).toMatchObject({ stroke: "#255b46", width: 1.5 });
    expect(markAt(30)).toMatchObject({ stroke: "#4e3e78", width: 2 });
    expect(markAt(20)).toMatchObject({ stroke: "#745b28", dash: [3, 2] });
    expect(markAt(40)).toMatchObject({ stroke: "#7a4d3a" });

    const words = texts(layout.shapes).map((shape) => shape.text);
    expect(words).toContain("Sleep and recorded timing");
    expect(words).toContain("June 2026");
    expect(words).toContain("no data");
    expect(words).toContain(report.actogram.axisLabels[0]);
    // The report's own boundary travels with the image.
    expect(words.join(" ")).toContain(report.notice.split(" ").slice(0, 4).join(" "));
    expect(words.join(" ")).toContain(`Generated ${report.generatedLabel} by ZeitBoard.`);
    expect(layout.rowsDrawn).toBe(4);
    expect(layout.height).toBeGreaterThan(4 * 18);
  });

  it("marks a double plot's middle and keeps the newest rows of a long range", () => {
    const double = clinicalChartLayout(medicationClinicalDoublePlotFixture(3), measure);
    const middle = 152 + (1280 - 40 - 152) / 2;
    expect(
      double.shapes.some(
        (shape) => shape.type === "line" && shape.x1 === middle && shape.stroke === "#9ba7a1",
      ),
    ).toBe(true);

    const long = medicationClinicalReportFixture(maxChartRows + 20);
    const layout = clinicalChartLayout(long, measure);
    expect(layout.rowsDrawn).toBe(maxChartRows);
    expect(layout.rowsTotal).toBe(maxChartRows + 20);
    const words = texts(layout.shapes).map((shape) => shape.text);
    expect(words).toContain(long.actogram.rows.at(-1)!.dayLabel);
    expect(words.join(" ")).toContain(`The newest ${maxChartRows} of ${maxChartRows + 20} days`);
  });

  it("paints every shape onto white paper at the scale asked", () => {
    const layout = clinicalChartLayout(medicationClinicalReportFixture(3), measure);
    const calls: string[] = [];
    const context = {
      fillStyle: "",
      strokeStyle: "",
      lineWidth: 1,
      font: "",
      textAlign: "left",
      scale: vi.fn((x: number) => calls.push(`scale ${x}`)),
      fillRect: vi.fn(() => calls.push("fillRect")),
      strokeRect: vi.fn(() => calls.push("strokeRect")),
      beginPath: vi.fn(),
      moveTo: vi.fn(),
      lineTo: vi.fn(),
      stroke: vi.fn(() => calls.push("stroke")),
      fillText: vi.fn(() => calls.push("fillText")),
      setLineDash: vi.fn(),
    } as unknown as ChartContext;
    paintClinicalChart(context, layout, 2);

    expect(calls[0]).toBe("scale 2");
    expect(context.fillRect).toHaveBeenNthCalledWith(1, 0, 0, layout.width, layout.height);
    const count = (type: ChartShape["type"]) =>
      layout.shapes.filter((shape) => shape.type === type).length;
    expect(calls.filter((call) => call === "fillText")).toHaveLength(count("text"));
    expect(calls.filter((call) => call === "stroke")).toHaveLength(count("line"));
    // The page is left without a dash set.
    expect(context.setLineDash).toHaveBeenLastCalledWith([]);
  });

  it("names the file by the report's range", () => {
    expect(clinicalChartFileName(medicationClinicalReportFixture())).toBe(
      "zeitboard-clinician-chart-2026-06-20-to-2026-07-21.png",
    );
  });
});

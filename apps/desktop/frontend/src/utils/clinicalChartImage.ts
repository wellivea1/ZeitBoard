import type {
  MedicationClinicalReport,
  MedicationClinicalActogramRow,
} from "../data/medicationReport";

// The clinical chart as a PNG, for a clinician who takes an image rather than
// a printed page (limitation 10). It is drawn from the same report the preview
// and the printed page show, in the printed page's colours, with the report's
// own interpretation boundary beneath it. Layout is worked out first, as plain
// shapes, so it is tested without a canvas; painting only replays the shapes.

export type ChartShape =
  | {
      type: "rect";
      x: number;
      y: number;
      w: number;
      h: number;
      fill?: string;
      stroke?: string;
      dash?: number[];
    }
  | {
      type: "line";
      x1: number;
      y1: number;
      x2: number;
      y2: number;
      stroke: string;
      width: number;
      dash?: number[];
    }
  | {
      type: "text";
      x: number;
      y: number;
      text: string;
      font: string;
      fill: string;
      align: "left" | "center" | "right";
    };

export interface ChartLayout {
  width: number;
  height: number;
  shapes: ChartShape[];
  /** How many rows were drawn, and how many the report has. */
  rowsDrawn: number;
  rowsTotal: number;
}

/** Measures text in a font, as a canvas does. */
export type MeasureText = (text: string, font: string) => number;

// The printed report's palette (medication_report_html.go), so the image and
// the page read alike.
const ink = "#17211c";
const muted = "#5b6862";
const rule = "#d8ddda";
const track = "#9ba7a1";
const grid = "#d9dfdc";
const weekend = "#f4f6f5";
const paper = "#ffffff";

const sleepStyles: Record<string, { fill: string; stroke?: string }> = {
  sleep_observed: { fill: "#315e91" },
  sleep_inferred: { fill: "#9eb4c8", stroke: "#315e91" },
  sleep_nap: { fill: "#6f8fae" },
  forecast: { fill: "#d7e1ea", stroke: "#758b9f" },
};

interface MarkStyle {
  stroke: string;
  width: number;
  dash?: number[];
}

const markStyles: Record<string, MarkStyle> = {
  medication_taken: { stroke: "#255b46", width: 1.5 },
  medication_skipped: { stroke: "#745b28", width: 1.5, dash: [3, 2] },
  medication_start: { stroke: "#4e3e78", width: 2 },
};
const contextMark: MarkStyle = { stroke: "#7a4d3a", width: 1.5 };

const sans = '"Public Sans", "Segoe UI", Arial, sans-serif';
const font = (size: number, weight = 400) => `${weight} ${size}px ${sans}`;

const width = 1280;
const margin = 40;
const dateColumn = 112;
const rowHeight = 18;
const monthHeight = 28;
/** A canvas taller than this fails in Chromium; the newest rows are kept. */
export const maxChartRows = 1500;

function wrap(text: string, fontSpec: string, maxWidth: number, measure: MeasureText): string[] {
  const lines: string[] = [];
  let line = "";
  for (const word of text.split(/\s+/).filter(Boolean)) {
    const next = line ? `${line} ${word}` : word;
    if (line && measure(next, fontSpec) > maxWidth) {
      lines.push(line);
      line = word;
    } else {
      line = next;
    }
  }
  if (line) lines.push(line);
  return lines;
}

function monthName(row: MedicationClinicalActogramRow) {
  return (
    row.monthLabel ||
    new Intl.DateTimeFormat(undefined, { month: "long", year: "numeric", timeZone: "UTC" }).format(
      new Date(`${row.civilDate}T12:00:00Z`),
    )
  );
}

/** Lays the chart out as shapes, top to bottom. */
export function clinicalChartLayout(
  report: MedicationClinicalReport,
  measure: MeasureText,
): ChartLayout {
  const shapes: ChartShape[] = [];
  const textWidth = width - margin * 2;
  const trackX = margin + dateColumn;
  const trackWidth = width - margin - trackX;
  const double = report.actogram.orientation === "48h";
  let y = margin;
  const text = (
    value: string,
    x: number,
    baseline: number,
    spec: string,
    fill = ink,
    align: "left" | "center" | "right" = "left",
  ) => shapes.push({ type: "text", x, y: baseline, text: value, font: spec, fill, align });
  const paragraph = (value: string, spec: string, size: number, fill = ink) => {
    for (const line of wrap(value, spec, textWidth, measure)) {
      y += size * 1.45;
      text(line, margin, y, spec, fill);
    }
  };

  // What it is, and for when.
  y += 12;
  text("CLINICAL-DAY VIEW", margin, y, font(11, 700), muted);
  y += 30;
  text("Sleep and recorded timing", margin, y, font(22, 700));
  paragraph(
    `${report.range.label} · ${report.range.dayStartLabel} anchor${double ? ", double plot" : ""}`,
    font(13),
    13,
    muted,
  );
  paragraph(report.actogram.summary, font(13), 13);
  y += 18;

  // The hours across the top, each over its rule.
  const labels = report.actogram.axisLabels;
  y += 12;
  labels.forEach((label, index) => {
    const x = trackX + (labels.length < 2 ? 0 : (index / (labels.length - 1)) * trackWidth);
    const align = index === 0 ? "left" : index === labels.length - 1 ? "right" : "center";
    text(label, x, y, font(11), muted, align);
  });
  y += 6;

  const rows = report.actogram.rows.slice(-maxChartRows);
  rows.forEach((row, index) => {
    if (row.monthLabel || index === 0) {
      shapes.push({
        type: "line",
        x1: margin,
        y1: y,
        x2: width - margin,
        y2: y,
        stroke: track,
        width: 1,
      });
      text(monthName(row), margin, y + 19, font(14, 700));
      y += monthHeight;
    }
    const top = y;
    if (row.weekend) {
      shapes.push({
        type: "rect",
        x: margin,
        y: top,
        w: width - margin * 2,
        h: rowHeight,
        fill: weekend,
      });
    }
    shapes.push({
      type: "line",
      x1: margin,
      y1: top,
      x2: width - margin,
      y2: top,
      stroke: rule,
      width: 1,
    });
    text(row.dayLabel, margin + 4, top + 13, font(11));
    // Quarter rules for a day; for a double plot, eighths and a firm middle.
    const fractions = double ? [1, 2, 3, 5, 6, 7].map((n) => n / 8) : [0.25, 0.5, 0.75];
    for (const fraction of fractions) {
      const x = trackX + fraction * trackWidth;
      shapes.push({
        type: "line",
        x1: x,
        y1: top,
        x2: x,
        y2: top + rowHeight,
        stroke: grid,
        width: 1,
      });
    }
    if (double) {
      const middle = trackX + trackWidth / 2;
      shapes.push({
        type: "line",
        x1: middle,
        y1: top,
        x2: middle,
        y2: top + rowHeight,
        stroke: track,
        width: 1,
      });
    }
    shapes.push({
      type: "line",
      x1: trackX,
      y1: top,
      x2: trackX,
      y2: top + rowHeight,
      stroke: track,
      width: 1,
    });
    if (row.noData) text("no data", trackX + 6, top + 13, font(10), muted);
    for (const segment of row.sleep) {
      const style = sleepStyles[segment.kind] ?? { fill: muted };
      shapes.push({
        type: "rect",
        x: trackX + (segment.startPercent / 100) * trackWidth,
        y: top + 5,
        w: Math.max(1, (segment.widthPercent / 100) * trackWidth),
        h: 8,
        fill: style.fill,
        ...(style.stroke ? { stroke: style.stroke, dash: [4, 2] } : {}),
      });
    }
    for (const annotation of row.annotations) {
      const style = markStyles[annotation.kind] ?? contextMark;
      const x = trackX + (annotation.positionPercent / 100) * trackWidth;
      shapes.push({
        type: "line",
        x1: x,
        y1: top + 2,
        x2: x,
        y2: top + rowHeight - 2,
        stroke: style.stroke,
        width: style.width,
        ...(style.dash ? { dash: style.dash } : {}),
      });
    }
    y += rowHeight;
  });
  shapes.push({
    type: "line",
    x1: margin,
    y1: y,
    x2: width - margin,
    y2: y,
    stroke: track,
    width: 1,
  });

  // What each mark is.
  y += 26;
  let x = margin;
  for (const item of report.actogram.legend) {
    const labelWidth = measure(item.label, font(12));
    if (x + 26 + labelWidth > width - margin) {
      x = margin;
      y += 22;
    }
    const sleep = sleepStyles[item.kind];
    if (sleep) {
      shapes.push({
        type: "rect",
        x,
        y: y - 8,
        w: 18,
        h: 7,
        fill: sleep.fill,
        ...(sleep.stroke ? { stroke: sleep.stroke, dash: [4, 2] } : {}),
      });
    } else {
      const mark = markStyles[item.kind] ?? contextMark;
      shapes.push({
        type: "line",
        x1: x + 8,
        y1: y - 11,
        x2: x + 8,
        y2: y + 1,
        stroke: mark.stroke,
        width: mark.width,
        ...(mark.dash ? { dash: mark.dash } : {}),
      });
    }
    text(item.label, x + 24, y, font(12));
    x += 24 + labelWidth + 24;
  }

  // The report's own boundary travels with the image.
  y += 16;
  paragraph(report.notice, font(12), 12, muted);
  const truncated =
    report.actogram.rows.length > rows.length
      ? ` The newest ${rows.length} of ${report.actogram.rows.length} days are drawn.`
      : "";
  paragraph(`Generated ${report.generatedLabel} by ZeitBoard.${truncated}`, font(12), 12, muted);

  return {
    width,
    height: Math.ceil(y + margin),
    shapes,
    rowsDrawn: rows.length,
    rowsTotal: report.actogram.rows.length,
  };
}

/** The part of a canvas context the painter uses. */
export type ChartContext = Pick<
  CanvasRenderingContext2D,
  | "fillStyle"
  | "strokeStyle"
  | "lineWidth"
  | "font"
  | "textAlign"
  | "fillRect"
  | "strokeRect"
  | "beginPath"
  | "moveTo"
  | "lineTo"
  | "stroke"
  | "fillText"
  | "setLineDash"
  | "scale"
>;

/** Replays a layout onto a canvas, on white paper. */
export function paintClinicalChart(context: ChartContext, layout: ChartLayout, scale: number) {
  context.scale(scale, scale);
  context.fillStyle = paper;
  context.fillRect(0, 0, layout.width, layout.height);
  for (const shape of layout.shapes) {
    switch (shape.type) {
      case "rect":
        if (shape.fill) {
          context.fillStyle = shape.fill;
          context.fillRect(shape.x, shape.y, shape.w, shape.h);
        }
        if (shape.stroke) {
          context.setLineDash(shape.dash ?? []);
          context.strokeStyle = shape.stroke;
          context.lineWidth = 1;
          context.strokeRect(shape.x + 0.5, shape.y + 0.5, shape.w - 1, shape.h - 1);
        }
        break;
      case "line":
        context.setLineDash(shape.dash ?? []);
        context.strokeStyle = shape.stroke;
        context.lineWidth = shape.width;
        context.beginPath();
        context.moveTo(shape.x1, shape.y1);
        context.lineTo(shape.x2, shape.y2);
        context.stroke();
        break;
      case "text":
        context.fillStyle = shape.fill;
        context.font = shape.font;
        context.textAlign = shape.align;
        context.fillText(shape.text, shape.x, shape.y);
        break;
    }
  }
  context.setLineDash([]);
}

/** Chromium refuses canvases taller than this, in device pixels. */
const maxCanvasHeight = 32_000;

/** The chart as a PNG, or undefined where there is no canvas to draw it on. */
export async function clinicalChartPNG(
  report: MedicationClinicalReport,
): Promise<Blob | undefined> {
  if (typeof document === "undefined") return undefined;
  // The app's own sans face, once loaded, is what the canvas draws with.
  await document.fonts?.load(font(12)).catch(() => undefined);
  const canvas = document.createElement("canvas");
  const context = canvas.getContext("2d");
  if (!context) return undefined;
  const layout = clinicalChartLayout(report, (value, spec) => {
    context.font = spec;
    return context.measureText(value).width;
  });
  const scale = Math.max(1, Math.min(2, Math.floor(maxCanvasHeight / layout.height)));
  canvas.width = layout.width * scale;
  canvas.height = layout.height * scale;
  paintClinicalChart(context, layout, scale);
  return new Promise((resolve) => canvas.toBlob((blob) => resolve(blob ?? undefined), "image/png"));
}

export function clinicalChartFileName(report: MedicationClinicalReport) {
  return `zeitboard-clinician-chart-${report.range.fromDate}-to-${report.range.toDate}.png`;
}

/** Saves the chart as a PNG download; the file name, or undefined if none was made. */
export async function saveClinicalChartImage(
  report: MedicationClinicalReport,
): Promise<string | undefined> {
  const image = await clinicalChartPNG(report);
  if (!image || typeof URL.createObjectURL !== "function") return undefined;
  const url = URL.createObjectURL(image);
  const link = document.createElement("a");
  link.href = url;
  link.download = clinicalChartFileName(report);
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
  return link.download;
}

import type { AccountActivity, NightSegment } from "../data/accountActivity";
import { appLocale } from "../utils/appLocale";
import { formatCivilDate } from "./timeProbeLogic";

// The geometry of the nights figure (ADR-0059), apart from the component so it
// can be read and tested as arithmetic.

export const DAY = 24 * 60;

/**
 * Rows are 6px high, so twelve weeks read as an actogram, until the figure
 * would pass 960px; then the days share that, and years of them read as a
 * texture whose slant is the drift.
 */
export function figureHeight(days: number) {
  return Math.min(960, Math.max(120, days * 6));
}

/**
 * One path for a kind of night. Row r holds day r on the left and day r + 1 on
 * the right, so each segment is drawn on its own row and the row above. inset
 * is the part of a row left empty above and below the bar.
 */
export function nightPath(segments: readonly NightSegment[], inset = 0) {
  const height = 1 - 2 * inset;
  const parts: string[] = [];
  for (const [day, start, end] of segments) {
    const width = end - start;
    const bar = `h${width}v${height}h${-width}z`;
    parts.push(`M${start} ${day + inset}${bar}`);
    if (day > 0) parts.push(`M${start + DAY} ${day - 1 + inset}${bar}`);
  }
  return parts.join("");
}

export interface RowLabel {
  row: number;
  text: string;
}

const monthYear = new Intl.DateTimeFormat(appLocale, {
  month: "short",
  year: "numeric",
  timeZone: "UTC",
});
const yearOnly = new Intl.DateTimeFormat(appLocale, { year: "numeric", timeZone: "UTC" });

/**
 * The dates down the side, far enough apart to read: every week or two over a
 * few months, the starts of months, quarters or years over longer spans.
 */
export function rowLabels(firstDate: string, days: number, height: number): RowLabel[] {
  const rowHeight = height / days;
  const gap = 26;
  if (days <= 120) {
    const step = [1, 2, 7, 14].find((rows) => rows * rowHeight >= gap) ?? 28;
    const labels: RowLabel[] = [];
    for (let row = 0; row < days; row += step) {
      labels.push({ row, text: formatCivilDate(firstDate, row, false) });
    }
    return labels;
  }
  const months = [1, 2, 3, 6, 12, 24, 60].find((step) => step * 30.44 * rowHeight >= gap) ?? 120;
  const first = new Date(`${firstDate}T00:00:00Z`);
  if (Number.isNaN(first.getTime())) return [];
  const labels: RowLabel[] = [];
  for (let row = 0; row < days; row++) {
    const date = new Date(first.getTime() + row * 86_400_000);
    if (date.getUTCDate() !== 1) continue;
    const month = date.getUTCFullYear() * 12 + date.getUTCMonth();
    if (month % months !== 0) continue;
    labels.push({ row, text: (months >= 12 ? yearOnly : monthYear).format(date) });
  }
  return labels;
}

/** How many days the recent view draws: enough weeks to see a drift. */
export const RECENT_DAYS = 84;

/** The last `rows` days of the figure, renumbered from the first one shown. */
export function lastDays(activity: AccountActivity, rows: number): AccountActivity {
  if (activity.days <= rows) return activity;
  const offset = activity.days - rows;
  const shift = (segments: readonly NightSegment[]) =>
    segments
      .filter(([day]) => day >= offset)
      .map(([day, start, end]): NightSegment => [day - offset, start, end]);
  const first = new Date(`${activity.firstDate}T00:00:00Z`);
  first.setUTCDate(first.getUTCDate() + offset);
  return {
    ...activity,
    days: rows,
    firstDate: first.toISOString().slice(0, 10),
    suggested: shift(activity.suggested),
    recorded: shift(activity.recorded),
  };
}

/** A span of days in words: "84 days", "20 months", "3 years". */
export function spanWords(days: number) {
  if (days < 60) return `${days} days`;
  if (days < 730) return `${Math.round(days / 30.44)} months`;
  return `${Math.round(days / 365.25)} years`;
}

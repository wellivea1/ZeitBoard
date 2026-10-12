import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { NightsFigure } from "../data/activityNights";
import { ActivityNightsFigure } from "./ActivityNightsFigure";
import { figureHeight, lastDays, nightPath, rowLabels, spanWords } from "./activityNightsLayout";

describe("ActivityNightsFigure", () => {
  // Row r holds day r and then day r + 1, so a night across midnight is drawn
  // whole on the row it began, and its second part again on its own row.
  it("double-plots each part of a night", () => {
    expect(
      nightPath([
        [0, 1380, 1440],
        [1, 0, 420],
      ]),
    ).toBe("M1380 0h60v1h-60zM0 1h420v1h-420zM1440 0h420v1h-420z");
    expect(nightPath([[2, 60, 120]], 0.3)).toBe("M60 2.3h60v0.4h-60zM1500 1.3h60v0.4h-60z");
  });

  it("shares a page's height between the days", () => {
    expect(figureHeight(10)).toBe(120);
    expect(figureHeight(84)).toBe(504);
    expect(figureHeight(3650)).toBe(960);
  });

  it("labels weeks over weeks, and years over years", () => {
    expect(rowLabels("2026-09-01", 30, figureHeight(30)).map((label) => label.text)).toEqual([
      "Sep 1",
      "Sep 8",
      "Sep 15",
      "Sep 22",
      "Sep 29",
    ]);
    const decade = rowLabels("2016-03-15", 3650, figureHeight(3650));
    expect(decade.slice(0, 3).map((label) => label.text)).toEqual([
      "Jul 2016",
      "Jan 2017",
      "Jul 2017",
    ]);
    expect(decade).toHaveLength(20);
    const year = rowLabels("2025-10-01", 365, figureHeight(365));
    expect(year[0]).toEqual({ row: 0, text: "Oct 2025" });
    expect(year.length).toBeGreaterThan(2);
  });

  it("names what it shows", () => {
    const figure: NightsFigure = {
      nights: 12,
      zoneId: "UTC",
      spanLabel: "Aug 31, 2026 to Sep 13, 2026",
      firstDate: "2026-08-31",
      days: 14,
      suggested: [[0, 1410, 1440]],
      recorded: [[7, 300, 780]],
      agreement: { recorded: 6, matched: 6, summary: "" },
    };
    render(<ActivityNightsFigure figure={figure} source="your Google activity" />);
    expect(
      screen.getByRole("img", {
        name: "Nights suggested by your Google activity, Aug 31, 2026 to Sep 13, 2026: 12 suggested nights, beside 6 recorded.",
      }),
    ).toBeInTheDocument();
    expect(screen.getByText(/Suggested by your Google activity/)).toBeInTheDocument();
    // Two weeks need no choice of span.
    expect(screen.queryByRole("group", { name: "Days drawn" })).toBeNull();
  });

  // Over a long export the rows would be too thin to compare, so the figure
  // opens on the last weeks and can show everything.
  it("opens a long export on its last weeks", () => {
    const figure: NightsFigure = {
      nights: 600,
      zoneId: "UTC",
      spanLabel: "Feb 10, 2025 to Oct 10, 2026",
      firstDate: "2025-02-10",
      days: 608,
      suggested: [
        [0, 1380, 1440],
        [600, 120, 600],
      ],
      recorded: [[607, 60, 540]],
    };
    render(<ActivityNightsFigure figure={figure} source="your Google activity" />);
    const recent = screen.getByRole("button", { name: "Last 12 weeks" });
    expect(recent).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("img").getAttribute("aria-label")).toMatch(/^The last 12 weeks/);
    fireEvent.click(screen.getByRole("button", { name: "All 20 months" }));
    expect(screen.getByRole("img").getAttribute("aria-label")).toMatch(
      /^Nights suggested by your Google activity, Feb 10, 2025 to Oct 10, 2026/,
    );
  });

  it("renumbers the last weeks from their first day", () => {
    const shown = lastDays(
      {
        nights: 3,
        spanLabel: "",
        zoneId: "UTC",
        firstDate: "2026-01-01",
        days: 100,
        suggested: [
          [10, 0, 60],
          [16, 0, 60],
          [99, 60, 120],
        ],
        recorded: [[16, 30, 90]],
      },
      84,
    );
    expect(shown.firstDate).toBe("2026-01-17");
    expect(shown.suggested).toEqual([
      [0, 0, 60],
      [83, 60, 120],
    ]);
    expect(shown.recorded).toEqual([[0, 30, 90]]);
    expect(spanWords(608)).toBe("20 months");
    expect(spanWords(1100)).toBe("3 years");
  });
});

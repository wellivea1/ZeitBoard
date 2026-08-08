import { readFileSync } from "node:fs";
import { resolve as resolvePath } from "node:path";
import { describe, expect, it } from "vitest";

// Parse the real CSS custom-property palettes so this guard tracks styles.css
// (rather than a hand-copied snapshot that could silently drift). vitest runs
// with the frontend package directory as the working directory.
const css = readFileSync(resolvePath(process.cwd(), "src/styles.css"), "utf8");

function blockVars(selector: string): Record<string, string> {
  const start = css.indexOf(`${selector} {`);
  if (start === -1) throw new Error(`CSS block not found: ${selector}`);
  const open = css.indexOf("{", start);
  const close = css.indexOf("\n}", open);
  const body = css.slice(open + 1, close);
  const vars: Record<string, string> = {};
  for (const match of body.matchAll(/--([\w-]+):\s*([^;]+);/g)) {
    const name = match[1];
    const value = match[2];
    if (name && value) vars[name] = value.trim();
  }
  return vars;
}

const light = blockVars(":root");
// Every preset cascades over the light root.
const dark = { ...light, ...blockVars('[data-theme="dark"]') };
const black = { ...light, ...blockVars('[data-theme="black"]') };
const amber = { ...light, ...blockVars('[data-theme="amber"]') };
const contrast = { ...light, ...blockVars('[data-theme="contrast"]') };

function resolve(map: Record<string, string>, token: string): string {
  let value: string | undefined = map[token];
  for (let i = 0; i < 5 && value !== undefined && value.startsWith("var("); i += 1) {
    const ref = value.slice(4, -1).trim().replace(/^--/, "");
    value = map[ref];
  }
  return value ?? "";
}

function channel(c: number): number {
  const s = c / 255;
  return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
}

function luminance(hex: string): number {
  const raw = hex.replace("#", "");
  const h = raw.length === 3 ? raw.replace(/(.)/g, "$1$1") : raw;
  const r = parseInt(h.slice(0, 2), 16);
  const g = parseInt(h.slice(2, 4), 16);
  const b = parseInt(h.slice(4, 6), 16);
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

function ratio(fg: string, bg: string): number {
  const a = luminance(fg);
  const b = luminance(bg);
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
}

// fg/bg are CSS custom-property names; min is the WCAG 2.2 AA threshold
// (4.5 for body text, 3.0 for large text / UI / graphical objects).
const pairs: { fg: string; bg: string; min: number; label: string }[] = [
  { fg: "muted", bg: "canvas", min: 4.5, label: "muted text on canvas" },
  { fg: "muted", bg: "chrome", min: 4.5, label: "muted text on the sidebar" },
  { fg: "muted", bg: "panel-alt", min: 4.5, label: "muted text on panel-alt" },
  { fg: "subtle", bg: "canvas", min: 4.5, label: "subtle text on canvas" },
  { fg: "subtle", bg: "panel-alt", min: 4.5, label: "subtle text on panel-alt" },
  { fg: "subtle", bg: "paper", min: 4.5, label: "subtle text on paper" },
  { fg: "sage-dark", bg: "paper", min: 4.5, label: "link / secondary-button text" },
  {
    fg: "confidence-high-text",
    bg: "confidence-high-bg",
    min: 4.5,
    label: "confidence high badge",
  },
  {
    fg: "confidence-medium-text",
    bg: "confidence-medium-bg",
    min: 4.5,
    label: "confidence medium badge",
  },
  { fg: "confidence-low-text", bg: "confidence-low-bg", min: 4.5, label: "confidence low badge" },
  { fg: "button-primary-text", bg: "button-primary-bg", min: 4.5, label: "primary button" },
  {
    fg: "actogram-observed-text",
    bg: "actogram-observed",
    min: 4.5,
    label: "actogram observed label",
  },
  { fg: "sage", bg: "paper", min: 3.0, label: "focus ring on paper" },
  { fg: "sage", bg: "canvas", min: 3.0, label: "focus ring on canvas" },
];

describe.each([
  ["light", light],
  ["dark", dark],
  ["black", black],
  ["amber", amber],
  ["contrast", contrast],
])("WCAG 2.2 AA contrast (%s theme)", (_theme, map) => {
  it.each(pairs)("$label meets $min:1", ({ fg, bg, min }) => {
    const foreground = resolve(map, fg);
    const background = resolve(map, bg);
    expect(foreground, `--${fg}`).toMatch(/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/);
    expect(background, `--${bg}`).toMatch(/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/);
    expect(ratio(foreground, background)).toBeGreaterThanOrEqual(min);
  });
});

// Section rules are the only thing separating the blocks on the Home surface,
// which is one continuous surface rather than a stack of cards. They were
// shipping at 1.09-1.17:1 against that surface in every everyday theme, which
// is not a quiet hairline, it is an invisible one, and the sections ran
// together. 1.4:1 is the floor at which a 1px rule reads as deliberate without
// turning the page into a grid.
describe.each([
  ["light", light],
  ["dark", dark],
  ["black", black],
  ["amber", amber],
  ["contrast", contrast],
])("section rules stay visible (%s theme)", (_theme, map) => {
  it.each([
    ["divider", "paper"],
    ["divider", "canvas"],
    ["line", "paper"],
  ])("--%s reads against --%s", (rule, surface) => {
    expect(ratio(resolve(map, rule), resolve(map, surface))).toBeGreaterThanOrEqual(1.4);
  });
});

// --- Amber glasses mode (ui-refactor-plan.md §3) ---
// Conservative display-compatibility simulation for a dark-amber filter.
// This is a UI regression heuristic, not lens spectroscopy or clinical proof.
const LENS_G = 0.25;
const LENS_B = 0.02;

function throughLensLuminance(hex: string): number {
  const raw = hex.replace("#", "");
  const h = raw.length === 3 ? raw.replace(/(.)/g, "$1$1") : raw;
  const r = parseInt(h.slice(0, 2), 16);
  const g = parseInt(h.slice(2, 4), 16);
  const b = parseInt(h.slice(4, 6), 16);
  return 0.2126 * channel(r) + 0.7152 * LENS_G * channel(g) + 0.0722 * LENS_B * channel(b);
}

function throughLensRatio(fg: string, bg: string): number {
  const a = throughLensLuminance(fg);
  const b = throughLensLuminance(bg);
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
}

describe("amber glasses mode", () => {
  it("body text keeps >=7:1 in the simulated dark-amber filter", () => {
    expect(
      throughLensRatio(resolve(amber, "ink"), resolve(amber, "canvas")),
    ).toBeGreaterThanOrEqual(7);
    expect(throughLensRatio(resolve(amber, "ink"), resolve(amber, "paper"))).toBeGreaterThanOrEqual(
      7,
    );
  });

  it("secondary text keeps >=3:1 in the simulated dark-amber filter", () => {
    expect(
      throughLensRatio(resolve(amber, "muted"), resolve(amber, "canvas")),
    ).toBeGreaterThanOrEqual(3);
    expect(
      throughLensRatio(resolve(amber, "subtle"), resolve(amber, "paper")),
    ).toBeGreaterThanOrEqual(3);
  });

  it("minimizes the commanded blue channel to <=10%", () => {
    const block = blockVars('[data-theme="amber"]');
    for (const [name, value] of Object.entries(block)) {
      for (const match of value.matchAll(/#([0-9a-fA-F]{6})(?![0-9a-fA-F])/g)) {
        const hex = match[1] ?? "";
        const blue = parseInt(hex.slice(4, 6), 16);
        expect(blue, `--${name}: #${hex}`).toBeLessThanOrEqual(26);
      }
      const rgba = value.match(/rgba\(\s*\d+\s*,\s*\d+\s*,\s*(\d+)/);
      if (rgba) {
        expect(Number(rgba[1]), `--${name}: ${value}`).toBeLessThanOrEqual(26);
      }
    }
  });
});

// High contrast preset promises AAA for its core text.
describe("high contrast preset", () => {
  it("ink on canvas reaches 7:1", () => {
    expect(ratio(resolve(contrast, "ink"), resolve(contrast, "canvas"))).toBeGreaterThanOrEqual(7);
    expect(ratio(resolve(contrast, "muted"), resolve(contrast, "paper"))).toBeGreaterThanOrEqual(7);
  });
});

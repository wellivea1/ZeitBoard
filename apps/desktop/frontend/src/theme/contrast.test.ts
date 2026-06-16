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
// Dark theme cascades over the light root.
const dark = { ...light, ...blockVars('[data-theme="dark"]') };

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
])("WCAG 2.2 AA contrast (%s theme)", (_theme, map) => {
  it.each(pairs)("$label meets $min:1", ({ fg, bg, min }) => {
    const foreground = resolve(map, fg);
    const background = resolve(map, bg);
    expect(foreground, `--${fg}`).toMatch(/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/);
    expect(background, `--${bg}`).toMatch(/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/);
    expect(ratio(foreground, background)).toBeGreaterThanOrEqual(min);
  });
});

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { basename, extname, join, relative, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const frontend = join(root, "apps", "desktop", "frontend", "src");
const failures = [];

function filesUnder(directory, extension) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return filesUnder(path, extension);
    return extname(entry.name) === extension ? [path] : [];
  });
}

function fail(path, message) {
  failures.push(`${relative(root, path)}: ${message}`);
}

const screenFiles = filesUnder(join(frontend, "screens"), ".tsx");
const uiFiles = [...screenFiles, ...filesUnder(join(frontend, "components"), ".tsx")];

for (const path of uiFiles) {
  const source = readFileSync(path, "utf8");
  const lineCount = source.split(/\r?\n/).length;
  if (lineCount > 600) fail(path, `UI modules are capped at 600 lines; found ${lineCount}.`);
  if (/globalThis\s*\.\s*go|\/wailsjs\//.test(source)) {
    fail(path, "UI modules must use typed data adapters instead of the desktop bridge directly.");
  }
}

for (const path of screenFiles.filter((path) => basename(path).endsWith("Screen.tsx"))) {
  const source = readFileSync(path, "utf8");
  const exports = source.match(/export function \w+Screen\s*\(/g) ?? [];
  if (exports.length !== 1) {
    fail(
      path,
      `Each screen module must export exactly one screen component; found ${exports.length}.`,
    );
  }
}

const secondaryScreens = join(frontend, "screens", "SecondaryScreens.tsx");
if (existsSync(secondaryScreens)) {
  fail(secondaryScreens, "The legacy multi-screen module must not be recreated.");
}

const overviewPath = join(frontend, "screens", "OverviewScreen.tsx");
const overview = readFileSync(overviewPath, "utf8");
for (const required of ["CycleStrip", "overview-surface", "overview-facts"]) {
  if (!overview.includes(required)) fail(overviewPath, `Overview must retain ${required}.`);
}
if (/metric-card|className="[^"]*\bpanel\b/.test(overview)) {
  fail(overviewPath, "Overview is one surface; generic panels and metric cards are forbidden.");
}

const componentStyles = filesUnder(join(frontend, "styles"), ".css");
for (const path of componentStyles) {
  const source = readFileSync(path, "utf8");
  const lines = source.split(/\r?\n/);
  lines.forEach((line, index) => {
    if (/#[0-9a-f]{3,8}\b|rgba?\(|hsla?\(/i.test(line)) {
      fail(path, `line ${index + 1} contains a raw color; define it in the theme token layer.`);
    }
    const radius = line.match(/border-radius:\s*([^;]+);/);
    if (radius && radius[1] !== "0" && !radius[1]?.includes("var(--radius-")) {
      fail(path, `line ${index + 1} contains a raw radius; use a radius token.`);
    }
  });
}

const tokenSource = readFileSync(join(frontend, "styles.css"), "utf8");
for (const token of [
  "--space-1",
  "--space-8",
  "--radius-control",
  "--radius-card",
  "--radius-overlay",
  "--type-data-large",
]) {
  if (!tokenSource.includes(`${token}:`)) {
    fail(join(frontend, "styles.css"), `Required UI token ${token} is missing.`);
  }
}

const rhythmStylesPath = join(frontend, "styles", "rhythm.css");
const rhythmStyles = readFileSync(rhythmStylesPath, "utf8");
if (!/\.actogram-panel\s*\{[^}]*overflow-x:\s*hidden;/s.test(rhythmStyles)) {
  fail(
    rhythmStylesPath,
    "The actogram panel must contain nested chart overflow on narrow screens.",
  );
}
if (!/\.actogram-chart\s*\{[^}]*overflow-x:\s*auto;/s.test(tokenSource)) {
  fail(
    join(frontend, "styles.css"),
    "The actogram chart must retain internal horizontal scrolling.",
  );
}
if (!/\.actogram-visual-grid\s*\{[^}]*min-width:\s*760px;/s.test(tokenSource)) {
  fail(
    join(frontend, "styles.css"),
    "The double-plot grid must retain its readable minimum width.",
  );
}

if (failures.length > 0) {
  console.error("UI standards check failed:\n" + failures.map((item) => `- ${item}`).join("\n"));
  process.exitCode = 1;
} else {
  console.log(
    `UI standards check passed (${screenFiles.length} screen modules, ${componentStyles.length} component stylesheets).`,
  );
}

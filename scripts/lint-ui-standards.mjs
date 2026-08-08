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

function hasStaticClass(source, className) {
  return [...source.matchAll(/className="([^"]+)"/g)].some((match) =>
    match[1].split(/\s+/).includes(className),
  );
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

const homePath = join(frontend, "screens", "HomeScreen.tsx");
const home = readFileSync(homePath, "utf8");
for (const required of ["CycleStrip", "OutlookPanel", "overview-surface", "overview-facts"]) {
  if (!home.includes(required)) fail(homePath, `Home must retain ${required}.`);
}
if (/metric-card/.test(home) || hasStaticClass(home, "panel")) {
  fail(homePath, "Home is one surface; generic panels and metric cards are forbidden.");
}

// Slice U-H: five primary destinations, then a utility group. The count is the
// point — eight equal-weight entries was too much undifferentiated navigation
// for someone operating under fatigue, and nothing stops it creeping back
// except a check.
const shellPath = join(frontend, "components", "AppShell.tsx");
const shell = readFileSync(shellPath, "utf8");
const primaryBlock = shell.match(/const primaryNavigation: NavItem\[\] = \[(.*?)\];/s)?.[1] ?? "";
const primaryCount = [...primaryBlock.matchAll(/\{\s*id:/g)].length;
if (primaryCount !== 5) {
  fail(shellPath, `Primary navigation must hold exactly five destinations; found ${primaryCount}.`);
}
if (!shell.includes("utilityNavigation") || !shell.includes('className="utility-nav"')) {
  fail(shellPath, "The utility group must stay separate from the primary destinations.");
}

// Legacy hashes stay routable. They are written down in the runbook, in this
// app's own links, and in whatever the user bookmarked; a dead link is a worse
// answer than a redirect.
const legacyStart = shell.indexOf("const legacyRoutes");
const legacyBlock =
  legacyStart < 0 ? "" : shell.slice(legacyStart, shell.indexOf("};", legacyStart));
for (const legacy of ["overview", "calendar", "tasks", "approvals", "medications", "timeline"]) {
  if (!legacyBlock.includes(`${legacy}: { screen:`)) {
    fail(shellPath, `The legacy #/${legacy} route must keep redirecting.`);
  }
}

// Plan and Log are tab hosts, not new monoliths: they compose the screens they
// absorbed rather than copying them.
for (const [name, required] of [
  ["PlanScreen.tsx", ["CalendarScreen", "TasksScreen", "ApprovalsScreen", "ScreenTabs"]],
  ["LogScreen.tsx", ["SleepLogPanel", "MedicationsScreen", "RhythmMarkersPanel", "ScreenTabs"]],
]) {
  const path = join(frontend, "screens", name);
  const source = readFileSync(path, "utf8");
  for (const symbol of required) {
    if (!source.includes(symbol)) fail(path, `${name} must compose ${symbol}.`);
  }
}

for (const [name, requiredClass] of [
  ["DataSourcesScreen.tsx", "data-source-workspace"],
  ["SharingScreen.tsx", "sharing-workspace"],
  ["PlanScreen.tsx", "screen-tabbed"],
  ["LogScreen.tsx", "screen-tabbed"],
]) {
  const path = join(frontend, "screens", name);
  const source = readFileSync(path, "utf8");
  if (!source.includes(requiredClass)) {
    fail(path, `${name} must retain its ruled ${requiredClass} composition.`);
  }
  if (hasStaticClass(source, "panel")) {
    fail(path, `${name} must not restore generic rounded panel wrappers.`);
  }
}

const sharingPath = join(frontend, "screens", "SharingScreen.tsx");
const sharing = readFileSync(sharingPath, "utf8");
if (/\bavatar\b|>\s*Active\s*</.test(sharing)) {
  fail(sharingPath, "Sharing examples must not look like real people or active links.");
}

const androidAppPath = join(
  root,
  "apps",
  "android",
  "app",
  "src",
  "main",
  "java",
  "org",
  "non24",
  "planner",
  "ui",
  "Non24App.kt",
);
const androidApp = readFileSync(androidAppPath, "utf8");
if (
  /private fun Panel\s*\(/.test(androidApp) ||
  !androidApp.includes("private fun RuledSection(")
) {
  fail(
    androidAppPath,
    "Android sections must use the ruled composition instead of a generic Panel wrapper.",
  );
}

// Hiding the sidebar footer on a narrow window used to be harmless because
// Data Sources was a primary destination. After slice U-H it lives in the
// utility group, and hiding the footer would strand it and Settings with it.
const shellStyles = readFileSync(join(frontend, "styles.css"), "utf8");
const narrow = shellStyles.slice(shellStyles.indexOf("@media (max-width: 700px)"));
if (/\.sidebar-footer,?[^{]*\{[^}]*display:\s*none/.test(narrow)) {
  fail(
    join(frontend, "styles.css"),
    "The utility group must stay reachable on a narrow window; do not hide .sidebar-footer.",
  );
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
if (!/\.rhythm-screen\s*\{[^}]*overflow-x:\s*clip;/s.test(rhythmStyles)) {
  fail(
    rhythmStylesPath,
    "The Rhythm screen must clip nested visualization overflow at the page boundary.",
  );
}
if (
  !/\.actogram-panel\s*\{(?=[^}]*contain:\s*paint;)[^}]*overflow-x:\s*hidden;/s.test(rhythmStyles)
) {
  fail(
    rhythmStylesPath,
    "The actogram panel must paint-contain nested chart overflow on narrow screens.",
  );
}
if (!/\.actogram-chart\s*\{[^}]*overflow-x:\s*auto;/s.test(tokenSource)) {
  fail(
    join(frontend, "styles.css"),
    "The actogram chart must retain internal horizontal scrolling.",
  );
}
if (!/\.sr-table\s*\{[^}]*table-layout:\s*fixed;/s.test(tokenSource)) {
  fail(
    join(frontend, "styles.css"),
    "Visually hidden data tables must not expand the narrow page layout.",
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

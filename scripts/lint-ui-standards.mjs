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
// Home is one screen: the current state with the one-tap actions, one
// timeline, and what needs you. The 24-hour strip restated the timeline, and a
// second one must not creep back; nor may the grid of metric cards it replaced.
for (const required of ["OutlookPanel", "QuickLogBar", "home-now", "NeedsYou"]) {
  if (!home.includes(required)) fail(homePath, `Home must retain ${required}.`);
}
if (/CycleStrip|cycle-strip/.test(home)) {
  fail(homePath, "Home draws one timeline; the 24-hour strip must not return.");
}
if (/metric-card/.test(home) || hasStaticClass(home, "panel")) {
  fail(homePath, "Generic panels and metric cards are forbidden on Home.");
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

// Legacy hashes stay routable. They are written down in the verification
// record and in whatever the user bookmarked; a dead link is a worse answer
// than a redirect.
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
  ["PlanScreen.tsx", ["WeekScreen", "TasksScreen", "ScreenTabs"]],
  ["LogScreen.tsx", ["SleepLogPanel", "MedicationsScreen", "RhythmMarkersPanel", "ScreenTabs"]],
]) {
  const path = join(frontend, "screens", name);
  const source = readFileSync(path, "utf8");
  for (const symbol of required) {
    if (!source.includes(symbol)) fail(path, `${name} must compose ${symbol}.`);
  }
}

// Tasks and the decisions about them are one view. Accepting a task's time on
// a different tab from the one it was added on was the most awkward loop in
// the app; the Approvals tab must not come back.
const tasksPath = join(frontend, "screens", "TasksScreen.tsx");
if (!readFileSync(tasksPath, "utf8").includes("<DecisionQueue")) {
  fail(tasksPath, "Tasks must show the decisions about them.");
}
if (existsSync(join(frontend, "screens", "ApprovalsScreen.tsx"))) {
  fail(tasksPath, "Decisions live beside tasks; a separate Approvals screen must not return.");
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

// This rule used to forbid anything that looked like a real person or an active
// link, because the screen was a static capability preview. Since roadmap slice
// 12a it shows the owner's actual links, so the thing to guard is the opposite:
// no invented rows, and the state must come from the adapter rather than from a
// literal in the module.
const sharingPath = join(frontend, "screens", "SharingScreen.tsx");
const sharing = readFileSync(sharingPath, "utf8");
if (/\bavatar\b|Example only|relationshipTemplates/.test(sharing)) {
  fail(sharingPath, "Sharing must render real links, not invented example rows.");
}
if (!sharing.includes("loadShareLinks")) {
  fail(sharingPath, "Sharing must read its state from the share-link adapter.");
}

// One-tap logging belongs on Home, where someone lands on waking. Burying it
// behind Log is what the four-field form already did, and the form is the thing
// this replaced.
if (!home.includes("<QuickLogBar")) {
  fail(homePath, "Home must offer the one-tap sleep actions.");
}

// A prefill drawn from the estimator has to say so on the field. Losing this
// label is how a forecast becomes something the reader believes was recorded.
const quickLogPath = join(frontend, "components", "QuickLogBar.tsx");
// Class names and comments are not what the reader sees, so they do not count
// towards the label.
const quickLogMarkup = readFileSync(quickLogPath, "utf8")
  .replace(/className="[^"]*"/g, "")
  .replace(/\/\*[\s\S]*?\*\//g, "")
  .replace(/\/\/[^\n]*/g, "");
if (!/isPrediction\s*&&/.test(quickLogMarkup) || !/predicted/i.test(quickLogMarkup)) {
  fail(quickLogPath, "A predicted prefill must be labelled as a prediction.");
}

// Reaching hours describe whoever the person needs to contact, and the outlook
// used to assert Monday-to-Friday nine-to-five as a fact. Neither the label nor
// the empty-state fallback may reintroduce a schedule nobody chose.
for (const name of ["outlook.ts", "reachingHours.ts"]) {
  const path = join(frontend, "data", name);
  const source = readFileSync(path, "utf8").replace(/\/\/[^\n]*/g, "");
  if (/Monday to Friday/.test(source)) {
    fail(path, `${name} must not hard-code a working week as the reaching-hours default.`);
  }
}

const reachingPanelPath = join(frontend, "screens", "settings", "ReachingHoursSettings.tsx");
const reachingPanel = readFileSync(reachingPanelPath, "utf8");
if (!reachingPanel.includes("saveReachingHours") || !reachingPanel.includes("loadReachingHours")) {
  fail(reachingPanelPath, "Reaching hours must read and write the real stored schedule.");
}
if (!/not yours/.test(reachingPanel)) {
  fail(reachingPanelPath, "Reaching hours must say the schedule belongs to the other party.");
}

const settingsScreenPath = join(frontend, "screens", "SettingsScreen.tsx");
if (!readFileSync(settingsScreenPath, "utf8").includes("<ReachingHoursSettings")) {
  fail(settingsScreenPath, "Settings must expose the reaching-hours schedule.");
}

// An owner-only file permission is not encryption, and the local database is
// not encrypted. privacy.md asserted otherwise for months; the readout that
// replaced that claim must not drift back towards it.
const protectionPanelPath = join(frontend, "screens", "settings", "StorageProtectionPanel.tsx");
const protectionPanel = readFileSync(protectionPanelPath, "utf8");
if (!protectionPanel.includes("report.detail")) {
  fail(protectionPanelPath, "The storage readout must render the caveat the app computed.");
}
// The sentence itself comes from Go, so that is where it has to be pinned. The
// adapter merely carries it, and the previous version of this rule inspected
// the adapter and therefore checked nothing.
const protectionSourcePath = join(root, "apps", "desktop", "app_storage_protection.go");
const protectionSource = readFileSync(protectionSourcePath, "utf8");
const detailLiteral = protectionSource.match(/storageProtectionDetail\s*=\s*([\s\S]*?)\n\n/);
if (!detailLiteral || !/not encrypted/.test(detailLiteral[1])) {
  fail(
    protectionSourcePath,
    "The storage caveat must still say the local files are not encrypted.",
  );
}

const protectionAdapterPath = join(frontend, "data", "storageProtection.ts");
const protectionAdapter = readFileSync(protectionAdapterPath, "utf8");
if (!/state:\s*"unknown"/.test(protectionAdapter)) {
  fail(
    protectionAdapterPath,
    "A permission the app could not check must read as unknown, never as protected.",
  );
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

// The utility group (Data Sources, Settings, the assistant) sits in the
// masthead beside the five destinations. A narrow window sets it on its own
// line; hiding it there would strand Data Sources and Settings.
const shellStyles = readFileSync(join(frontend, "styles.css"), "utf8");
for (const match of shellStyles.matchAll(/@media \(max-width: \d+px\)\s*\{/g)) {
  const block = shellStyles.slice(match.index, shellStyles.indexOf("\n}\n", match.index));
  if (/\.utility-nav,?[^{]*\{[^}]*display:\s*none/.test(block)) {
    fail(
      join(frontend, "styles.css"),
      "The utility group must stay reachable on a narrow window; do not hide .utility-nav.",
    );
  }
}

// One content column on the Home surface. Every section there carries a
// full-bleed rule, so the rules line up and any difference in inline padding
// shows as content that does not — the status header and cycle strip sat at
// space-7 while everything below them sat at space-6, and 4px of stagger down
// a page reads as sloppiness rather than as hierarchy.
for (const name of ["overview.css", "outlook.css"]) {
  const path = join(frontend, "styles", name);
  readFileSync(path, "utf8")
    .split(/\r?\n/)
    .forEach((line, index) => {
      if (/padding[^:]*:[^;]*--space-7/.test(line)) {
        fail(
          path,
          `line ${index + 1} insets a Home surface section by a different step; the surface is one content column.`,
        );
      }
    });
}

// The assistant toggle is a masthead link like Settings. It used to float over
// the top-right of the content, where every page header puts its controls,
// and each header had to reserve room for it.
const masthead = shell.match(/<header className="masthead">([\s\S]*?)<\/header>/)?.[1] ?? "";
if (!masthead.includes('className="assistant-toggle"')) {
  fail(shellPath, "The assistant toggle belongs in the masthead, not floating over the page.");
}

// The Almanac redesign (ui-refactor-plan.md §14): no coloured side stripes and
// no pills. A 3px bar down the left of a block and a rounded tinted capsule
// around a word were the two habits that made every screen look generated;
// a rule across the page and the word itself do the same work. Hairlines of
// 1px (dividers, ticks, a now line) are allowed; anything heavier on one side
// is a stripe.
const stripe =
  /border-(?:left|right|inline-start|inline-end)(?:-width)?\s*:\s*(?:\d*\.)?\d+px/;
for (const path of [join(frontend, "styles.css"), ...filesUnder(join(frontend, "styles"), ".css")]) {
  const source = readFileSync(path, "utf8").replace(/\/\*[\s\S]*?\*\//g, (comment) =>
    comment.replace(/[^\n]/g, " "),
  );
  source.split(/\r?\n/).forEach((line, index) => {
    const side = line.match(stripe);
    const width = side ? Number.parseFloat(side[0].split(":")[1]) : 0;
    if (width > 1.5) {
      fail(path, `line ${index + 1} draws a ${width}px side stripe; divide with a rule instead.`);
    }
    const inset = line.match(/inset\s+(-?(?:\d*\.)?\d+)px\s+0\s+0/);
    if (inset && Math.abs(Number.parseFloat(inset[1])) > 1.5) {
      fail(path, `line ${index + 1} draws a side stripe with an inset shadow.`);
    }
    if (/--radius-pill|border-radius:\s*999/.test(line)) {
      fail(path, `line ${index + 1} rounds a pill; set the word on the page instead.`);
    }
  });
  // A pseudo-element painted as a narrow full-height bar is the same stripe.
  for (const rule of source.matchAll(/([^{}]*::?(?:before|after)[^{}]*)\{([^}]*)\}/g)) {
    const body = rule[2];
    const narrow = body.match(/(?:^|[\s;])width:\s*((?:\d*\.)?\d+)px/);
    const tall = /inset:\s*0 auto 0 0|inset-block:\s*0|top:\s*0;[\s\S]*bottom:\s*0|height:\s*100%/.test(
      body,
    );
    if (narrow && Number.parseFloat(narrow[1]) > 1.5 && Number.parseFloat(narrow[1]) <= 8 && tall) {
      fail(path, `${rule[1].trim()} paints a side stripe.`);
    }
  }
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
  "--font-serif",
  "--accent",
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
// `clip`, not `hidden`: with a visible overflow-y, an `overflow-x: hidden`
// makes the used overflow-y `auto` (CSS Overflow 3 §3.3), which turned this
// panel into a scroll container and drew a scrollbar straight over the
// confidence label at the end of every actogram row. `clip` contains the same
// nested chart overflow without that side effect.
if (
  !/\.actogram-panel\s*\{(?=[^}]*contain:\s*paint;)[^}]*overflow-x:\s*clip;/s.test(rhythmStyles)
) {
  fail(
    rhythmStylesPath,
    "The actogram panel must clip nested chart overflow with `overflow-x: clip`.",
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

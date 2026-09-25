import { lazy, Suspense, useEffect, useState, type ReactNode } from "react";
import { usePendingApprovalsCount } from "../state/approvalQueue";
import type { LogTab, PlanTab, ScreenId, SettingsTab } from "../types";

const AssistantRail = lazy(() =>
  import("./AssistantRail").then((module) => ({ default: module.AssistantRail })),
);

interface NavItem {
  id: ScreenId;
  label: string;
  badge?: string;
}

// Five primary destinations (slice U-H). Eight equal-weight entries was too
// much undifferentiated navigation for someone operating under fatigue, which
// is the condition this product is for.
const primaryNavigation: NavItem[] = [
  { id: "home", label: "Home" },
  { id: "plan", label: "Plan" },
  { id: "rhythm", label: "Rhythm" },
  { id: "log", label: "Log" },
  { id: "sharing", label: "Sharing" },
];

// The utility group: things you configure once and revisit rarely. They stay
// reachable, and they stop competing with the five you use daily.
const utilityNavigation: NavItem[] = [
  { id: "data-sources", label: "Data Sources" },
  { id: "settings", label: "Settings" },
];

const screenIds = new Set<ScreenId>([
  ...primaryNavigation.map((item) => item.id),
  ...utilityNavigation.map((item) => item.id),
]);

export interface Route {
  screen: ScreenId;
  planTab: PlanTab;
  logTab: LogTab;
  settingsTab: SettingsTab;
}

const planTabs = new Set<PlanTab>(["tasks", "week"]);
const logTabs = new Set<LogTab>(["sleep", "medications", "markers"]);
const settingsTabs = new Set<SettingsTab>(["display", "reaching", "sync", "computer", "data"]);

// Routes that existed before the consolidation. They are still written down,
// in the verification record and in whatever was bookmarked, and a dead link is
// a worse answer than a redirect that costs one line each. Approvals became
// part of Tasks, so its old address lands there.
const legacyRoutes: Record<string, Partial<Route> & { screen: ScreenId }> = {
  overview: { screen: "home" },
  timeline: { screen: "rhythm" },
  calendar: { screen: "plan", planTab: "week" },
  tasks: { screen: "plan", planTab: "tasks" },
  approvals: { screen: "plan", planTab: "tasks" },
  medications: { screen: "log", logTab: "medications" },
};

// Plan opens on Tasks: it carries the pending count, so the badge on Plan and
// the page it opens agree about what needs you.
const defaultRoute: Route = {
  screen: "home",
  planTab: "tasks",
  logTab: "sleep",
  settingsTab: "display",
};

export function readRouteFromHash(hash: string): Route {
  const path = hash.replace(/^#\/?/, "");
  const [head = "", second = ""] = path.split("/");

  const legacy = legacyRoutes[head];
  if (legacy) return { ...defaultRoute, ...legacy };

  if (!screenIds.has(head as ScreenId)) return defaultRoute;
  const screen = head as ScreenId;

  if (screen === "plan" && planTabs.has(second as PlanTab)) {
    return { ...defaultRoute, screen, planTab: second as PlanTab };
  }
  // Plan › Calendar became Plan › Week when the board turned calendar-native.
  if (screen === "plan" && second === "calendar") {
    return { ...defaultRoute, screen, planTab: "week" };
  }
  if (screen === "log" && logTabs.has(second as LogTab)) {
    return { ...defaultRoute, screen, logTab: second as LogTab };
  }
  if (screen === "settings" && settingsTabs.has(second as SettingsTab)) {
    return { ...defaultRoute, screen, settingsTab: second as SettingsTab };
  }
  return { ...defaultRoute, screen };
}

// eslint-disable-next-line react-refresh/only-export-components
export function useScreenNavigation() {
  const [route, setRoute] = useState<Route>(() => readRouteFromHash(window.location.hash));

  useEffect(() => {
    const onHashChange = () => setRoute(readRouteFromHash(window.location.hash));
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  // A tab is part of the address: coming back to a bookmarked or reloaded view
  // should land where it did, and the assistant's links can point at one.
  const selectPlanTab = (planTab: PlanTab) => {
    window.location.hash = `#/plan/${planTab}`;
  };
  const selectLogTab = (logTab: LogTab) => {
    window.location.hash = `#/log/${logTab}`;
  };
  const selectSettingsTab = (settingsTab: SettingsTab) => {
    window.location.hash = `#/settings/${settingsTab}`;
  };

  return { route, selectPlanTab, selectLogTab, selectSettingsTab };
}

function NavigationLink({ item, active }: { item: NavItem; active: boolean }) {
  return (
    <a
      className="nav-link"
      data-active={active}
      href={`#/${item.id}`}
      aria-label={item.badge ? `${item.label}, ${item.badge} pending` : item.label}
      aria-current={active ? "page" : undefined}
    >
      {item.label}
      {item.badge && (
        <span className="nav-badge" aria-hidden="true">
          {item.badge}
        </span>
      )}
    </a>
  );
}

function useToday() {
  const [today, setToday] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setToday(new Date()), 60_000);
    return () => window.clearInterval(timer);
  }, []);
  return today.toLocaleDateString(undefined, { weekday: "long", day: "numeric", month: "long" });
}

export function AppShell({ screen, children }: { screen: ScreenId; children: ReactNode }) {
  const pendingCount = usePendingApprovalsCount();
  const today = useToday();
  const [assistantOpen, setAssistantOpen] = useState(false);
  const [assistantLoaded, setAssistantLoaded] = useState(false);

  const toggleAssistant = () => {
    if (!assistantOpen) setAssistantLoaded(true);
    setAssistantOpen((open) => !open);
  };

  return (
    <div className="app-shell" data-assistant-open={assistantOpen || undefined}>
      {/* A masthead over one sheet: the name, the day, and the destinations in
          small capitals. The sidebar it replaces spent a fixed column on seven
          links and a privacy blurb. */}
      <header className="masthead">
        <a className="wordmark" href="#/home" aria-label="ZeitBoard home">
          ZeitBoard
        </a>
        <span className="masthead-date">{today}</span>
        <nav className="primary-nav" aria-label="Primary navigation">
          {primaryNavigation.map((item) => (
            <NavigationLink
              key={item.id}
              item={
                item.id === "plan" && pendingCount > 0
                  ? { ...item, badge: String(pendingCount) }
                  : item
              }
              active={screen === item.id}
            />
          ))}
        </nav>
        <nav className="utility-nav" aria-label="Settings and sources">
          {utilityNavigation.map((item) => (
            <NavigationLink key={item.id} item={item} active={screen === item.id} />
          ))}
          <button
            className="assistant-toggle"
            type="button"
            data-active={assistantOpen || undefined}
            aria-pressed={assistantOpen}
            onClick={toggleAssistant}
          >
            Assistant
          </button>
        </nav>
      </header>

      <main className="main-content" id="main-content" tabIndex={-1}>
        {children}
      </main>

      {assistantLoaded && (
        <Suspense
          fallback={
            assistantOpen ? (
              <div className="assistant-rail" role="status">
                Loading assistant...
              </div>
            ) : null
          }
        >
          <AssistantRail open={assistantOpen} onClose={() => setAssistantOpen(false)} />
        </Suspense>
      )}
    </div>
  );
}

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  level = "page",
}: {
  eyebrow?: string;
  title: string;
  /**
   * Optional. Most screens are clear from their title; a sentence restating
   * the title on every page was part of what made the app read as prose.
   */
  description?: string;
  actions?: ReactNode;

  /**
   * `panel` is for a screen rendered inside a tab (slice U-H). The tab already
   * names the view and labels the panel, so repeating the title as a heading
   * would be noise — but the description and the actions are not noise, and
   * dropping the header entirely would have taken a calendar's date controls
   * with it.
   */
  level?: "page" | "panel";
}) {
  if (level === "panel") {
    return (
      <header className="panel-header">
        {description && <p>{description}</p>}
        {actions && <div className="page-actions">{actions}</div>}
      </header>
    );
  }
  return (
    <header className="page-header">
      <div>
        {eyebrow && <p className="eyebrow">{eyebrow}</p>}
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className="page-actions">{actions}</div>}
    </header>
  );
}

export function PlaceholderNotice({ children }: { children: ReactNode }) {
  return <p className="placeholder-notice">{children}</p>;
}

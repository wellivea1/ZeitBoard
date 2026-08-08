import { lazy, Suspense, useEffect, useState, type ReactNode } from "react";
import { Icon, type IconName } from "./Icon";
import { usePendingApprovalsCount } from "../state/approvals";
import type { LogTab, PlanTab, ScreenId } from "../types";

const AssistantRail = lazy(() =>
  import("./AssistantRail").then((module) => ({ default: module.AssistantRail })),
);

interface NavItem {
  id: ScreenId;
  label: string;
  icon: IconName;
  badge?: string;
}

// Five primary destinations (slice U-H). Eight equal-weight entries was too
// much undifferentiated navigation for someone operating under fatigue, which
// is the condition this product is for.
const primaryNavigation: NavItem[] = [
  { id: "home", label: "Home", icon: "overview" },
  { id: "plan", label: "Plan", icon: "calendar" },
  { id: "rhythm", label: "Rhythm", icon: "timeline" },
  { id: "log", label: "Log", icon: "sources" },
  { id: "sharing", label: "Sharing", icon: "sharing" },
];

// The utility group: things you configure once and revisit rarely. They stay
// reachable, and they stop competing with the five you use daily.
const utilityNavigation: NavItem[] = [
  { id: "data-sources", label: "Data Sources", icon: "medications" },
  { id: "settings", label: "Settings", icon: "settings" },
];

const screenIds = new Set<ScreenId>([
  ...primaryNavigation.map((item) => item.id),
  ...utilityNavigation.map((item) => item.id),
]);

export interface Route {
  screen: ScreenId;
  planTab: PlanTab;
  logTab: LogTab;
}

const planTabs = new Set<PlanTab>(["calendar", "tasks", "approvals"]);
const logTabs = new Set<LogTab>(["sleep", "medications", "markers"]);

// Routes that existed before the consolidation. They are kept because they are
// still written down — in this app's own links, in the runbook, and in whatever
// the user has bookmarked — and because a dead link is a worse answer than a
// redirect that costs one line each.
const legacyRoutes: Record<string, Partial<Route> & { screen: ScreenId }> = {
  overview: { screen: "home" },
  timeline: { screen: "rhythm" },
  calendar: { screen: "plan", planTab: "calendar" },
  tasks: { screen: "plan", planTab: "tasks" },
  approvals: { screen: "plan", planTab: "approvals" },
  medications: { screen: "log", logTab: "medications" },
};

const defaultRoute: Route = { screen: "home", planTab: "calendar", logTab: "sleep" };

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
  if (screen === "log" && logTabs.has(second as LogTab)) {
    return { ...defaultRoute, screen, logTab: second as LogTab };
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

  return { route, selectPlanTab, selectLogTab };
}

function NavigationLink({ item, active }: { item: NavItem; active: boolean }) {
  return (
    <a
      className="nav-link"
      data-active={active}
      href={`#/${item.id}`}
      aria-label={item.badge ? `${item.label}, ${item.badge} pending` : item.label}
      aria-current={active ? "page" : undefined}
      title={item.label}
    >
      <Icon name={item.icon} />
      <span>{item.label}</span>
      {item.badge && (
        <span className="nav-badge" aria-label={`${item.badge} pending`}>
          {item.badge}
        </span>
      )}
    </a>
  );
}

export function AppShell({ screen, children }: { screen: ScreenId; children: ReactNode }) {
  const pendingCount = usePendingApprovalsCount();
  const [assistantOpen, setAssistantOpen] = useState(false);
  const [assistantLoaded, setAssistantLoaded] = useState(false);

  const toggleAssistant = () => {
    if (!assistantOpen) setAssistantLoaded(true);
    setAssistantOpen((open) => !open);
  };

  return (
    <div className="app-shell" data-assistant-open={assistantOpen || undefined}>
      <aside className="sidebar">
        <a className="brand" href="#/home" aria-label="ZeitBoard home">
          <span className="brand-mark" aria-hidden="true">
            <span />
          </span>
          <span>
            <strong>ZeitBoard</strong>
            <small>Planner</small>
          </span>
        </a>

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

        <div className="sidebar-footer">
          <nav className="utility-nav" aria-label="Settings and sources">
            {utilityNavigation.map((item) => (
              <NavigationLink key={item.id} item={item} active={screen === item.id} />
            ))}
          </nav>
          <div className="privacy-note">
            <Icon name="shield" />
            <span>
              <strong>Private by design</strong>
              <small>Data stays local; syncs only to your own server if you turn sync on.</small>
            </span>
          </div>
        </div>
      </aside>

      <main className="main-content" id="main-content">
        <button
          className="assistant-toggle"
          type="button"
          data-active={assistantOpen || undefined}
          aria-pressed={assistantOpen}
          onClick={toggleAssistant}
        >
          <Icon name="sparkle" />
          <span>Assistant</span>
        </button>
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
  description: string;
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
        <p>{description}</p>
        {actions && <div className="page-actions">{actions}</div>}
      </header>
    );
  }
  return (
    <header className="page-header">
      <div>
        {eyebrow && <p className="eyebrow">{eyebrow}</p>}
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {actions && <div className="page-actions">{actions}</div>}
    </header>
  );
}

export function PlaceholderNotice({ children }: { children: ReactNode }) {
  return <p className="placeholder-notice">{children}</p>;
}

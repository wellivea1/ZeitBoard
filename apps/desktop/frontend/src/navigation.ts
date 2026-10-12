import { useEffect, useRef, useState } from "react";
import type { LogTab, PlanTab, RhythmTab, ScreenId, SettingsTab } from "./types";

// Where the window can go: its destinations, the address of each, and the
// hook that follows the address. The shell draws them; this module decides
// them, so the address logic can be read and tested apart from the layout.

export interface NavItem {
  id: ScreenId;
  label: string;
  badge?: string;
}

// Five primary destinations (slice U-H). Eight equal-weight entries was too
// much undifferentiated navigation for someone operating under fatigue, which
// is the condition this product is for.
export const primaryNavigation: NavItem[] = [
  { id: "home", label: "Home" },
  { id: "plan", label: "Plan" },
  { id: "rhythm", label: "Rhythm" },
  { id: "log", label: "Log" },
  { id: "sharing", label: "Sharing" },
];

// The utility group: things you configure once and revisit rarely. They stay
// reachable, and they stop competing with the five you use daily.
export const utilityNavigation: NavItem[] = [
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
  rhythmTab: RhythmTab;
  logTab: LogTab;
  settingsTab: SettingsTab;
}

const planTabs = new Set<PlanTab>(["tasks", "week"]);
const rhythmTabs = new Set<RhythmTab>(["actogram", "drift", "sources"]);
const logTabs = new Set<LogTab>(["sleep", "medications", "context"]);
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
  rhythmTab: "actogram",
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
  if (screen === "rhythm" && rhythmTabs.has(second as RhythmTab)) {
    return { ...defaultRoute, screen, rhythmTab: second as RhythmTab };
  }
  if (screen === "log" && logTabs.has(second as LogTab)) {
    return { ...defaultRoute, screen, logTab: second as LogTab };
  }
  if (screen === "settings" && settingsTabs.has(second as SettingsTab)) {
    return { ...defaultRoute, screen, settingsTab: second as SettingsTab };
  }
  return { ...defaultRoute, screen };
}

export function useScreenNavigation() {
  const [route, setRoute] = useState<Route>(() => readRouteFromHash(window.location.hash));

  useEffect(() => {
    const onHashChange = () => setRoute(readRouteFromHash(window.location.hash));
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  // A new destination opens at its top, and a screen reader lands in it
  // rather than back at the navigation it just used. Switching tabs within a
  // screen keeps both, since the tabs are already where the reader is.
  const openedScreen = useRef(route.screen);
  useEffect(() => {
    if (route.screen === openedScreen.current) return;
    openedScreen.current = route.screen;
    window.scrollTo(0, 0);
    document.getElementById("main-content")?.focus({ preventScroll: true });
  }, [route.screen]);

  // A tab is part of the address: coming back to a bookmarked or reloaded view
  // should land where it did, and the assistant's links can point at one.
  const selectPlanTab = (planTab: PlanTab) => {
    window.location.hash = `#/plan/${planTab}`;
  };
  const selectRhythmTab = (rhythmTab: RhythmTab) => {
    window.location.hash = `#/rhythm/${rhythmTab}`;
  };
  const selectLogTab = (logTab: LogTab) => {
    window.location.hash = `#/log/${logTab}`;
  };
  const selectSettingsTab = (settingsTab: SettingsTab) => {
    window.location.hash = `#/settings/${settingsTab}`;
  };

  return { route, selectPlanTab, selectRhythmTab, selectLogTab, selectSettingsTab };
}

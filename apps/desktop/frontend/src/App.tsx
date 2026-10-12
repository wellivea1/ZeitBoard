import { lazy, Suspense, useEffect, type ReactNode } from "react";
import { AppShell } from "./components/AppShell";
import { useScreenNavigation } from "./navigation";
import { ApprovalsProvider } from "./state/approvals";
import { VisitorRequestsProvider } from "./state/visitorRequests";
import { ApprovalQueueProvider } from "./state/approvalQueue";
import { BackendProposalsProvider } from "./state/backendProposals";
import { AgentProposalsProvider } from "./state/agentProposals";
import { HomeScreen } from "./screens/HomeScreen";
import { ScreenErrorBoundary } from "./components/ScreenErrorBoundary";
import { subscribeBackendEvents } from "./data/backendEvents";
import { loadClockSetting } from "./data/clock";
import { setWindowClock, useWindowClock } from "./utils/windowClock";

const PlanScreen = lazy(() =>
  import("./screens/PlanScreen").then((module) => ({ default: module.PlanScreen })),
);
const LogScreen = lazy(() =>
  import("./screens/LogScreen").then((module) => ({ default: module.LogScreen })),
);
const DataSourcesScreen = lazy(() =>
  import("./screens/DataSourcesScreen").then((module) => ({ default: module.DataSourcesScreen })),
);
const RhythmScreen = lazy(() =>
  import("./screens/RhythmScreen").then((module) => ({ default: module.RhythmScreen })),
);
const SettingsScreen = lazy(() =>
  import("./screens/SettingsScreen").then((module) => ({ default: module.SettingsScreen })),
);
const SharingScreen = lazy(() =>
  import("./screens/SharingScreen").then((module) => ({ default: module.SharingScreen })),
);

function ScreenLoading() {
  return (
    <p className="screen-loading" role="status">
      Opening…
    </p>
  );
}

export default function App() {
  const { route, selectPlanTab, selectRhythmTab, selectLogTab, selectSettingsTab } =
    useScreenNavigation();
  useEffect(subscribeBackendEvents, []);
  // The remembered clock draws the first page; the desktop's answer replaces
  // it if the owner or Windows has since changed it (ADR-0057).
  useEffect(() => {
    void loadClockSetting().then((setting) => {
      if (setting) setWindowClock(setting.clock);
    });
  }, []);
  const clock = useWindowClock();

  const content: ReactNode = {
    home: <HomeScreen />,
    plan: <PlanScreen tab={route.planTab} onSelect={selectPlanTab} />,
    rhythm: <RhythmScreen tab={route.rhythmTab} onSelect={selectRhythmTab} />,
    log: <LogScreen tab={route.logTab} onSelect={selectLogTab} />,
    sharing: <SharingScreen />,
    "data-sources": <DataSourcesScreen />,
    settings: <SettingsScreen tab={route.settingsTab} onSelect={selectSettingsTab} />,
  }[route.screen];

  return (
    <ApprovalsProvider>
      <BackendProposalsProvider>
        <VisitorRequestsProvider>
          <AgentProposalsProvider>
            <ApprovalQueueProvider>
              <a
                className="skip-link"
                href="#main-content"
                onClick={(event) => {
                  event.preventDefault();
                  document.getElementById("main-content")?.focus();
                }}
              >
                Skip to content
              </a>
              <AppShell screen={route.screen}>
                {/* Settings tabs are left out of the key: switching one must not
                  remount the screen and lose a half-typed sync enrollment. The
                  clock is in it everywhere else, so every label, the desktop's
                  and the window's, is drawn again on a new one. */}
                <ScreenErrorBoundary
                  key={`${route.screen}/${route.planTab}/${route.logTab}/${route.screen === "settings" ? "" : clock}`}
                >
                  <Suspense fallback={<ScreenLoading />}>{content}</Suspense>
                </ScreenErrorBoundary>
              </AppShell>
            </ApprovalQueueProvider>
          </AgentProposalsProvider>
        </VisitorRequestsProvider>
      </BackendProposalsProvider>
    </ApprovalsProvider>
  );
}

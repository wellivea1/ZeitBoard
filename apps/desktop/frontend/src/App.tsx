import { lazy, Suspense, type ReactNode } from "react";
import { AppShell, useScreenNavigation } from "./components/AppShell";
import { ApprovalsProvider } from "./state/approvals";
import { BackendProposalsProvider } from "./state/backendProposals";
import { HomeScreen } from "./screens/HomeScreen";

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
    <div className="panel empty-state" role="status">
      Loading view...
    </div>
  );
}

export default function App() {
  const { route, selectPlanTab, selectLogTab } = useScreenNavigation();

  const content: ReactNode = {
    home: <HomeScreen />,
    plan: <PlanScreen tab={route.planTab} onSelect={selectPlanTab} />,
    rhythm: <RhythmScreen />,
    log: <LogScreen tab={route.logTab} onSelect={selectLogTab} />,
    sharing: <SharingScreen />,
    "data-sources": <DataSourcesScreen />,
    settings: <SettingsScreen />,
  }[route.screen];

  return (
    <ApprovalsProvider>
      <BackendProposalsProvider>
        <a className="skip-link" href="#main-content">
          Skip to content
        </a>
        <AppShell screen={route.screen}>
          <Suspense fallback={<ScreenLoading />}>{content}</Suspense>
        </AppShell>
      </BackendProposalsProvider>
    </ApprovalsProvider>
  );
}

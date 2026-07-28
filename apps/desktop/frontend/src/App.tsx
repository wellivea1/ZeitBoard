import { lazy, Suspense, type ReactNode } from "react";
import { AppShell, useScreenNavigation } from "./components/AppShell";
import { ApprovalsProvider } from "./state/approvals";
import { BackendProposalsProvider } from "./state/backendProposals";
import { OverviewScreen } from "./screens/OverviewScreen";

const CalendarScreen = lazy(() =>
  import("./screens/CalendarScreen").then((module) => ({ default: module.CalendarScreen })),
);
const DataSourcesScreen = lazy(() =>
  import("./screens/DataSourcesScreen").then((module) => ({ default: module.DataSourcesScreen })),
);
const MedicationsScreen = lazy(() =>
  import("./screens/MedicationsScreen").then((module) => ({ default: module.MedicationsScreen })),
);
const ApprovalsScreen = lazy(() =>
  import("./screens/ApprovalsScreen").then((module) => ({ default: module.ApprovalsScreen })),
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
const TasksScreen = lazy(() =>
  import("./screens/TasksScreen").then((module) => ({ default: module.TasksScreen })),
);

function ScreenLoading() {
  return (
    <div className="panel empty-state" role="status">
      Loading view...
    </div>
  );
}

export default function App() {
  const screen = useScreenNavigation();

  const content: ReactNode = {
    overview: <OverviewScreen />,
    calendar: <CalendarScreen />,
    tasks: <TasksScreen />,
    approvals: <ApprovalsScreen />,
    rhythm: <RhythmScreen />,
    medications: <MedicationsScreen />,
    sharing: <SharingScreen />,
    "data-sources": <DataSourcesScreen />,
    settings: <SettingsScreen />,
  }[screen];

  return (
    <ApprovalsProvider>
      <BackendProposalsProvider>
        <a className="skip-link" href="#main-content">
          Skip to content
        </a>
        <AppShell screen={screen}>
          <Suspense fallback={<ScreenLoading />}>{content}</Suspense>
        </AppShell>
      </BackendProposalsProvider>
    </ApprovalsProvider>
  );
}

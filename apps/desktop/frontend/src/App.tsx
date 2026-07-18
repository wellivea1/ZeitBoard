import { AppShell, useScreenNavigation } from "./components/AppShell";
import { ApprovalsProvider } from "./state/approvals";
import { OverviewScreen } from "./screens/OverviewScreen";
import { CalendarScreen } from "./screens/CalendarScreen";
import { DataSourcesScreen } from "./screens/DataSourcesScreen";
import { MedicationsScreen } from "./screens/MedicationsScreen";
import { ApprovalsScreen } from "./screens/ApprovalsScreen";
import { RhythmScreen } from "./screens/RhythmScreen";
import { SettingsScreen } from "./screens/SettingsScreen";
import { SharingScreen } from "./screens/SharingScreen";
import { TasksScreen } from "./screens/TasksScreen";

export default function App() {
  const screen = useScreenNavigation();

  const content = {
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
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <AppShell screen={screen}>{content}</AppShell>
    </ApprovalsProvider>
  );
}

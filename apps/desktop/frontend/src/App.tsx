import { AppShell, useScreenNavigation } from "./components/AppShell";
import { OverviewScreen } from "./screens/OverviewScreen";
import {
  CalendarScreen,
  DataSourcesScreen,
  MedicationsScreen,
  SettingsScreen,
  SharingScreen,
  TasksScreen,
  TimelineScreen,
} from "./screens/SecondaryScreens";

export default function App() {
  const screen = useScreenNavigation();

  const content = {
    overview: <OverviewScreen />,
    calendar: <CalendarScreen />,
    tasks: <TasksScreen />,
    timeline: <TimelineScreen />,
    medications: <MedicationsScreen />,
    sharing: <SharingScreen />,
    "data-sources": <DataSourcesScreen />,
    settings: <SettingsScreen />,
  }[screen];

  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <AppShell screen={screen}>{content}</AppShell>
    </>
  );
}

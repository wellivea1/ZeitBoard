import { PageHeader } from "../components/AppShell";
import { ScreenTabPanel, ScreenTabs, type ScreenTab } from "../components/ScreenTabs";
import { usePendingApprovalsCount } from "../state/approvalQueue";
import { CalendarScreen } from "./CalendarScreen";
import { TasksScreen } from "./TasksScreen";
import type { PlanTab } from "../types";

// Plan is two views of one question — what should happen, and when.
//
// Tasks holds the work and every decision about it: a task is added, its
// suggested time appears directly beneath, and it is accepted there. This was
// three tabs (Calendar, Tasks, Approvals), and adding a task on one while
// accepting its time on another was the most awkward loop in the app. The
// pending count stays on the tab, and on Plan in the navigation.

const planTabs = (pending: number): ScreenTab<PlanTab>[] => [
  { id: "tasks", label: "Tasks", badge: pending },
  { id: "calendar", label: "Calendar" },
];

export function PlanScreen({ tab, onSelect }: { tab: PlanTab; onSelect: (tab: PlanTab) => void }) {
  const pending = usePendingApprovalsCount();

  return (
    <>
      <PageHeader title="Plan" />
      <section className="screen-tabbed" aria-label="Plan">
        <ScreenTabs
          name="plan"
          label="Plan views"
          tabs={planTabs(pending)}
          active={tab}
          onSelect={onSelect}
        />

        <ScreenTabPanel name="plan" id="tasks" active={tab}>
          <TasksScreen embedded />
        </ScreenTabPanel>

        <ScreenTabPanel name="plan" id="calendar" active={tab}>
          <CalendarScreen embedded />
        </ScreenTabPanel>
      </section>
    </>
  );
}

import { PageHeader } from "../components/AppShell";
import { ScreenTabPanel, ScreenTabs, type ScreenTab } from "../components/ScreenTabs";
import { usePendingApprovalsCount } from "../state/approvals";
import { ApprovalsScreen } from "./ApprovalsScreen";
import { CalendarScreen } from "./CalendarScreen";
import { TasksScreen } from "./TasksScreen";
import type { PlanTab } from "../types";

// Plan absorbs Calendar, Tasks and Approvals (slice U-H).
//
// They were three equal-weight destinations answering one question — what is
// happening and what should be — and Approvals in particular was a permanent
// destination that is empty most of the time. A destination that is usually
// empty teaches people to skip it, which is the last thing a queue holding
// every pending change should do. It is a tab with a count instead: invisible
// when there is nothing, and impossible to miss when there is.

const planTabs = (pending: number): ScreenTab<PlanTab>[] => [
  { id: "calendar", label: "Calendar" },
  { id: "tasks", label: "Tasks" },
  { id: "approvals", label: "Approvals", badge: pending },
];

export function PlanScreen({ tab, onSelect }: { tab: PlanTab; onSelect: (tab: PlanTab) => void }) {
  const pending = usePendingApprovalsCount();

  return (
    <>
      <PageHeader
        title="Plan"
        description="Fixed events, flexible tasks, and every change waiting for your decision."
      />
      <section className="screen-tabbed" aria-label="Plan">
        <ScreenTabs
          name="plan"
          label="Plan views"
          tabs={planTabs(pending)}
          active={tab}
          onSelect={onSelect}
        />

        <ScreenTabPanel name="plan" id="calendar" active={tab}>
          <CalendarScreen embedded />
        </ScreenTabPanel>

        <ScreenTabPanel name="plan" id="tasks" active={tab}>
          <TasksScreen embedded />
        </ScreenTabPanel>

        <ScreenTabPanel name="plan" id="approvals" active={tab}>
          <ApprovalsScreen embedded />
        </ScreenTabPanel>
      </section>
    </>
  );
}

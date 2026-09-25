import { PageHeader } from "../components/AppShell";
import { ScreenTabPanel, ScreenTabs, type ScreenTab } from "../components/ScreenTabs";
import { usePendingApprovalsCount } from "../state/approvalQueue";
import { TasksScreen } from "./TasksScreen";
import { WeekScreen } from "./WeekScreen";
import type { PlanTab } from "../types";

// Plan is two views of one question — what should happen, and when.
//
// Tasks holds the work and every decision about it: a task is added, its
// suggested time appears directly beneath, and it is accepted there. Week is
// the same plan on a calendar, with the sleep forecast painted into the days,
// for arranging things rather than listing them; suggestions can be decided
// on either. The pending count stays on the Tasks tab, and on Plan in the
// navigation.

const planTabs = (pending: number): ScreenTab<PlanTab>[] => [
  { id: "tasks", label: "Tasks", badge: pending },
  { id: "week", label: "Week" },
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

        <ScreenTabPanel name="plan" id="week" active={tab}>
          <WeekScreen />
        </ScreenTabPanel>
      </section>
    </>
  );
}

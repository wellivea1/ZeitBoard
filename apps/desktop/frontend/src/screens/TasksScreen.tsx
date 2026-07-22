import { useEffect, useState, type FormEvent } from "react";
import { Icon } from "../components/Icon";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";
import { ProposalCard } from "../components/ProposalCard";
import { useApprovals } from "../state/approvals";
import {
  addTask,
  deleteTask,
  loadTasks,
  setTaskDone,
  type Task,
  type TasksData,
} from "../data/tasks";

function TaskRow({
  task,
  busy,
  onToggleDone,
  onDelete,
}: {
  task: Task;
  busy: boolean;
  onToggleDone: (task: Task) => void;
  onDelete: (task: Task) => void;
}) {
  const details = [task.durationLabel, task.windowLabel, task.afterWakeLabel]
    .filter(Boolean)
    .join(" · ");
  return (
    <div className="task-row" role="row" data-status={task.status}>
      <span role="cell">
        <input
          type="checkbox"
          checked={task.status === "done"}
          disabled={busy}
          onChange={() => onToggleDone(task)}
          aria-label={`Mark ${task.title} ${task.status === "done" ? "open" : "done"}`}
        />
        {task.title}
      </span>
      <span role="cell">{details}</span>
      <span role="cell">
        <span className="task-chip">{task.status === "done" ? "Done" : "Open"}</span>
      </span>
      <span role="cell">
        <button
          className="button secondary"
          type="button"
          disabled={busy}
          onClick={() => onDelete(task)}
        >
          Delete
        </button>
      </span>
    </div>
  );
}

export function TasksScreen() {
  const { pending, pendingCount, unplaced } = useApprovals();
  const firstUnplaced = unplaced[0];
  const [data, setData] = useState<TasksData>({ status: "unavailable", tasks: [] });
  const [title, setTitle] = useState("");
  const [durationMinutes, setDurationMinutes] = useState(45);
  const [latestFinishLocal, setLatestFinishLocal] = useState("");
  const [afterWakeMinutes, setAfterWakeMinutes] = useState(0);
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState("");
  const [announcement, setAnnouncement] = useState("");

  useEffect(() => {
    let current = true;
    void loadTasks().then((result) => {
      if (current) setData(result);
    });
    return () => {
      current = false;
    };
  }, []);

  const available = data.status === "ok";
  const openTasks = data.tasks.filter((task) => task.status === "open");
  const doneTasks = data.tasks.filter((task) => task.status === "done");

  const runMutation = (
    operation: Promise<TasksData>,
    successMessage: string,
    onSuccess?: () => void,
  ) => {
    setBusy(true);
    setFormError("");
    setAnnouncement("");
    operation
      .then((result) => {
        setData(result);
        setAnnouncement(successMessage);
        onSuccess?.();
      })
      .catch((error: unknown) => {
        setFormError(error instanceof Error ? error.message : "The task action failed.");
      })
      .finally(() => setBusy(false));
  };

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const submittedTitle = title.trim();
    runMutation(
      addTask({
        title: submittedTitle,
        durationMinutes,
        ...(latestFinishLocal ? { latestFinishLocal } : {}),
        ...(afterWakeMinutes > 0 ? { preferredAfterWakeMinutes: afterWakeMinutes } : {}),
        zoneId: Intl.DateTimeFormat().resolvedOptions().timeZone,
      }),
      `Added ${submittedTitle}.`,
      () => {
        setTitle("");
        setLatestFinishLocal("");
        setAfterWakeMinutes(0);
      },
    );
  };

  return (
    <>
      <PageHeader
        title="Tasks"
        description="Describe flexibility and effort; the planner returns proposals, not calendar changes."
      />
      {!available && (
        <PlaceholderNotice>
          {data.message ??
            "This browser preview is read-only. Open the ZeitBoard desktop app to manage tasks."}
        </PlaceholderNotice>
      )}
      <section className="screen-grid" aria-label="Task planning and approvals">
        <section
          className="panel phase-two-panel approval-summary"
          aria-labelledby="approval-title"
        >
          <div className="panel-heading">
            <div>
              <p className="section-kicker">Proposal review</p>
              <h2 id="approval-title">Approval queue</h2>
            </div>
            <a href="#/approvals">
              Open all <Icon name="chevron" />
            </a>
          </div>
          <p className="phase-two-copy">
            {pendingCount > 0
              ? `${pendingCount} pending ${pendingCount === 1 ? "proposal is" : "proposals are"} waiting for explicit approval.`
              : "No proposals are waiting for approval."}
          </p>
          {pending.slice(0, 1).map((proposal) => (
            <ProposalCard proposal={proposal} key={proposal.id} />
          ))}
          <aside className="unplaced-row" aria-labelledby="unplaced-title">
            <div>
              <p className="section-kicker">Not proposed</p>
              <h3 id="unplaced-title">
                {firstUnplaced ? firstUnplaced.title : "Nothing waiting on a window"}
              </h3>
            </div>
            <div>
              <p>
                {firstUnplaced
                  ? firstUnplaced.reason
                  : "Every open task either has a proposal or there are no open tasks."}
              </p>
              {firstUnplaced && <small>{firstUnplaced.nextAction}</small>}
            </div>
          </aside>
        </section>

        {available && (
          <section className="panel table-panel task-list-panel" aria-labelledby="add-task-title">
            <div className="panel-heading">
              <div>
                <p className="section-kicker">Flexible work</p>
                <h2 id="add-task-title">Add a task</h2>
              </div>
            </div>
            <form className="sleep-entry-fields" onSubmit={onSubmit}>
              <label>
                Task
                <input
                  type="text"
                  value={title}
                  maxLength={120}
                  required
                  disabled={busy}
                  onChange={(event) => setTitle(event.target.value)}
                />
              </label>
              <label>
                Duration (minutes)
                <input
                  type="number"
                  min={5}
                  max={720}
                  value={durationMinutes}
                  required
                  disabled={busy}
                  onChange={(event) => setDurationMinutes(Number(event.target.value))}
                />
              </label>
              <label>
                Finish by (optional)
                <input
                  type="datetime-local"
                  value={latestFinishLocal}
                  disabled={busy}
                  onChange={(event) => setLatestFinishLocal(event.target.value)}
                />
              </label>
              <label>
                Minutes after waking (optional)
                <input
                  type="number"
                  min={0}
                  max={1440}
                  value={afterWakeMinutes}
                  disabled={busy}
                  onChange={(event) => setAfterWakeMinutes(Number(event.target.value))}
                />
              </label>
              <button className="button primary" type="submit" disabled={busy || !title.trim()}>
                Save task
              </button>
            </form>
            {formError && (
              <p className="diff-note" role="alert">
                {formError}
              </p>
            )}

            <div className="panel-heading">
              <div>
                <p className="section-kicker">Your tasks</p>
                <h2 id="task-list-title">
                  {openTasks.length} open{doneTasks.length > 0 && `, ${doneTasks.length} done`}
                </h2>
              </div>
            </div>
            {data.tasks.length > 0 ? (
              <div className="task-table" role="table" aria-labelledby="task-list-title">
                <div className="task-row task-head" role="row">
                  <span role="columnheader">Task</span>
                  <span role="columnheader">Constraints</span>
                  <span role="columnheader">Status</span>
                  <span role="columnheader">Actions</span>
                </div>
                {data.tasks.map((task) => (
                  <TaskRow
                    task={task}
                    busy={busy}
                    onToggleDone={(target) =>
                      runMutation(
                        setTaskDone(target.taskId, target.status !== "done"),
                        `${target.title} marked ${target.status === "done" ? "open" : "done"}.`,
                      )
                    }
                    onDelete={(target) =>
                      runMutation(deleteTask(target.taskId), `Deleted ${target.title}.`)
                    }
                    key={task.taskId}
                  />
                ))}
              </div>
            ) : (
              <p className="phase-two-copy">
                No tasks yet. Add flexible work above and the planner will propose windows inside
                your predicted waking time — nothing is scheduled without your approval.
              </p>
            )}
            <p role="status" aria-live="polite" className="sr-only">
              {announcement}
            </p>
          </section>
        )}
      </section>
    </>
  );
}

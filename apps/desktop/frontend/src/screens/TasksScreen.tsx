import { useEffect, useRef, useState } from "react";
import { Icon } from "../components/Icon";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";
import { ProposalCard } from "../components/ProposalCard";
import { TaskEditor } from "../components/TaskEditor";
import { useApprovals } from "../state/approvals";
import {
  addTask,
  updateTask,
  deleteTask,
  loadTasks,
  setTaskDone,
  type Task,
  type TaskInput,
  type TasksData,
} from "../data/tasks";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import { hasDesktopBridge } from "../data/wailsBridge";
import { createCoalescedRefresh } from "../utils/coalescedRefresh";
import { subscribeProjectionRefresh } from "../utils/projectionRefresh";

function TaskRow({
  task,
  busy,
  onToggleDone,
  onEdit,
  onDelete,
}: {
  task: Task;
  busy: boolean;
  onToggleDone: (task: Task) => void;
  onEdit: (task: Task) => void;
  onDelete: (task: Task) => void;
}) {
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
      <span role="cell">
        {[task.durationLabel, task.windowLabel, task.afterWakeLabel].filter(Boolean).join(" · ")}
      </span>
      <span role="cell">
        <span className="task-chip">{task.status === "done" ? "Done" : "Open"}</span>
      </span>
      <span role="cell" className="task-actions">
        <button
          className="button secondary"
          type="button"
          disabled={busy || !task.editable}
          aria-label={`Edit ${task.title}`}
          onClick={() => onEdit(task)}
        >
          Edit
        </button>
        <button
          className="button secondary"
          type="button"
          disabled={busy}
          aria-label={`Delete ${task.title}`}
          onClick={() => onDelete(task)}
        >
          Delete
        </button>
      </span>
    </div>
  );
}

export function TasksScreen({ embedded }: { embedded?: boolean } = {}) {
  const { pending, pendingCount, unplaced, error: proposalError } = useApprovals();
  const firstUnplaced = unplaced[0];
  const [data, setData] = useState<TasksData>({
    status: "unavailable",
    tasks: [],
    message: "Loading your tasks…",
  });
  const [editing, setEditing] = useState<Task | null>(null);
  const [deleting, setDeleting] = useState<Task | null>(null);
  const [editorVersion, setEditorVersion] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [readError, setReadError] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const busyRef = useRef(false);
  const refreshRef = useRef<ReturnType<typeof createCoalescedRefresh> | null>(null);
  const editorRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const refresh = createCoalescedRefresh(loadTasks, (result) => {
      setReadError(
        result.status === "ok" ? "" : (result.message ?? "Tasks could not be refreshed."),
      );
      setData((current) => (result.status === "ok" || current.status !== "ok" ? result : current));
    });
    refreshRef.current = refresh;
    const unsubscribe = subscribeProjectionRefresh(() => {
      if (!busyRef.current) refresh.request();
    }, sleepDataChangedEvent);
    return () => {
      unsubscribe();
      refresh.dispose();
      refreshRef.current = null;
    };
  }, []);

  const runMutation = async (
    operation: () => Promise<TasksData>,
    success: string,
    resetEditor = false,
  ) => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setAnnouncement("");
    refreshRef.current?.supersede();
    try {
      setData(await operation());
      setReadError("");
      setAnnouncement(success);
      setDeleting(null);
      if (resetEditor) {
        setEditing(null);
        setEditorVersion((version) => version + 1);
      }
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "The task action failed. Refresh and try again.",
      );
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
  };
  const save = (input: TaskInput) =>
    void runMutation(
      () => (editing ? updateTask(input) : addTask(input)),
      `${editing ? "Updated" : "Added"} ${input.title}. Review proposals to place it on your calendar.`,
      true,
    );
  const edit = (task: Task) => {
    setEditing(task);
    setDeleting(null);
    setError("");
    setEditorVersion((version) => version + 1);
    editorRef.current?.scrollIntoView?.({ block: "nearest" });
  };
  const available = data.status === "ok";
  const openCount = data.tasks.filter((task) => task.status === "open").length;

  return (
    <>
      <PageHeader
        title="Tasks"
        description="Add flexible work, then review a proposed time before it reaches your calendar."
        level={embedded ? "panel" : "page"}
        actions={
          hasDesktopBridge() && (
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => refreshRef.current?.request()}
            >
              Refresh tasks
            </button>
          )
        }
      />
      {!available && <PlaceholderNotice>{data.message}</PlaceholderNotice>}
      <section className="task-workspace" aria-label="Task planning and approvals">
        {available && readError && (
          <p role="alert">
            Latest refresh failed. Showing the last loaded tasks; your draft is kept. {readError}
          </p>
        )}
        {available && (
          <section className="task-list-panel" aria-label="Your tasks">
            <div ref={editorRef}>
              <TaskEditor
                key={editorVersion}
                task={editing}
                busy={busy}
                error={deleting ? "" : error}
                onSave={save}
                onCancel={() => {
                  setEditing(null);
                  setError("");
                  setEditorVersion((version) => version + 1);
                }}
              />
            </div>
            <p className="task-feedback" role="status">
              {announcement}
            </p>
            <div className="panel-heading">
              <h2 id="task-list-title">
                {openCount} open · {data.tasks.length - openCount} done
              </h2>
            </div>
            {deleting && (
              <section className="task-delete-confirmation" aria-label={`Delete ${deleting.title}`}>
                <strong>Delete “{deleting.title}” permanently?</strong>
                <p>
                  This removes the task and syncs its deletion to your enrolled devices. You can
                  mark it done instead to keep it.
                </p>
                {error && <p role="alert">{error}</p>}
                <div className="page-actions">
                  <button
                    className="button danger"
                    disabled={busy}
                    onClick={() =>
                      void runMutation(
                        () => deleteTask(deleting.taskId, deleting.revision),
                        `Deleted ${deleting.title}.`,
                        editing?.taskId === deleting.taskId,
                      )
                    }
                  >
                    Delete task
                  </button>
                  <button
                    className="button secondary"
                    disabled={busy}
                    onClick={() => {
                      setDeleting(null);
                      setError("");
                    }}
                  >
                    Keep task
                  </button>
                </div>
              </section>
            )}
            {data.tasks.length ? (
              <div className="task-table" role="table" aria-labelledby="task-list-title">
                <div className="task-row task-head" role="row">
                  <span role="columnheader">Task</span>
                  <span role="columnheader">Constraints</span>
                  <span role="columnheader">Status</span>
                  <span role="columnheader">Actions</span>
                </div>
                {[...data.tasks]
                  .sort((a, b) => Number(a.status === "done") - Number(b.status === "done"))
                  .map((task) => (
                    <TaskRow
                      key={task.taskId}
                      task={task}
                      busy={busy}
                      onEdit={edit}
                      onDelete={(target) => {
                        setDeleting(target);
                        setError("");
                      }}
                      onToggleDone={(target) =>
                        void runMutation(
                          () =>
                            setTaskDone(target.taskId, target.revision, target.status !== "done"),
                          `${target.title} marked ${target.status === "done" ? "open" : "done"}.`,
                        )
                      }
                    />
                  ))}
              </div>
            ) : (
              <p className="phase-two-copy">
                No tasks yet. Start with a name and duration. Timing constraints are optional.
              </p>
            )}
            {data.tasks.some((task) => !task.editable) && (
              <p className="phase-two-copy">
                Update the desktop app to edit all saved timing constraints.
              </p>
            )}
          </section>
        )}

        <section
          className="approval-summary task-proposal-summary"
          aria-labelledby="approval-title"
        >
          {proposalError && (
            <p role="alert">
              {proposalError} <a href="#/plan/approvals">Review queue</a>
            </p>
          )}
          <div className="panel-heading">
            <h2 id="approval-title">Proposed times</h2>
            <a href="#/plan/approvals">
              Review all <Icon name="chevron" />
            </a>
          </div>
          <p className="phase-two-copy">
            {pendingCount > 0
              ? `${pendingCount} pending ${pendingCount === 1 ? "proposal needs" : "proposals need"} your approval.`
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
                  : "Open tasks with no feasible window will appear here."}
              </p>
              {firstUnplaced && <small>{firstUnplaced.nextAction}</small>}
            </div>
          </aside>
        </section>
      </section>
    </>
  );
}

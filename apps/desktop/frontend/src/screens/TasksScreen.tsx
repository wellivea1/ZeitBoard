import { useEffect, useRef, useState } from "react";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";
import { DecisionHistory, DecisionQueue } from "../components/DecisionQueue";
import { TaskEditor } from "../components/TaskEditor";
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
import { calendarDataChangedEvent } from "../data/calendar";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import { hasDesktopBridge } from "../data/wailsBridge";
import { createCoalescedRefresh } from "../utils/coalescedRefresh";
import { subscribeProjectionRefresh } from "../utils/projectionRefresh";
import { blockWording } from "../utils/relativeTime";

// One line per task: what it is, when it is, and what can be done to it. The
// list used to be a four-column table whose buttons wrapped under each row at
// most window widths, and it never said which tasks already had a time.
function TaskItem({
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
  const details = [task.durationLabel, task.windowLabel, task.afterWakeLabel]
    .filter(Boolean)
    .join(" · ");
  const scheduled = task.status === "open" ? task.scheduled : undefined;
  return (
    <li className="task-item" data-status={task.status}>
      <input
        type="checkbox"
        checked={task.status === "done"}
        disabled={busy || task.needsReview}
        onChange={() => onToggleDone(task)}
        aria-label={`Mark ${task.title} ${task.status === "done" ? "open" : "done"}`}
      />
      <div className="task-item-text">
        <strong>{task.title}</strong>
        <span>
          {scheduled && (
            <>
              <span className="task-scheduled" title={scheduled.label}>
                {blockWording(scheduled.startAt, scheduled.endAt, scheduled.label)}
              </span>
              {" · "}
            </>
          )}
          {details}
        </span>
        {task.needsReview && <span className="task-chip">Choose a version above</span>}
      </div>
      <div className="task-actions">
        <button
          className="button ghost compact"
          type="button"
          disabled={busy || !task.editable}
          aria-label={`Edit ${task.title}`}
          onClick={() => onEdit(task)}
        >
          Edit
        </button>
        <button
          className="button ghost compact"
          type="button"
          disabled={busy}
          aria-label={`Delete ${task.title}`}
          onClick={() => onDelete(task)}
        >
          Delete
        </button>
      </div>
    </li>
  );
}

export function TasksScreen({ embedded }: { embedded?: boolean } = {}) {
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
    const request = () => {
      if (!busyRef.current) refresh.request();
    };
    const unsubscribe = subscribeProjectionRefresh(request, sleepDataChangedEvent);
    // Accepting or undoing a suggested time writes the calendar, and each
    // task row shows its accepted time.
    window.addEventListener(calendarDataChangedEvent, request);
    return () => {
      unsubscribe();
      window.removeEventListener(calendarDataChangedEvent, request);
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
      `${editing ? "Updated" : "Added"} ${input.title}. Its suggested time appears under Needs your decision.`,
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
      {!embedded && <PageHeader title="Tasks" />}
      {!available && (
        <PlaceholderNotice>
          {data.message}{" "}
          {/* Recovery sits with the failure. A permanent Refresh button on a
              list that refreshes itself was noise the rest of the time. */}
          {hasDesktopBridge() && (
            <button
              className="button secondary compact"
              type="button"
              onClick={() => refreshRef.current?.request()}
            >
              Try again
            </button>
          )}
        </PlaceholderNotice>
      )}
      <section className="task-workspace" aria-label="Tasks and decisions">
        {available && readError && (
          <p role="alert">
            Latest refresh failed. Showing the last loaded tasks; your draft is kept. {readError}
          </p>
        )}
        {available && (
          <section className="task-add-panel" aria-label="Add a task">
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
          </section>
        )}
        <DecisionQueue />
        {available && (
          <section className="task-list-panel" aria-label="Your tasks">
            <div className="plan-section-head">
              <h2 id="task-list-title">
                Your tasks <span className="count">{openCount}</span>
              </h2>
              {data.tasks.length - openCount > 0 && (
                <small>{data.tasks.length - openCount} done</small>
              )}
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
              <ul className="task-list" aria-labelledby="task-list-title">
                {[...data.tasks]
                  .sort((a, b) => Number(a.status === "done") - Number(b.status === "done"))
                  .map((task) => (
                    <TaskItem
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
              </ul>
            ) : (
              <p className="plan-empty">
                No tasks yet. Add one above with a name and how long it takes.
              </p>
            )}
          </section>
        )}

        <DecisionHistory />
      </section>
    </>
  );
}

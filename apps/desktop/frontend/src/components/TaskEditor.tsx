import { useState, type FormEvent } from "react";
import type { Task, TaskInput } from "../data/tasks";

function civilInput(instant?: string): string {
  if (!instant) return "";
  const date = new Date(instant);
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function TaskEditor({
  task,
  busy,
  error,
  onSave,
  onCancel,
}: {
  task: Task | null;
  busy: boolean;
  error: string;
  onSave: (input: TaskInput) => void;
  onCancel: () => void;
}) {
  const [title, setTitle] = useState(task?.title ?? "");
  const [duration, setDuration] = useState(task?.durationMinutes ?? 45);
  const [start, setStart] = useState(civilInput(task?.earliestStartAt));
  const [finish, setFinish] = useState(civilInput(task?.latestFinishAt));
  const [afterWake, setAfterWake] = useState(task?.preferredAfterWakeMinutes ?? 0);
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSave({
      ...(task ? { taskId: task.taskId, revision: task.revision } : {}),
      title: title.trim(),
      durationMinutes: duration,
      zoneId: zone,
      // Preserve an unchanged instant exactly, even in a repeated DST hour.
      ...(start && start === civilInput(task?.earliestStartAt)
        ? { earliestStartAt: task?.earliestStartAt }
        : { earliestStartLocal: start }),
      ...(finish && finish === civilInput(task?.latestFinishAt)
        ? { latestFinishAt: task?.latestFinishAt }
        : { latestFinishLocal: finish }),
      preferredAfterWakeMinutes: afterWake,
      minimumConfidence: task?.minimumConfidence ?? "",
    });
  };

  return (
    <section className="task-editor" aria-labelledby="task-editor-title">
      <h2 id="task-editor-title">{task ? "Edit task" : "Add a task"}</h2>
      <form onSubmit={submit} className="sleep-entry-fields">
        <label>
          Task
          <input
            name="title"
            autoFocus={Boolean(task)}
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
            step={1}
            value={duration}
            required
            disabled={busy}
            onChange={(event) => setDuration(Number(event.target.value))}
          />
        </label>
        <details className="task-timing" open={task ? true : undefined}>
          <summary>Timing constraints (optional)</summary>
          <p>Times are shown in {zone}. Leave blank for any predicted waking window.</p>
          <div className="sleep-entry-fields">
            <label>
              Not before
              <input
                type="datetime-local"
                value={start}
                disabled={busy}
                onChange={(event) => setStart(event.target.value)}
              />
            </label>
            <label>
              Finish by
              <input
                type="datetime-local"
                value={finish}
                min={start || undefined}
                disabled={busy}
                onChange={(event) => setFinish(event.target.value)}
              />
            </label>
            <label>
              Minutes after waking
              <input
                type="number"
                min={0}
                max={1440}
                step={1}
                value={afterWake}
                disabled={busy}
                onChange={(event) => setAfterWake(Number(event.target.value))}
              />
            </label>
          </div>
        </details>
        {error && (
          <p className="task-feedback" role="alert">
            {error}
          </p>
        )}
        <div className="page-actions task-editor-actions">
          <button className="button primary" type="submit" disabled={busy || !title.trim()}>
            {busy ? "Saving…" : task ? "Save changes" : "Save task"}
          </button>
          {task && (
            <button className="button secondary" type="button" disabled={busy} onClick={onCancel}>
              Cancel edit
            </button>
          )}
        </div>
      </form>
    </section>
  );
}

import { useState } from "react";
import { Notice } from "./Notice";
import { useApprovals } from "../state/approvals";
import type { TaskConflict, TaskConflictHistory } from "../data/taskConflicts";
import type { Task } from "../data/tasks";

function exactTime(value: string | undefined) {
  return value
    ? new Date(value).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })
    : "No limit";
}

// The fields a person chooses between. Listing all of them for every version
// buried the one or two that actually differ under rows of identical values,
// so the options show what differs and the rest is stated once.
const conflictFields: { label: string; value: (task: Task) => string }[] = [
  { label: "Task", value: (task) => task.title },
  { label: "Duration", value: (task) => task.durationLabel },
  { label: "Status", value: (task) => (task.status === "done" ? "Done" : "Open") },
  { label: "Not before", value: (task) => exactTime(task.earliestStartAt) },
  { label: "Finish by", value: (task) => exactTime(task.latestFinishAt) },
  { label: "After waking", value: (task) => task.afterWakeLabel || "No preference" },
  { label: "Minimum confidence", value: (task) => task.minimumConfidence || "Low" },
];
function TaskVersionDetails({ task }: { task: Task }) {
  return (
    <dl className="task-version-details">
      <div>
        <dt>Task</dt>
        <dd>{task.title}</dd>
      </div>
      <div>
        <dt>Duration</dt>
        <dd>{task.durationLabel}</dd>
      </div>
      <div>
        <dt>Status</dt>
        <dd>{task.status === "done" ? "Done" : "Open"}</dd>
      </div>
      <div>
        <dt>Not before</dt>
        <dd>{exactTime(task.earliestStartAt)}</dd>
      </div>
      <div>
        <dt>Finish by</dt>
        <dd>{exactTime(task.latestFinishAt)}</dd>
      </div>
      <div>
        <dt>After waking</dt>
        <dd>{task.afterWakeLabel || "No preference"}</dd>
      </div>
      <div>
        <dt>Minimum confidence</dt>
        <dd>{task.minimumConfidence || "Low"}</dd>
      </div>
      <div>
        <dt>Updated</dt>
        <dd>{exactTime(task.updatedAt)}</dd>
      </div>
    </dl>
  );
}
export function TaskConflictCard({ conflict }: { conflict: TaskConflict }) {
  const queue = useApprovals();
  const [choice, setChoice] = useState("");
  const busy = queue.busyProposalId !== null || Boolean(queue.loadError);
  const versions = [{ choiceId: "local", task: conflict.local }, ...conflict.downloaded];
  const differing = conflictFields.filter(
    (field) => new Set(versions.map((version) => field.value(version.task))).size > 1,
  );
  const same = conflictFields.filter((field) => !differing.includes(field));
  return (
    <article className="proposal-card task-conflict-card" data-origin="sync_conflict">
      <p className="section-kicker">Conflicting task edits</p>
      <h2>{conflict.local.title}</h2>
      <p>Changed on two devices. Until you keep one version it stays out of planning and upload.</p>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (choice) void queue.resolveTaskConflict(conflict, choice);
        }}
      >
        <fieldset disabled={busy}>
          <legend>Compare all retained versions</legend>
          <div className="task-conflict-versions">
            {versions.map((version) => {
              const id = `task-choice-${conflict.taskId}-${version.choiceId}`;
              return (
                <div className="task-conflict-version" key={version.choiceId}>
                  {/* Named from its heading alone. Wrapping the details in the
                      label left the radio with no usable name in Chromium. */}
                  <input
                    id={id}
                    type="radio"
                    name={`task-choice-${conflict.taskId}`}
                    value={version.choiceId}
                    checked={choice === version.choiceId}
                    onChange={() => setChoice(version.choiceId)}
                    aria-describedby={`${id}-details`}
                  />
                  <label htmlFor={id}>
                    {version.choiceId === "local"
                      ? "Keep local version"
                      : `Use downloaded revision ${version.task.revision}`}
                  </label>
                  <dl className="task-version-details" id={`${id}-details`}>
                    {differing.map((field) => (
                      <div key={field.label}>
                        <dt>{field.label}</dt>
                        <dd>{field.value(version.task)}</dd>
                      </div>
                    ))}
                    <div className="task-version-edited">
                      <dt>Edited</dt>
                      <dd>{exactTime(version.task.updatedAt)}</dd>
                    </div>
                  </dl>
                </div>
              );
            })}
          </div>
          {same.length > 0 && (
            <p className="task-conflict-same">
              Same in every version:{" "}
              {same.map((field) => `${field.label} ${field.value(conflict.local)}`).join(" · ")}
            </p>
          )}
        </fieldset>
        <Notice id="tasks.conflict">
          The version you keep is saved as a new revision and synced. Calendar placements stay as
          they are, the versions you compared stay in history, and you can edit the task afterwards.
        </Notice>
        <button className="button primary" type="submit" disabled={busy || !choice}>
          {queue.busyProposalId === conflict.taskId ? "Saving review..." : "Save selected version"}
        </button>
      </form>
    </article>
  );
}
export function TaskConflictHistoryCard({ item }: { item: TaskConflictHistory }) {
  return (
    <details className="task-review-history">
      <summary>
        Resolved task: {item.result.title} · {item.decidedLabel}
      </summary>
      <p>
        {item.choice} kept as revision {item.result.revision}. No calendar placement was changed.
      </p>
      <div className="task-conflict-versions">
        <section>
          <h3>Chosen result</h3>
          <TaskVersionDetails task={item.result} />
        </section>
        <section>
          <h3>Previous local version</h3>
          <TaskVersionDetails task={item.before.local} />
        </section>
        {item.before.downloaded.map((version) => (
          <section key={version.choiceId}>
            <h3>Downloaded revision {version.task.revision}</h3>
            <TaskVersionDetails task={version.task} />
          </section>
        ))}
      </div>
      <a href="#/plan/tasks">Open Tasks to edit the current version</a>
    </details>
  );
}

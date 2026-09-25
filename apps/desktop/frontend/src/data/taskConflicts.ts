import { normalizeTask, type Task } from "./tasks";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export interface TaskConflictVersion {
  choiceId: string;
  task: Task;
}
export interface TaskConflict {
  taskId: string;
  reviewToken: string;
  local: Task;
  downloaded: TaskConflictVersion[];
}
export interface TaskConflictHistory {
  taskId: string;
  reviewToken: string;
  choice: string;
  decidedLabel: string;
  result: Task;
  before: TaskConflict;
}
function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
export function normalizeTaskConflict(value: unknown): TaskConflict | undefined {
  if (
    !record(value) ||
    typeof value.taskId !== "string" ||
    typeof value.reviewToken !== "string" ||
    !value.reviewToken.startsWith("task_review_") ||
    !Array.isArray(value.downloaded)
  )
    return undefined;
  const local = normalizeTask(value.local);
  if (!local || local.taskId !== value.taskId) return undefined;
  const downloaded: TaskConflictVersion[] = [];
  for (const item of value.downloaded) {
    if (
      !record(item) ||
      typeof item.choiceId !== "string" ||
      !item.choiceId.startsWith("task_choice_")
    )
      return undefined;
    const task = normalizeTask(item.task);
    if (!task || task.taskId !== value.taskId) return undefined;
    downloaded.push({ choiceId: item.choiceId, task });
  }
  if (downloaded.length === 0) return undefined;
  return { taskId: value.taskId, reviewToken: value.reviewToken, local, downloaded };
}
export function normalizeTaskConflictHistory(value: unknown): TaskConflictHistory | undefined {
  if (
    !record(value) ||
    typeof value.taskId !== "string" ||
    typeof value.reviewToken !== "string" ||
    typeof value.choice !== "string" ||
    typeof value.decidedLabel !== "string"
  )
    return undefined;
  const before = normalizeTaskConflict(value.before),
    result = normalizeTask(value.result);
  if (
    !before ||
    !result ||
    before.taskId !== value.taskId ||
    result.taskId !== value.taskId ||
    before.reviewToken !== value.reviewToken
  )
    return undefined;
  return {
    taskId: value.taskId,
    reviewToken: value.reviewToken,
    choice: value.choice,
    decidedLabel: value.decidedLabel,
    before,
    result,
  };
}
export async function resolveTaskConflict(
  input: { taskId: string; reviewToken: string; choiceId: string },
  root: WailsRoot = globalThis as WailsRoot,
): Promise<TaskConflictHistory> {
  const method = findWailsMethod(root, ["ResolveTaskConflict"]);
  if (!method) throw new Error("Task conflict review requires the desktop app.");
  const result = normalizeTaskConflictHistory(await method(input));
  if (!result) throw new Error("Could not confirm the task review. Refresh before retrying.");
  return result;
}

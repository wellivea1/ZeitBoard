import { notifySleepDataChanged } from "./sleepDataEvents";
import { findWailsMethod, hasDesktopBridge, type WailsRoot } from "./wailsBridge";

// User-owned flexible tasks (ADR-0018): real planning items the scheduler
// proposes windows for. Titles are private user text and stay local.

export interface Task {
  taskId: string;
  revision: number;
  title: string;
  durationMinutes: number;
  durationLabel: string;
  status: "open" | "done";
  windowLabel?: string;
  afterWakeLabel?: string;
  createdLabel: string;
  earliestStartAt?: string;
  latestFinishAt?: string;
  preferredAfterWakeMinutes?: number;
  minimumConfidence?: string;
  editable: boolean;
}

export interface TasksData {
  status: "ok" | "unavailable";
  message?: string;
  tasks: Task[];
}

export interface TaskInput {
  taskId?: string;
  revision?: number;
  title: string;
  durationMinutes: number;
  earliestStartLocal?: string;
  latestFinishLocal?: string;
  earliestStartAt?: string;
  latestFinishAt?: string;
  zoneId?: string;
  preferredAfterWakeMinutes?: number;
  minimumConfidence?: string;
}

type UnknownRecord = Record<string, unknown>;

const unavailable: TasksData = {
  status: "unavailable",
  message: "This browser preview is read-only. Open the ZeitBoard desktop app to manage tasks.",
  tasks: [],
};

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function positiveInteger(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0 ? value : undefined;
}

function normalizeTask(value: unknown): Task | undefined {
  if (!isRecord(value)) return undefined;
  const taskId = str(value.taskId);
  const title = str(value.title);
  const durationLabel = str(value.durationLabel);
  const createdLabel = str(value.createdLabel);
  const status = value.status === "open" || value.status === "done" ? value.status : undefined;
  const durationMinutes =
    typeof value.durationMinutes === "number" && Number.isInteger(value.durationMinutes)
      ? value.durationMinutes
      : undefined;
  const revision = positiveInteger(value.revision);
  if (
    !taskId ||
    !title ||
    !durationLabel ||
    !createdLabel ||
    !status ||
    revision === undefined ||
    durationMinutes === undefined
  ) {
    return undefined;
  }
  const windowLabel = str(value.windowLabel);
  const afterWakeLabel = str(value.afterWakeLabel);
  const earliestStartAt = str(value.earliestStartAt);
  const latestFinishAt = str(value.latestFinishAt);
  if (
    [earliestStartAt, latestFinishAt].some((time) => time && !Number.isFinite(Date.parse(time)))
  ) {
    return undefined;
  }
  const preferredAfterWakeMinutes =
    typeof value.preferredAfterWakeMinutes === "number" &&
    Number.isInteger(value.preferredAfterWakeMinutes) &&
    value.preferredAfterWakeMinutes >= 0 &&
    value.preferredAfterWakeMinutes <= 1440
      ? value.preferredAfterWakeMinutes
      : undefined;
  const minimumConfidence =
    typeof value.minimumConfidence === "string" ? value.minimumConfidence : undefined;
  return {
    taskId,
    revision,
    title,
    durationMinutes,
    durationLabel,
    status,
    ...(windowLabel ? { windowLabel } : {}),
    ...(afterWakeLabel ? { afterWakeLabel } : {}),
    createdLabel,
    ...(earliestStartAt ? { earliestStartAt } : {}),
    ...(latestFinishAt ? { latestFinishAt } : {}),
    preferredAfterWakeMinutes,
    minimumConfidence,
    // Older builds expose only prose. Never parse it or erase hidden constraints.
    editable: preferredAfterWakeMinutes !== undefined && minimumConfidence !== undefined,
  };
}

export function normalizeTasks(value: unknown): TasksData | undefined {
  if (!isRecord(value) || !Array.isArray(value.tasks)) return undefined;
  const status = value.status === "ok" || value.status === "unavailable" ? value.status : undefined;
  if (!status) return undefined;
  const tasks: Task[] = [];
  for (const item of value.tasks) {
    const task = normalizeTask(item);
    if (!task) return undefined;
    tasks.push(task);
  }
  const message = str(value.message);
  return { status, ...(message ? { message } : {}), tasks };
}

export async function loadTasks(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<TasksData> {
  const method = findWailsMethod(root, ["ListTasks"]);
  if (!method && !hasDesktopBridge(root)) return unavailable;
  try {
    const normalized = normalizeTasks(await method?.());
    if (normalized) return normalized;
  } catch {
    // fall through to unavailable
  }
  return {
    status: "unavailable",
    tasks: [],
    message:
      "Your tasks could not be loaded. Refresh to try again; saved tasks have not been changed.",
  };
}

async function mutateTasks(
  root: WailsRoot,
  names: readonly string[],
  input: unknown,
  failure: string,
): Promise<TasksData> {
  const method = findWailsMethod(root, names);
  if (!method) throw new Error("Task planning needs the ZeitBoard desktop app.");
  const result = await method(input);
  const normalized = normalizeTasks(result);
  if (!normalized || normalized.status !== "ok") throw new Error(failure);
  notifySleepDataChanged(); // proposals depend on open tasks; refresh projections
  return normalized;
}

export function addTask(
  input: TaskInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<TasksData> {
  return mutateTasks(root, ["AddTask"], input, "The task could not be added.");
}

export function updateTask(
  input: TaskInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<TasksData> {
  return mutateTasks(root, ["UpdateTask"], input, "The task could not be updated.");
}

export function setTaskDone(
  taskId: string,
  revision: number,
  done: boolean,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<TasksData> {
  return mutateTasks(
    root,
    ["SetTaskDone"],
    { taskId, revision, done },
    "The task status could not change.",
  );
}

export function deleteTask(
  taskId: string,
  revision: number,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<TasksData> {
  return mutateTasks(root, ["DeleteTask"], { taskId, revision }, "The task could not be deleted.");
}

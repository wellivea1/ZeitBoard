import { notifySleepDataChanged } from "./sleepDataEvents";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// User-owned flexible tasks (ADR-0018): real planning items the scheduler
// proposes windows for. Titles are private user text and stay local.

export interface Task {
  taskId: string;
  title: string;
  durationMinutes: number;
  durationLabel: string;
  status: "open" | "done";
  windowLabel?: string;
  afterWakeLabel?: string;
  createdLabel: string;
}

export interface TasksData {
  status: "ok" | "unavailable";
  message?: string;
  tasks: Task[];
}

export interface TaskInput {
  taskId?: string;
  title: string;
  durationMinutes: number;
  earliestStartLocal?: string;
  latestFinishLocal?: string;
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
  if (
    !taskId ||
    !title ||
    !durationLabel ||
    !createdLabel ||
    !status ||
    durationMinutes === undefined
  ) {
    return undefined;
  }
  const windowLabel = str(value.windowLabel);
  const afterWakeLabel = str(value.afterWakeLabel);
  return {
    taskId,
    title,
    durationMinutes,
    durationLabel,
    status,
    ...(windowLabel ? { windowLabel } : {}),
    ...(afterWakeLabel ? { afterWakeLabel } : {}),
    createdLabel,
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
  if (!method) return unavailable;
  try {
    const normalized = normalizeTasks(await method());
    if (normalized) return normalized;
  } catch {
    // fall through to unavailable
  }
  return unavailable;
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
  if (!normalized) throw new Error(failure);
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
  done: boolean,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<TasksData> {
  return mutateTasks(root, ["SetTaskDone"], { taskId, done }, "The task status could not change.");
}

export function deleteTask(
  taskId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<TasksData> {
  return mutateTasks(root, ["DeleteTask"], { taskId }, "The task could not be deleted.");
}

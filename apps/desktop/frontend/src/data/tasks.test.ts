import { describe, expect, it } from "vitest";

import { addTask, deleteTask, loadTasks, normalizeTasks, setTaskDone } from "./tasks";

const backendTasks = {
  status: "ok",
  tasks: [
    {
      needsReview: false,
      updatedAt: "2026-09-21T12:00:00Z",
      taskId: "task_abc123def456",
      revision: 3,
      preferredAfterWakeMinutes: 60,
      minimumConfidence: "medium",
      title: "File paperwork",
      durationMinutes: 45,
      durationLabel: "45 minutes",
      status: "open",
      windowLabel: "Finish by Jul 5, 3:00 PM",
      afterWakeLabel: "At least 60 min after waking",
      createdLabel: "Added Jul 2",
    },
  ],
};

describe("tasks", () => {
  it("normalizes the task list", () => {
    const data = normalizeTasks(backendTasks);
    expect(data?.status).toBe("ok");
    expect(data?.tasks[0]?.title).toBe("File paperwork");
    expect(data?.tasks[0]?.windowLabel).toContain("Finish by");
  });

  it("keeps an accepted block only when it is whole", () => {
    const withBlock = (block: Record<string, string>) =>
      normalizeTasks({ ...backendTasks, tasks: [{ ...backendTasks.tasks[0], ...block }] })
        ?.tasks[0];
    expect(
      withBlock({
        scheduledStartAt: "2026-09-24T21:30:00Z",
        scheduledEndAt: "2026-09-24T21:50:00Z",
        scheduledLabel: "Thu Sep 24, 5:30 PM EDT to Thu Sep 24, 5:50 PM EDT",
      })?.scheduled,
    ).toEqual({
      startAt: "2026-09-24T21:30:00Z",
      endAt: "2026-09-24T21:50:00Z",
      label: "Thu Sep 24, 5:30 PM EDT to Thu Sep 24, 5:50 PM EDT",
    });
    expect(withBlock({ scheduledStartAt: "2026-09-24T21:30:00Z" })?.scheduled).toBeUndefined();
    expect(
      withBlock({ scheduledStartAt: "soon", scheduledEndAt: "later", scheduledLabel: "x" })
        ?.scheduled,
    ).toBeUndefined();
  });

  it("rejects malformed tasks and off-enum statuses", () => {
    expect(
      normalizeTasks({ status: "ok", tasks: [{ taskId: "t", status: "archived" }] }),
    ).toBeUndefined();
    expect(normalizeTasks({ status: "weird", tasks: [] })).toBeUndefined();
  });

  it("is read-only unavailable without the Wails bridge", async () => {
    const result = await loadTasks({});
    expect(result.status).toBe("unavailable");
    expect(result.tasks).toEqual([]);
    await expect(addTask({ title: "x", durationMinutes: 30 }, {})).rejects.toThrow("desktop app");
  });

  it("runs CRUD through the Wails methods and returns the refreshed list", async () => {
    const calls: Array<[string, unknown]> = [];
    const respond = (name: string) => async (input?: unknown) => {
      calls.push([name, input]);
      return backendTasks;
    };
    const root = {
      go: {
        main: {
          App: {
            AddTask: respond("AddTask"),
            SetTaskDone: respond("SetTaskDone"),
            DeleteTask: respond("DeleteTask"),
          },
        },
      },
    };
    await addTask({ title: "File paperwork", durationMinutes: 45 }, root);
    await setTaskDone("task_abc123def456", 3, true, root);
    await deleteTask("task_abc123def456", 3, root);
    expect(calls.map(([name]) => name)).toEqual(["AddTask", "SetTaskDone", "DeleteTask"]);
    expect(calls[1]?.[1]).toEqual({ taskId: "task_abc123def456", revision: 3, done: true });
    expect(calls[2]?.[1]).toEqual({ taskId: "task_abc123def456", revision: 3 });
  });
});

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TasksScreen } from "./TasksScreen";

vi.mock("../state/approvals", () => ({
  useApprovals: () => ({ pending: [], pendingCount: 0, unplaced: [] }),
}));
const task = {
  taskId: "task_synthetic",
  revision: 4,
  title: "File paperwork",
  durationMinutes: 30,
  durationLabel: "30 minutes",
  status: "open",
  createdLabel: "Added Sep 8",
  earliestStartAt: "2026-11-01T06:30:12Z",
  latestFinishAt: "2026-11-01T09:00:00Z",
  preferredAfterWakeMinutes: 90,
  minimumConfidence: "medium",
};
const list = { status: "ok", tasks: [task] };
const service = { ListTasks: vi.fn(), UpdateTask: vi.fn(), AddTask: vi.fn(), DeleteTask: vi.fn() };
beforeEach(() => {
  for (const method of Object.values(service)) method.mockReset().mockResolvedValue(list);
  (globalThis as { go?: unknown }).go = { main: { App: service } };
});
afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("task workflow", () => {
  it("edits a task without dropping deadlines, wake constraints, or its revision", async () => {
    render(<TasksScreen />);
    fireEvent.click(await screen.findByRole("button", { name: "Edit File paperwork" }));
    expect(screen.getByRole("textbox", { name: "Task" })).toHaveFocus();
    expect(screen.getByLabelText("Minutes after waking")).toHaveValue(90);
    fireEvent.change(screen.getByRole("textbox", { name: "Task" }), {
      target: { value: "Submit paperwork" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() =>
      expect(service.UpdateTask).toHaveBeenCalledWith(
        expect.objectContaining({
          taskId: task.taskId,
          revision: 4,
          title: "Submit paperwork",
          durationMinutes: 30,
          earliestStartAt: task.earliestStartAt,
          latestFinishAt: task.latestFinishAt,
          preferredAfterWakeMinutes: 90,
          minimumConfidence: "medium",
        }),
      ),
    );
    expect(service.AddTask).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent("Updated Submit paperwork"),
    );
  });

  it("requires a separate delete decision and offers keeping the task", async () => {
    service.DeleteTask.mockResolvedValue({ status: "ok", tasks: [] });
    render(<TasksScreen />);
    fireEvent.click(await screen.findByRole("button", { name: "Delete File paperwork" }));
    expect(service.DeleteTask).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Keep task" }));
    expect(screen.queryByText(/permanently/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Delete File paperwork" }));
    fireEvent.click(screen.getByRole("button", { name: "Delete task" }));
    await waitFor(() =>
      expect(service.DeleteTask).toHaveBeenCalledWith({ taskId: task.taskId, revision: 4 }),
    );
    expect(await screen.findByText(/No tasks yet/)).toBeVisible();
  });

  it("keeps a failed save editable and never announces an unavailable response as success", async () => {
    service.AddTask.mockResolvedValue({ status: "unavailable", tasks: [] });
    render(<TasksScreen />);
    await screen.findByRole("button", { name: "Edit File paperwork" });
    fireEvent.change(screen.getByRole("textbox", { name: "Task" }), {
      target: { value: "Draft a note" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save task" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("could not be added");
    expect(screen.getByRole("textbox", { name: "Task" })).toHaveValue("Draft a note");
    expect(screen.getByRole("status")).not.toHaveTextContent("Added Draft a note");
  });

  it("refreshes after a service failure without presenting it as an empty list", async () => {
    service.ListTasks.mockRejectedValueOnce(new Error("synthetic unavailable"));
    render(<TasksScreen />);
    expect(await screen.findByText(/Your tasks could not be loaded/)).toBeVisible();
    expect(screen.queryByText(/No tasks yet/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Refresh tasks" }));
    expect(await screen.findByRole("button", { name: "Edit File paperwork" })).toBeVisible();
  });
});

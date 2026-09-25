import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskConflictCard, TaskConflictHistoryCard } from "./TaskConflictCard";
import { ApprovalsProvider, useApprovals } from "../state/approvals";
import { proposalsFixture } from "../data/proposals";
import { normalizeTaskConflict, type TaskConflict } from "../data/taskConflicts";
import { notifySleepDataChanged } from "../data/sleepDataEvents";
const task = {
  taskId: "task_shared",
  revision: 2,
  title: "Local synthetic task",
  durationMinutes: 30,
  durationLabel: "30 minutes",
  status: "open",
  createdLabel: "Added today",
  updatedAt: "2026-09-21T12:00:00Z",
  needsReview: false,
  minimumConfidence: "medium",
  preferredAfterWakeMinutes: 90,
  afterWakeLabel: "At least 90 min after waking",
  earliestStartAt: "2026-11-01T05:30:00Z",
  latestFinishAt: "2026-11-01T08:00:00Z",
};
const raw = {
  taskId: task.taskId,
  reviewToken: "task_review_first",
  local: task,
  downloaded: [
    {
      choiceId: "task_choice_remote",
      task: {
        ...task,
        title: "Downloaded synthetic task",
        durationMinutes: 90,
        durationLabel: "90 minutes",
        status: "done",
      },
    },
  ],
};
const conflict = normalizeTaskConflict(raw) as TaskConflict;
const plan = {
  ...proposalsFixture,
  fixtureMode: false,
  proposals: [],
  unplaced: [],
  taskConflicts: [conflict],
};
function View() {
  const queue = useApprovals();
  return (
    <>
      <output>{queue.pendingCount} pending</output>
      {queue.error && <p role="alert">{queue.error}</p>}
      {queue.taskConflicts.map((item) => (
        <TaskConflictCard conflict={item} key={item.reviewToken} />
      ))}
      {queue.taskConflictHistory.map((item) => (
        <TaskConflictHistoryCard item={item} key={item.reviewToken} />
      ))}
    </>
  );
}
function install(methods: Record<string, unknown>) {
  (globalThis as { go?: unknown }).go = {
    main: { App: { GetProposals: async () => plan, ...methods } },
  };
}
afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});
describe("task conflict review", () => {
  it("shows exact constraints, requires a choice, serializes save and records confirmed history", async () => {
    let finish!: (value: unknown) => void;
    const save = vi.fn(
      (input: unknown) =>
        new Promise((resolve) => {
          void input;
          finish = resolve;
        }),
    );
    const get = vi
      .fn()
      .mockResolvedValueOnce(plan)
      .mockRejectedValue(new Error("Synthetic read unavailable"));
    install({ GetProposals: get, ResolveTaskConflict: save });
    render(
      <ApprovalsProvider>
        <View />
      </ApprovalsProvider>,
    );
    expect(await screen.findByText("1 pending")).toBeVisible();
    expect(screen.getByRole("button", { name: "Save selected version" })).toBeDisabled();
    expect(screen.getByText("90 minutes")).toBeVisible();
    expect(screen.getByText("Done")).toBeVisible();
    // Identical in both versions, so it is stated once rather than in each
    // option, where it used to crowd out what actually differs.
    expect(screen.getAllByText(/90 min after waking/)).toHaveLength(1);
    // Named by its own label. Wrapping the details in the label gave the
    // radio no usable name in Chromium, which jsdom did not catch.
    expect(screen.getByRole("radio", { name: "Use downloaded revision 2" })).toBeVisible();
    fireEvent.click(screen.getByRole("radio", { name: /Use downloaded revision 2/ }));
    const submit = screen.getByRole("button", { name: "Save selected version" });
    fireEvent.click(submit);
    fireEvent.click(submit);
    expect(save).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenCalledWith({
      taskId: task.taskId,
      reviewToken: raw.reviewToken,
      choiceId: "task_choice_remote",
    });
    const confirmed = {
      taskId: task.taskId,
      reviewToken: raw.reviewToken,
      choice: "Downloaded version",
      decidedLabel: "Today",
      result: { ...raw.downloaded[0]!.task, revision: 3 },
      before: conflict,
    };
    await act(async () => finish(confirmed));
    expect(screen.getByText("0 pending")).toBeVisible();
    expect(screen.getByText(/Resolved task: Downloaded synthetic task/)).toBeVisible();
    expect(screen.getByRole("alert")).toHaveTextContent("Synthetic read unavailable");
    expect(screen.queryByRole("button", { name: "Save selected version" })).toBeNull();
  });
  it("resets the choice when another downloaded version changes review authority", async () => {
    const changed = {
      ...conflict,
      reviewToken: "task_review_second",
      downloaded: [
        ...conflict.downloaded,
        { choiceId: "task_choice_new", task: { ...conflict.local, revision: 3 } },
      ],
    };
    const get = vi
      .fn()
      .mockResolvedValueOnce(plan)
      .mockResolvedValue({ ...plan, taskConflicts: [changed] });
    install({ GetProposals: get });
    render(
      <ApprovalsProvider>
        <View />
      </ApprovalsProvider>,
    );
    fireEvent.click(await screen.findByRole("radio", { name: /Keep local version/ }));
    expect(screen.getByRole("button", { name: "Save selected version" })).toBeEnabled();
    act(notifySleepDataChanged);
    await screen.findByRole("radio", { name: /Use downloaded revision 3/ });
    expect(screen.getByRole("button", { name: "Save selected version" })).toBeDisabled();
  });
  it("keeps a failed review visible after list refresh", async () => {
    const save = vi.fn(async () => {
      throw new Error("The downloaded versions changed. Review again.");
    });
    install({ ResolveTaskConflict: save });
    render(
      <ApprovalsProvider>
        <View />
      </ApprovalsProvider>,
    );
    fireEvent.click(await screen.findByRole("radio", { name: /Keep local version/ }));
    fireEvent.click(screen.getByRole("button", { name: "Save selected version" }));
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        "The downloaded versions changed. Review again.",
      ),
    );
    expect(screen.getByText("1 pending")).toBeVisible();
  });
});

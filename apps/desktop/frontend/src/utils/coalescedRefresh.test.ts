import { describe, expect, it, vi } from "vitest";

import { createCoalescedRefresh } from "./coalescedRefresh";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

describe("createCoalescedRefresh", () => {
  it("runs at most one load at a time and commits only the newest request", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    const load = vi
      .fn<() => Promise<string>>()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    const commit = vi.fn();
    const refresh = createCoalescedRefresh(load, commit);

    refresh.request();
    refresh.request();
    refresh.request();
    expect(load).toHaveBeenCalledTimes(1);

    first.resolve("stale");
    await vi.waitFor(() => expect(load).toHaveBeenCalledTimes(2));
    expect(commit).not.toHaveBeenCalled();

    second.resolve("current");
    await vi.waitFor(() => expect(commit).toHaveBeenCalledWith("current"));
    expect(load).toHaveBeenCalledTimes(2);
  });

  it("lets an authoritative mutation supersede an older read", async () => {
    const pending = deferred<string>();
    const commit = vi.fn();
    const refresh = createCoalescedRefresh(() => pending.promise, commit);

    refresh.request();
    refresh.supersede();
    pending.resolve("older projection");

    await Promise.resolve();
    await Promise.resolve();
    expect(commit).not.toHaveBeenCalled();
  });

  it("cancels a queued rerun when authoritative data supersedes the queue", async () => {
    const pending = deferred<string>();
    const load = vi.fn(() => pending.promise);
    const commit = vi.fn();
    const refresh = createCoalescedRefresh(load, commit);

    refresh.request();
    refresh.request();
    refresh.supersede();
    pending.resolve("older projection");

    await Promise.resolve();
    await Promise.resolve();
    expect(load).toHaveBeenCalledTimes(1);
    expect(commit).not.toHaveBeenCalled();
  });

  it("does not publish a result after disposal", async () => {
    const pending = deferred<string>();
    const commit = vi.fn();
    const refresh = createCoalescedRefresh(() => pending.promise, commit);

    refresh.request();
    refresh.dispose();
    pending.resolve("late");

    await Promise.resolve();
    await Promise.resolve();
    expect(commit).not.toHaveBeenCalled();
  });
});

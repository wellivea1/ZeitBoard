import { act, render, screen } from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useLoaded, type LoadedOptions } from "./useLoaded";

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return { promise, resolve, reject };
}

type LoadedString = ReturnType<typeof useLoaded<string>>;
let latest: LoadedString | undefined;
const expose = (loaded: LoadedString) => {
  latest = loaded;
};

function Probe({
  load,
  options,
}: {
  load: () => Promise<string>;
  options?: LoadedOptions<string>;
}) {
  const loaded = useLoaded(load, options);
  useEffect(() => expose(loaded));
  return <p>{loaded.data === undefined ? "loading" : loaded.data}</p>;
}

describe("useLoaded", () => {
  it("is loading, not empty, until the first read settles", async () => {
    const first = deferred<string>();
    render(<Probe load={() => first.promise} />);
    expect(screen.getByText("loading")).toBeVisible();
    await act(async () => first.resolve("three nights"));
    expect(screen.getByText("three nights")).toBeVisible();
  });

  it("reads again after its events", async () => {
    let calls = 0;
    const load = vi.fn(async () => `read ${++calls}`);
    render(<Probe load={load} options={{ events: ["test:changed"] }} />);
    expect(await screen.findByText("read 1")).toBeVisible();
    await act(async () => {
      window.dispatchEvent(new Event("test:changed"));
    });
    expect(await screen.findByText("read 2")).toBeVisible();
  });

  it("keeps a value a mutation returned over a read that started earlier", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    const queue = [first, second];
    render(<Probe load={() => (queue.shift() ?? second).promise} />);
    await act(async () => first.resolve("before"));
    act(() => latest?.reload());
    act(() => latest?.set("saved"));
    await act(async () => second.resolve("stale read"));
    expect(screen.getByText("saved")).toBeVisible();
  });

  it("shows a fallback for a failed read, and keeps the last value without one", async () => {
    const failing = () => Promise.reject(new Error("store is locked"));
    const { unmount } = render(
      <Probe
        load={failing}
        options={{ fallback: (reason) => `failed: ${(reason as Error).message}` }}
      />,
    );
    expect(await screen.findByText("failed: store is locked")).toBeVisible();
    unmount();

    let fail = false;
    const load = () => (fail ? Promise.reject(new Error("gone")) : Promise.resolve("kept"));
    render(<Probe load={load} />);
    expect(await screen.findByText("kept")).toBeVisible();
    fail = true;
    await act(async () => latest?.reload());
    expect(screen.getByText("kept")).toBeVisible();
  });

  it("refreshes a time-based value when the window returns", async () => {
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    let calls = 0;
    render(<Probe load={async () => `read ${++calls}`} options={{ expires: true }} />);
    expect(await screen.findByText("read 1")).toBeVisible();
    await act(async () => {
      window.dispatchEvent(new Event("focus"));
    });
    expect(await screen.findByText("read 2")).toBeVisible();
  });
});

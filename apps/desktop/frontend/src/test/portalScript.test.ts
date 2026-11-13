import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The shared page's own script, run as a browser runs it. It lives with the
// server that serves it; it is tested here because this is where a browser
// environment is. What is held: an open page does not go on saying what it
// last knew through an outage, and a refresh does not empty a message that is
// part way written.

const script = readFileSync(
  resolve(import.meta.dirname, "../../../../server/internal/portal/assets/portal.js"),
  "utf8",
);

class FakeEventSource {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 2;
  static last: FakeEventSource | undefined;
  readyState = FakeEventSource.CONNECTING;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  private listeners = new Map<string, (event: { data: string }) => void>();
  readonly url: string;
  constructor(url: string) {
    this.url = url;
    FakeEventSource.last = this;
  }
  addEventListener(name: string, listener: (event: { data: string }) => void) {
    this.listeners.set(name, listener);
  }
  emit(name: string, data: unknown) {
    this.listeners.get(name)?.({ data: JSON.stringify(data) });
  }
  close() {
    this.readyState = FakeEventSource.CLOSED;
  }
}

const start = new Date("2026-07-08T12:00:00Z");

function page(main: string, live: string, refreshAt?: string) {
  document.body.innerHTML = `<main>${main}</main>`;
  document.body.setAttribute("data-live", live);
  if (refreshAt) document.body.setAttribute("data-refresh-at", refreshAt);
  else document.body.removeAttribute("data-refresh-at");
  document.body.removeAttribute("data-offline");
}

function answer(main: string, refreshAt?: string) {
  const attribute = refreshAt ? ` data-refresh-at="${refreshAt}"` : "";
  return {
    ok: true,
    status: 200,
    text: async () =>
      `<!DOCTYPE html><html><body data-live="stream"${attribute}><main>${main}</main></body></html>`,
  };
}

const awake = `<h1 class="lead">Likely awake right now</h1><p class="lead-detail">Based on recent sleep records.</p><p class="meta">Updated 5 minutes ago</p>`;
const lead = () => document.querySelector("main .lead")?.textContent;
const run = () => new Function(script)();

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(start);
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("EventSource", FakeEventSource);
  FakeEventSource.last = undefined;
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  document.body.innerHTML = "";
});

describe("the shared page through an outage", () => {
  it("withdraws its claim once the claim has expired without a fresh answer", async () => {
    const boundary = new Date(start.getTime() + 10 * 60_000).toISOString();
    page(awake, "stream", boundary);
    fetchMock.mockRejectedValue(new TypeError("offline"));
    run();

    // Up to the boundary the page still says what the server last said.
    await vi.advanceTimersByTimeAsync(9 * 60_000);
    expect(lead()).toBe("Likely awake right now");

    // At the boundary it asks, the answer does not come, and the claim goes.
    await vi.advanceTimersByTimeAsync(61_000 + 1000);
    expect(fetchMock).toHaveBeenCalled();
    expect(lead()).toBe("This page could not be updated");
    expect(document.querySelector("main .lead-detail")?.textContent).toMatch(
      /may no longer be true/,
    );
    expect(document.body.hasAttribute("data-offline")).toBe(true);

    // It goes on asking, and takes the server's answer when it comes.
    const calls = fetchMock.mock.calls.length;
    fetchMock.mockResolvedValue(
      answer(
        `<h1 class="lead">Likely not awake right now</h1><p class="meta">Updated just now</p>`,
      ),
    );
    await vi.advanceTimersByTimeAsync(61_000);
    expect(fetchMock.mock.calls.length).toBeGreaterThan(calls);
    expect(lead()).toBe("Likely not awake right now");
    expect(document.body.hasAttribute("data-offline")).toBe(false);
  });

  it("says the connection is lost and tries again before the claim expires", async () => {
    const boundary = new Date(start.getTime() + 60 * 60_000).toISOString();
    page(awake, "stream", boundary);
    fetchMock.mockRejectedValue(new TypeError("offline"));
    run();
    const events = FakeEventSource.last!;
    events.emit("state", { version: 1, freshness: "current" });
    events.emit("state", { version: 2, freshness: "current" });
    await vi.advanceTimersByTimeAsync(10);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(lead()).toBe("Likely awake right now");
    expect(document.getElementById("live-note")?.textContent).toBe(
      "The connection was lost. Trying again.",
    );

    // Sooner at first: five seconds, not the next boundary an hour away.
    await vi.advanceTimersByTimeAsync(5_000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("asks once a minute while the stream is trying to reconnect", async () => {
    page(awake, "stream", new Date(start.getTime() + 60 * 60_000).toISOString());
    fetchMock.mockResolvedValue(answer(awake, new Date(start.getTime() + 3_600_000).toISOString()));
    run();
    const events = FakeEventSource.last!;
    // Reconnecting, not closed: this state used to start no polling at all.
    events.readyState = FakeEventSource.CONNECTING;
    events.onerror?.();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // Once the stream is back it is followed again, and the polling stops.
    events.onopen?.();
    await vi.advanceTimersByTimeAsync(5 * 60_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("treats a busy server as an outage and not as the link's own answer", async () => {
    page(awake, "stream", new Date(start.getTime() + 60_000).toISOString());
    fetchMock.mockResolvedValue({ ok: false, status: 429, text: async () => "" });
    run();
    await vi.advanceTimersByTimeAsync(62_000);
    expect(lead()).toBe("This page could not be updated");
  });
});

describe("a message part way written", () => {
  const thread = (extra = "") =>
    `<h1 class="lead">Waiting for an answer</h1>${extra}<form method="post" action="/p/x/requests/1/messages"><input type="hidden" name="csrf" value="old"><textarea id="message" name="message"></textarea></form>`;

  it("survives the page refreshing around it, with the cursor where it was", async () => {
    page(thread(), "poll");
    fetchMock.mockResolvedValue({
      ok: true,
      status: 200,
      text: async () =>
        `<html><body data-live="poll"><main>${thread("<p>They replied.</p>").replace('value="old"', 'value="new"')}</main></body></html>`,
    });
    run();
    const before = document.querySelector<HTMLTextAreaElement>("#message")!;
    before.value = "Could we make it later";
    before.focus();
    before.setSelectionRange(5, 5);

    await vi.advanceTimersByTimeAsync(60_000);

    const after = document.querySelector<HTMLTextAreaElement>("#message")!;
    expect(after).not.toBe(before);
    expect(document.querySelector("main")?.textContent).toContain("They replied.");
    expect(after.value).toBe("Could we make it later");
    expect(document.activeElement).toBe(after);
    expect(after.selectionStart).toBe(5);
    // What the server alone decides still comes from the fresh page.
    expect(document.querySelector<HTMLInputElement>('input[name="csrf"]')?.value).toBe("new");
  });
});

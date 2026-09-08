import { afterEach, describe, expect, it, vi } from "vitest";
import { subscribeProjectionRefresh } from "./projectionRefresh";

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("projection freshness", () => {
  it("refreshes with time and on return, pauses while hidden, and releases listeners", () => {
    vi.useFakeTimers();
    const visibility = vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    const request = vi.fn();
    const unsubscribe = subscribeProjectionRefresh(request, "review:changed");
    expect(request).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(60_000);
    expect(request).toHaveBeenCalledTimes(2);
    visibility.mockReturnValue("hidden");
    vi.advanceTimersByTime(120_000);
    expect(request).toHaveBeenCalledTimes(2);
    visibility.mockReturnValue("visible");
    document.dispatchEvent(new Event("visibilitychange"));
    window.dispatchEvent(new Event("focus"));
    window.dispatchEvent(new Event("review:changed"));
    expect(request).toHaveBeenCalledTimes(5);
    unsubscribe();
    vi.advanceTimersByTime(60_000);
    window.dispatchEvent(new Event("focus"));
    window.dispatchEvent(new Event("review:changed"));
    expect(request).toHaveBeenCalledTimes(5);
  });
});

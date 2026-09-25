import { afterEach, expect, it, vi } from "vitest";
import { sleepDataChangedEvent, subscribeAnalysisUpdates } from "./sleepDataEvents";

afterEach(() => vi.unstubAllGlobals());

it("routes background analysis updates through existing evidence listeners and releases the bridge", () => {
  const dispose = vi.fn();
  const EventsOn = vi.fn<(name: string, callback: () => void) => () => void>(() => dispose);
  vi.stubGlobal("runtime", { EventsOn });
  const listener = vi.fn();
  window.addEventListener(sleepDataChangedEvent, listener);
  try {
    const unsubscribe = subscribeAnalysisUpdates();
    expect(EventsOn).toHaveBeenCalledWith("zeitboard:analysis-updated", expect.any(Function));
    EventsOn.mock.calls[0]?.[1]();
    expect(listener).toHaveBeenCalledTimes(1);
    expect(listener.mock.calls[0]?.[0]).toBeInstanceOf(Event);
    unsubscribe();
    expect(dispose).toHaveBeenCalledTimes(1);
  } finally {
    window.removeEventListener(sleepDataChangedEvent, listener);
  }
});

it("keeps browser preview usable without a native runtime", () => {
  vi.stubGlobal("runtime", undefined);
  expect(() => subscribeAnalysisUpdates()()).not.toThrow();
});

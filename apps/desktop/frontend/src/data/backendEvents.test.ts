import { afterEach, expect, it, vi } from "vitest";
import { subscribeBackendEvents } from "./backendEvents";
import { calendarDataChangedEvent } from "./calendar";
import { doseProposalsChangedEvent } from "./doseProposals";
import { medicationDataChangedEvent } from "./medications";
import { rhythmMarkersChangedEvent } from "./rhythmMarkers";
import { sleepDataChangedEvent } from "./sleepDataEvents";

afterEach(() => vi.unstubAllGlobals());

function bridge() {
  const dispose = vi.fn();
  const handlers = new Map<string, () => void>();
  const EventsOn = vi.fn((name: string, callback: () => void) => {
    handlers.set(name, callback);
    return dispose;
  });
  vi.stubGlobal("runtime", { EventsOn });
  const heard: string[] = [];
  const views = [
    sleepDataChangedEvent,
    medicationDataChangedEvent,
    rhythmMarkersChangedEvent,
    calendarDataChangedEvent,
    doseProposalsChangedEvent,
  ];
  const listeners = views.map((view) => {
    const listener = (event: Event) => heard.push(event.type);
    window.addEventListener(view, listener);
    return () => window.removeEventListener(view, listener);
  });
  const unsubscribe = subscribeBackendEvents();
  return {
    heard,
    dispose,
    announce: (name: string) => handlers.get(name)?.(),
    release: () => {
      unsubscribe();
      for (const remove of listeners) remove();
    },
  };
}

it("refreshes evidence views when a background analysis finishes", () => {
  const { heard, announce, release } = bridge();
  try {
    announce("zeitboard:analysis-updated");
    expect(heard).toEqual([sleepDataChangedEvent]);
  } finally {
    release();
  }
});

it("refreshes every synced view when a sync downloads or erases records", () => {
  const { heard, announce, release, dispose } = bridge();
  announce("zeitboard:sync-applied");
  expect(heard).toEqual([
    sleepDataChangedEvent,
    medicationDataChangedEvent,
    rhythmMarkersChangedEvent,
    calendarDataChangedEvent,
  ]);
  release();
  expect(dispose).toHaveBeenCalledTimes(3);
});

it("shows a dose an agent proposed without waiting for a refresh", () => {
  const { heard, announce, release } = bridge();
  try {
    announce("zeitboard:dose-proposed");
    expect(heard).toEqual([doseProposalsChangedEvent]);
  } finally {
    release();
  }
});

it("keeps browser preview usable without a native runtime", () => {
  vi.stubGlobal("runtime", undefined);
  expect(() => subscribeBackendEvents()()).not.toThrow();
});

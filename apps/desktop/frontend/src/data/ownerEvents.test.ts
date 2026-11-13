import { describe, expect, it, vi } from "vitest";
import {
  addOwnerEvent,
  deleteOwnerEvent,
  hasOwnerEventService,
  loadOwnerEvent,
  normalizeOwnerEvent,
} from "./ownerEvents";

const stored = {
  eventId: "event_synthetic",
  revision: 2,
  title: "Dentist (synthetic)",
  startLocal: "2026-07-08T15:00:00-04:00",
  endLocal: "2026-07-08T16:00:00-04:00",
  zoneId: "America/New_York",
  startLabel: "Jul 8, 2026, 3:00 PM EDT",
  endLabel: "Jul 8, 2026, 4:00 PM EDT",
};

const root = (methods: Record<string, unknown>) => ({ go: { main: { App: methods } } });

describe("owner event adapter", () => {
  it("reads an event the desktop returns, and refuses one it cannot trust", () => {
    expect(normalizeOwnerEvent(stored)).toEqual(stored);
    expect(normalizeOwnerEvent({ ...stored, revision: 0 })).toBeUndefined();
    expect(normalizeOwnerEvent({ ...stored, title: "" })).toBeUndefined();
    // A time without its offset names no instant.
    expect(normalizeOwnerEvent({ ...stored, startLocal: "2026-07-08T15:00" })).toBeUndefined();
    expect(normalizeOwnerEvent({ ...stored, endLocal: stored.startLocal })).toBeUndefined();
  });

  it("is offered only where the desktop can keep an event", () => {
    expect(hasOwnerEventService({})).toBe(false);
    expect(hasOwnerEventService(root({ AddOwnerEvent: vi.fn() }))).toBe(true);
  });

  it("sends a new event with no id and no revision", async () => {
    const AddOwnerEvent = vi.fn(async () => stored);
    await expect(
      addOwnerEvent(
        {
          title: stored.title,
          startLocal: stored.startLocal,
          endLocal: stored.endLocal,
          zoneId: stored.zoneId,
        },
        root({ AddOwnerEvent }),
      ),
    ).resolves.toEqual(stored);
    expect(AddOwnerEvent).toHaveBeenCalledWith({
      eventId: "",
      revision: 0,
      title: stored.title,
      startLocal: stored.startLocal,
      endLocal: stored.endLocal,
      zoneId: stored.zoneId,
    });
  });

  it("deletes the event at the revision it was read at", async () => {
    const GetOwnerEvent = vi.fn(async () => stored);
    const DeleteOwnerEvent = vi.fn(async () => undefined);
    const desktop = root({ GetOwnerEvent, DeleteOwnerEvent });
    await deleteOwnerEvent(await loadOwnerEvent(stored.eventId, desktop), desktop);
    expect(DeleteOwnerEvent).toHaveBeenCalledWith({ eventId: stored.eventId, revision: 2 });
  });

  it("says events are added in the desktop app where they cannot be", async () => {
    await expect(addOwnerEvent(stored, {})).rejects.toThrow(
      "Events are added in the ZeitBoard desktop app.",
    );
  });
});

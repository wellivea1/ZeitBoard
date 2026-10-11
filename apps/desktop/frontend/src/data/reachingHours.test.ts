import { describe, expect, it } from "vitest";
import {
  dayHoursNote,
  daySpans,
  loadReachingHours,
  newReachingContact,
  normalizeReachingHours,
  reachingHoursUnavailable,
  saveReachingHours,
  withDayHours,
  withDayOpen,
  type ReachingContact,
} from "./reachingHours";

const tuesday = { day: 2, startLocal: "09:00", endLocal: "13:00" };
const thursday = { day: 4, startLocal: "13:00", endLocal: "17:00" };

function clinic(overrides: Partial<ReachingContact> = {}): ReachingContact {
  return {
    id: "contact-clinic",
    label: "The clinic",
    zoneId: "America/New_York",
    off: false,
    hours: [tuesday, thursday],
    ...overrides,
  };
}

function payload(overrides: Record<string, unknown> = {}, contacts: unknown[] = [clinic()]) {
  return { state: { contacts }, revision: 3, conflict: false, ...overrides };
}

function rootWith(name: string, method: (input?: unknown) => Promise<unknown>) {
  return { go: { main: { App: { [name]: method } } } };
}

describe("normalizeReachingHours", () => {
  it("accepts a stored list", () => {
    const parsed = normalizeReachingHours(payload());
    expect(parsed?.state.contacts).toEqual([clinic()]);
    expect(parsed?.revision).toBe(3);
  });

  it("accepts a list with nobody on it", () => {
    expect(normalizeReachingHours(payload({}, []))?.state.contacts).toEqual([]);
  });

  it("puts each person's days in order", () => {
    const parsed = normalizeReachingHours(payload({}, [clinic({ hours: [thursday, tuesday] })]));
    expect(parsed?.state.contacts[0]?.hours).toEqual([tuesday, thursday]);
  });

  // Half a list saved back would replace the whole of it, so anything that
  // fails to parse makes the payload no list at all.
  it("refuses a list it only partly understands", () => {
    for (const broken of [
      { ...clinic(), id: "" },
      { ...clinic(), label: undefined },
      { ...clinic(), hours: undefined },
      { ...clinic(), hours: [{ day: 9, startLocal: "09:00", endLocal: "17:00" }] },
      { ...clinic(), hours: [{ day: 1, startLocal: "9am", endLocal: "17:00" }] },
      { ...clinic(), hours: [tuesday, { ...tuesday, startLocal: "10:00" }] },
    ]) {
      const other = clinic({ id: "contact-other", label: "Someone else" });
      expect(normalizeReachingHours(payload({}, [other, broken]))).toBeUndefined();
    }
    expect(normalizeReachingHours({ state: {}, revision: 3 })).toBeUndefined();
  });

  it("carries a conflict through so a lost edit is not reported as saved", () => {
    expect(normalizeReachingHours(payload({ conflict: true }))?.conflict).toBe(true);
  });
});

describe("loadReachingHours", () => {
  it("reads the stored list", async () => {
    const envelope = await loadReachingHours(rootWith("GetReachingHours", async () => payload()));
    expect(envelope.state.contacts[0]?.label).toBe("The clinic");
  });

  // The fallback must not read as a list the person chose.
  it("falls back to nobody outside the desktop app, and says why", async () => {
    const envelope = await loadReachingHours({});
    expect(envelope).toEqual(reachingHoursUnavailable);
    expect(envelope.state.contacts).toEqual([]);
    expect(envelope.message).toMatch(/desktop app/i);
  });

  it("says when the stored list could not be read", async () => {
    const envelope = await loadReachingHours(
      rootWith("GetReachingHours", async () => ({ state: 4 })),
    );
    expect(envelope.state.contacts).toEqual([]);
    // Saving from here loses to what is stored rather than replacing it.
    expect(envelope.revision).toBe(0);
    expect(envelope.message).toMatch(/could not be read/i);
  });
});

describe("saveReachingHours", () => {
  it("sends the list and the base revision", async () => {
    const sent: unknown[] = [];
    const root = rootWith("SaveReachingHours", async (input) => {
      sent.push(input);
      return payload({ revision: 4 });
    });
    const saved = await saveReachingHours({ contacts: [clinic()] }, 3, root);
    expect(saved.revision).toBe(4);
    expect(sent[0]).toEqual({ baseRevision: 3, state: { contacts: [clinic()] } });
  });

  it("fails rather than pretending a save landed", async () => {
    await expect(saveReachingHours({ contacts: [] }, 0, {})).rejects.toThrow(/desktop app/);
  });

  it("fails when the answer cannot be trusted", async () => {
    const root = rootWith("SaveReachingHours", async () => ({ revision: 4 }));
    await expect(saveReachingHours({ contacts: [] }, 0, root)).rejects.toThrow();
  });
});

describe("editing a person", () => {
  it("starts someone new on the working week in the zone given, with an id of their own", () => {
    const first = newReachingContact("Europe/Berlin");
    const second = newReachingContact("Europe/Berlin");
    expect(first.id).toMatch(/^contact-[0-9a-f]{10}$/);
    expect(first.id).not.toBe(second.id);
    expect(first).toMatchObject({ label: "", zoneId: "Europe/Berlin", off: false });
    expect(first.hours.map((hours) => hours.day)).toEqual([1, 2, 3, 4, 5]);
  });

  it("opens a day on the hours of the nearest open day before it", () => {
    expect(withDayOpen(clinic(), 3, true).hours).toEqual([
      tuesday,
      { ...tuesday, day: 3 },
      thursday,
    ]);
  });

  it("looks after the day when nothing before it in the week is open", () => {
    expect(withDayOpen(clinic(), 1, true).hours[0]).toEqual({ ...tuesday, day: 1 });
  });

  it("opens a day on the starting hours when no other day is open", () => {
    expect(withDayOpen(clinic({ hours: [] }), 0, true).hours).toEqual([
      { day: 0, startLocal: "09:00", endLocal: "17:00" },
    ]);
  });

  it("closes one day, and changes one day's hours, leaving the rest", () => {
    expect(withDayOpen(clinic(), 2, false).hours).toEqual([thursday]);
    expect(withDayHours(clinic(), 4, { endLocal: "18:00" }).hours).toEqual([
      tuesday,
      { ...thursday, endLocal: "18:00" },
    ]);
  });

  it("says when a day's hours run into the next day or last all day", () => {
    expect(dayHoursNote({ day: 5, startLocal: "22:00", endLocal: "06:00" })).toBe("next day");
    expect(dayHoursNote({ day: 5, startLocal: "00:00", endLocal: "00:00" })).toBe("all day");
    expect(dayHoursNote({ day: 5, startLocal: "09:00", endLocal: "17:00" })).toBeUndefined();
    expect(dayHoursNote({ day: 5, startLocal: "", endLocal: "17:00" })).toBeUndefined();
  });

  // A night that crosses midnight is drawn on both days it touches, the
  // second part marked as carried over.
  it("draws an overnight stretch across the two days it touches", () => {
    const friday = [{ day: 5, startLocal: "22:00", endLocal: "06:00" }];
    expect(daySpans(friday, 5)).toEqual([{ start: 22 / 24, end: 1, carried: false }]);
    expect(daySpans(friday, 6)).toEqual([{ start: 0, end: 6 / 24, carried: true }]);
    expect(daySpans(friday, 4)).toEqual([]);
    const saturday = [{ day: 6, startLocal: "20:00", endLocal: "02:00" }];
    expect(daySpans(saturday, 0)).toEqual([{ start: 0, end: 2 / 24, carried: true }]);
  });

  it("fills a day open all day, and draws nothing for a half-typed time", () => {
    expect(daySpans([{ day: 0, startLocal: "00:00", endLocal: "00:00" }], 0)).toEqual([
      { start: 0, end: 1, carried: false },
    ]);
    expect(daySpans([{ day: 0, startLocal: "", endLocal: "17:00" }], 0)).toEqual([]);
  });
});

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { ReachingContact } from "../../data/reachingHours";
import { ReachingHoursSettings } from "./ReachingHoursSettings";

interface Saved {
  state: { contacts: ReachingContact[] };
  baseRevision: number;
}

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

function install(app: Record<string, unknown>) {
  (globalThis as unknown as { go?: unknown }).go = { main: { App: app } };
}

function stub(contacts: ReachingContact[] = [clinic()]) {
  const saves: Saved[] = [];
  install({
    GetReachingHours: async () => ({ state: { contacts }, revision: 2, conflict: false }),
    SaveReachingHours: async (input: unknown) => {
      const sent = input as Saved;
      saves.push(sent);
      return { state: sent.state, revision: sent.baseRevision + 1, conflict: false };
    },
  });
  return saves;
}

const save = () => fireEvent.click(screen.getByRole("button", { name: /save reaching hours/i }));
const names = () =>
  screen.getAllByLabelText("Name").map((input) => (input as HTMLInputElement).value);

afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

describe("ReachingHoursSettings", () => {
  it("says whose hours these are", async () => {
    stub();
    render(<ReachingHoursSettings />);
    expect(await screen.findByText(/other people's hours, not yours/i)).toBeTruthy();
  });

  it("shows each person with their own hours on each day", async () => {
    stub([
      clinic(),
      clinic({
        id: "contact-sister",
        label: "My sister",
        zoneId: "Europe/Berlin",
        hours: [{ day: 6, startLocal: "18:00", endLocal: "22:00" }],
      }),
    ]);
    render(<ReachingHoursSettings />);
    const week = await screen.findByRole("group", { name: "When The clinic is available" });
    expect(names()).toEqual(["The clinic", "My sister"]);
    expect((within(week).getByLabelText("Tuesday") as HTMLInputElement).checked).toBe(true);
    expect((within(week).getByLabelText("Monday") as HTMLInputElement).checked).toBe(false);
    expect((within(week).getByLabelText("Thursday opens") as HTMLInputElement).value).toBe("13:00");
    expect(within(week).queryByLabelText("Monday opens")).toBeNull();
    expect(within(week).getAllByText("Closed")).toHaveLength(5);
    const sister = screen.getByRole("group", { name: "When My sister is available" });
    expect((within(sister).getByLabelText("Saturday closes") as HTMLInputElement).value).toBe(
      "22:00",
    );
  });

  it("saves the edited list with the revision it loaded", async () => {
    const saves = stub();
    render(<ReachingHoursSettings />);
    const week = await screen.findByRole("group", { name: "When The clinic is available" });

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "The walk-in clinic" } });
    // A day that opens takes the hours of the open day before it.
    fireEvent.click(within(week).getByLabelText("Wednesday"));
    fireEvent.change(within(week).getByLabelText("Thursday closes"), {
      target: { value: "18:30" },
    });
    save();

    await waitFor(() => expect(saves).toHaveLength(1));
    expect(saves[0]).toEqual({
      baseRevision: 2,
      state: {
        contacts: [
          clinic({
            label: "The walk-in clinic",
            hours: [tuesday, { ...tuesday, day: 3 }, { ...thursday, endLocal: "18:30" }],
          }),
        ],
      },
    });
    expect(await screen.findByRole("status")).toHaveTextContent(/Saved/);
  });

  it("says when there is nobody on the list", async () => {
    stub([]);
    render(<ReachingHoursSettings />);
    expect(await screen.findByText(/Nobody is on the list/)).toBeInTheDocument();
  });

  it("adds people up to the five Home has room for, and removes them", async () => {
    const saves = stub([]);
    render(<ReachingHoursSettings />);
    const add = await screen.findByRole("button", { name: "Add someone" });
    for (let count = 0; count < 5; count += 1) fireEvent.click(add);
    expect(screen.getAllByLabelText("Name")).toHaveLength(5);
    expect(add).toBeDisabled();
    expect(screen.getByText(/room for 5 people/)).toBeInTheDocument();

    fireEvent.click(screen.getAllByRole("button", { name: "Remove" })[0] as HTMLElement);
    expect(screen.getAllByLabelText("Name")).toHaveLength(4);
    expect(add).not.toBeDisabled();

    save();
    await waitFor(() => expect(saves).toHaveLength(1));
    const sent = saves[0]?.state.contacts ?? [];
    expect(sent).toHaveLength(4);
    expect(new Set(sent.map((contact) => contact.id)).size).toBe(4);
    expect(sent[0]).toMatchObject({ label: "", off: false });
    expect(sent[0]?.hours.map((hours) => hours.day)).toEqual([1, 2, 3, 4, 5]);
  });

  // The list's order is the order of Home's rows.
  it("moves someone up the list", async () => {
    const saves = stub([clinic(), clinic({ id: "contact-sister", label: "My sister" })]);
    render(<ReachingHoursSettings />);
    await screen.findAllByLabelText("Name");
    const moves = screen.getAllByRole("button", { name: "Move up" });
    expect(moves).toHaveLength(1);
    fireEvent.click(moves[0] as HTMLElement);
    expect(names()).toEqual(["My sister", "The clinic"]);

    save();
    await waitFor(() => expect(saves).toHaveLength(1));
    expect(saves[0]?.state.contacts.map((contact) => contact.id)).toEqual([
      "contact-sister",
      "contact-clinic",
    ]);
  });

  it("keeps someone's hours when they are taken off Home", async () => {
    const saves = stub();
    render(<ReachingHoursSettings />);
    await screen.findByLabelText("Name");
    fireEvent.click(screen.getByLabelText(/show on home/i));
    save();
    await waitFor(() => expect(saves).toHaveLength(1));
    expect(saves[0]?.state.contacts[0]).toMatchObject({ off: true, hours: [tuesday, thursday] });
  });

  // An overnight window is the case the old fixed schedule could not express,
  // so the form says what it means rather than look like a typo.
  it("marks hours that close the next day or never close", async () => {
    stub([
      clinic({
        hours: [
          { day: 0, startLocal: "00:00", endLocal: "00:00" },
          { day: 5, startLocal: "22:00", endLocal: "06:00" },
        ],
      }),
    ]);
    render(<ReachingHoursSettings />);
    expect(await screen.findByText("next day")).toBeInTheDocument();
    expect(screen.getByText("all day")).toBeInTheDocument();
  });

  // A lost edit reported as saved is worse than an error: the person walks away
  // believing a list is in force that is not.
  it("does not call a conflict a save", async () => {
    install({
      GetReachingHours: async () => ({
        state: { contacts: [clinic({ label: "Mine" })] },
        revision: 2,
        conflict: false,
      }),
      SaveReachingHours: async () => ({
        state: { contacts: [clinic({ label: "Theirs" })] },
        revision: 5,
        conflict: true,
      }),
    });
    render(<ReachingHoursSettings />);
    await screen.findByLabelText("Name");
    save();

    const status = await screen.findByRole("status");
    expect(status.textContent).toMatch(/was not applied/i);
    // The form shows what is actually stored, not the edit that lost.
    await waitFor(() => expect(names()).toEqual(["Theirs"]));
  });

  it("reports a failed save rather than looking like it worked", async () => {
    install({
      GetReachingHours: async () => ({
        state: { contacts: [clinic()] },
        revision: 1,
        conflict: false,
      }),
      SaveReachingHours: async () => {
        throw new Error("Choose at least one day for The clinic, or switch them off.");
      },
    });
    render(<ReachingHoursSettings />);
    await screen.findByLabelText("Name");
    save();
    expect(await screen.findByRole("alert")).toHaveTextContent(/at least one day/i);
  });

  it("discards an edit without saving it", async () => {
    const saves = stub();
    render(<ReachingHoursSettings />);
    await screen.findByLabelText("Name");

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Something else" } });
    fireEvent.click(screen.getByRole("button", { name: "Add someone" }));
    fireEvent.click(screen.getByRole("button", { name: /discard changes/i }));
    expect(names()).toEqual(["The clinic"]);
    expect(saves).toHaveLength(0);
  });
});

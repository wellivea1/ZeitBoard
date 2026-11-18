import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { calendarDataChangedEvent } from "../data/calendar";
import type { OwnerEvent } from "../data/ownerEvents";
import { OwnerEventForm } from "./OwnerEventForm";

const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;

function saved(input: { title: string; startLocal: string; endLocal: string; zoneId: string }) {
  return {
    eventId: "event_synthetic",
    revision: 1,
    title: input.title,
    startLocal: input.startLocal,
    endLocal: input.endLocal,
    zoneId: input.zoneId,
    startLabel: "Jul 8, 2026, 3:00 PM",
    endLabel: "Jul 8, 2026, 4:00 PM",
  };
}

function bridge(methods: Record<string, unknown>) {
  (globalThis as { go?: unknown }).go = { main: { App: methods } };
}

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("the form for an event the owner adds", () => {
  it("adds an event with exact times and says the calendar changed", async () => {
    const AddOwnerEvent = vi.fn(async (input: Parameters<typeof saved>[0]) => saved(input));
    bridge({ AddOwnerEvent });
    const changed = vi.fn();
    window.addEventListener(calendarDataChangedEvent, changed);
    const onSaved = vi.fn();
    render(
      <OwnerEventForm
        startWall="2026-07-08T15:00"
        sleepWindows={[]}
        onSaved={onSaved}
        onCancel={() => {}}
      />,
    );

    expect(screen.getByLabelText("Starts")).toHaveValue("2026-07-08T15:00");
    expect(screen.getByLabelText("Ends")).toHaveValue("2026-07-08T16:00");
    fireEvent.change(screen.getByLabelText("What it is"), {
      target: { value: "  Dentist (synthetic) " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add event" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const sent = AddOwnerEvent.mock.calls[0]![0];
    expect(sent.title).toBe("Dentist (synthetic)");
    expect(sent.zoneId).toBe(zone);
    // Each time carries the offset of the occurrence meant.
    expect(sent.startLocal).toMatch(/^2026-07-08T15:00:00[+-]\d\d:\d\d$/);
    expect(sent.endLocal).toMatch(/^2026-07-08T16:00:00[+-]\d\d:\d\d$/);
    expect(onSaved.mock.calls[0]![1]).toBe("Added Dentist (synthetic).");
    expect(changed).toHaveBeenCalled();
    window.removeEventListener(calendarDataChangedEvent, changed);
  });

  it("keeps the event's length when its start moves", () => {
    render(
      <OwnerEventForm
        startWall="2026-07-08T15:00"
        sleepWindows={[]}
        onSaved={() => {}}
        onCancel={() => {}}
      />,
    );
    fireEvent.change(screen.getByLabelText("Ends"), { target: { value: "2026-07-08T16:30" } });
    fireEvent.change(screen.getByLabelText("Starts"), { target: { value: "2026-07-09T09:00" } });
    expect(screen.getByLabelText("Ends")).toHaveValue("2026-07-09T10:30");
  });

  it("says when the event falls in a predicted sleep window, and still keeps it", () => {
    const start = new Date(2026, 6, 8, 13, 0);
    const end = new Date(2026, 6, 8, 20, 0);
    render(
      <OwnerEventForm
        startWall="2026-07-08T15:00"
        sleepWindows={[{ startAt: start.toISOString(), endAt: end.toISOString() }]}
        onSaved={() => {}}
        onCancel={() => {}}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      /This falls in a predicted sleep window, .+ The event is kept as entered\./,
    );

    // Outside the window the note goes.
    fireEvent.change(screen.getByLabelText("Starts"), { target: { value: "2026-07-08T09:00" } });
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("says what is wrong beside the form, and sends nothing", () => {
    const AddOwnerEvent = vi.fn();
    bridge({ AddOwnerEvent });
    render(
      <OwnerEventForm
        startWall="2026-07-08T15:00"
        sleepWindows={[]}
        onSaved={() => {}}
        onCancel={() => {}}
      />,
    );
    fireEvent.change(screen.getByLabelText("What it is"), { target: { value: "Dentist" } });
    fireEvent.change(screen.getByLabelText("Ends"), { target: { value: "2026-07-08T14:00" } });
    fireEvent.click(screen.getByRole("button", { name: "Add event" }));
    expect(screen.getByRole("alert")).toHaveTextContent("An event must end after it starts.");

    fireEvent.change(screen.getByLabelText("Ends"), { target: { value: "2026-07-10T15:00" } });
    fireEvent.click(screen.getByRole("button", { name: "Add event" }));
    expect(screen.getByRole("alert")).toHaveTextContent("An event can last at most a day.");
    expect(AddOwnerEvent).not.toHaveBeenCalled();
  });

  it("changes an event at the revision it was read at, keeping untouched times exact", async () => {
    const ChangeOwnerEvent = vi.fn(async (input: Parameters<typeof saved>[0]) => saved(input));
    bridge({ ChangeOwnerEvent });
    const event: OwnerEvent = {
      eventId: "event_synthetic",
      revision: 3,
      title: "Dentist",
      startLocal: "2026-07-08T15:00:00-04:00",
      endLocal: "2026-07-08T16:00:00-04:00",
      zoneId: "America/New_York",
      startLabel: "Jul 8, 2026, 3:00 PM EDT",
      endLabel: "Jul 8, 2026, 4:00 PM EDT",
    };
    const onSaved = vi.fn();
    render(
      <OwnerEventForm event={event} sleepWindows={[]} onSaved={onSaved} onCancel={() => {}} />,
    );

    expect(screen.getByRole("heading", { name: "Change the event" })).toBeVisible();
    expect(screen.getByLabelText("Starts")).toHaveValue("2026-07-08T15:00");
    fireEvent.change(screen.getByLabelText("What it is"), {
      target: { value: "Dentist, moved" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(ChangeOwnerEvent).toHaveBeenCalledWith({
      eventId: "event_synthetic",
      revision: 3,
      title: "Dentist, moved",
      startLocal: "2026-07-08T15:00:00-04:00",
      endLocal: "2026-07-08T16:00:00-04:00",
      zoneId: "America/New_York",
    });
  });

  it("shows the desktop's refusal and stays open", async () => {
    bridge({
      AddOwnerEvent: vi.fn(async () => {
        throw "that event changed meanwhile";
      }),
    });
    render(
      <OwnerEventForm
        startWall="2026-07-08T15:00"
        sleepWindows={[]}
        onSaved={() => {}}
        onCancel={() => {}}
      />,
    );
    fireEvent.change(screen.getByLabelText("What it is"), { target: { value: "Dentist" } });
    fireEvent.click(screen.getByRole("button", { name: "Add event" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("That event changed meanwhile.");
    expect(screen.getByRole("button", { name: "Add event" })).toBeEnabled();
  });
});

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SleepLogPanel } from "./SleepLogPanel";

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("SleepLogPanel", () => {
  // Moved out of Data Sources in slice U-H. The behaviour is unchanged and the
  // test came with it: a long history must page rather than either flooding the
  // screen or quietly dropping evidence.
  it("bounds sleep entries and correction history without discarding evidence", async () => {
    const corrections = Array.from({ length: 51 }, (_, index) => ({
      correctionId: `correction_${index + 1}`,
      createdLabel: `Created ${String(index + 1).padStart(3, "0")}`,
      reason: "owner correction",
      summary: `Correction ${index + 1}`,
    }));
    // One night per day from Thu, Jan 1, so each row names a different date.
    const day = (index: number, hour: string) => {
      const date = new Date(2026, 0, 1 + index);
      const pad = (value: number) => String(value).padStart(2, "0");
      return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${hour}`;
    };
    const entries = Array.from({ length: 51 }, (_, index) => {
      const number = index + 1;
      return {
        observationId: `observation_${number}`,
        startLocal: "2026-03-01T22:00",
        endLocal: "2026-03-02T06:00",
        startLabel: `Start ${number}`,
        endLabel: `End ${number}`,
        zoneId: "America/New_York",
        classification: "principal",
        effectiveStartLocal: day(index, "22:00"),
        effectiveEndLocal: day(index + 1, "06:00"),
        effectiveStartLabel: `Effective start ${number}`,
        effectiveEndLabel: `Effective end ${number}`,
        effectiveClassification: "principal",
        reviewToken: "synthetic-review-token",
        needsReview: false,
        sourceWindowLabel: "Synthetic source window",
        activeEdits: [],
        durationLabel: "8 hours 0 minutes",
        suppressed: false,
        sourceLabel: "Manual sleep log",
        provenanceLabel: "manual / user reported",
        history: index === 0 ? corrections : [],
      };
    });
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          // Pages of fifty, as the desktop reads them.
          GetSleepLogPage: async ({ page }: { page: number }) => ({
            status: "ready",
            empty: false,
            message: "51 local sleep entries stored on this device.",
            entries: entries.slice(page * 50, page * 50 + 50),
            total: entries.length,
            page,
            pageSize: 50,
          }),
        },
      },
    };

    render(<SleepLogPanel />);

    expect(await screen.findByText("Entries 1-50 of 51")).toBeVisible();
    expect(screen.getByText("Thu, Jan 1")).toBeVisible();
    expect(screen.queryByText("Fri, Feb 20")).not.toBeInTheDocument();

    expect(screen.queryByText("Created 001")).not.toBeInTheDocument();
    const history = screen.getByText("Correction history (51)").closest("details");
    expect(history).not.toBeNull();
    history!.open = true;
    fireEvent(history!, new Event("toggle"));
    expect(screen.getByText("Created 001")).toBeVisible();
    expect(screen.queryByText("Created 051")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Next corrections" }));
    expect(screen.getByText("Created 051")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Next entries" }));
    expect(await screen.findByText("Entries 51-51 of 51")).toBeVisible();
    expect(screen.getByText("Fri, Feb 20")).toBeVisible();
    expect(screen.queryByText("Thu, Jan 1")).not.toBeInTheDocument();
  });

  // The review of 2026-10-28 moved a wake time by retyping
  // "2026-10-25T14:00:00-04:00", and a time it could not read was reported
  // as "Wake time must be after sleep start." under the other form.
  it("corrects a night with clock-time pickers and says what is wrong beside the form", async () => {
    const stored = {
      observationId: "observation_1",
      startLocal: "2026-10-25T06:00:00-04:00",
      endLocal: "2026-10-25T14:00:00-04:00",
      startLabel: "Start",
      endLabel: "End",
      zoneId: "America/New_York",
      classification: "principal",
      effectiveStartLocal: "2026-10-25T06:00:00-04:00",
      effectiveEndLocal: "2026-10-25T14:00:00-04:00",
      effectiveStartLabel: "Effective start",
      effectiveEndLabel: "Effective end",
      effectiveClassification: "principal",
      reviewToken: "synthetic-review-token",
      needsReview: false,
      sourceWindowLabel: "Synthetic source window",
      activeEdits: [],
      canUndo: false,
      durationLabel: "8 hours",
      suppressed: false,
      sourceLabel: "Imported",
      provenanceLabel: "file import / directly observed",
      history: [],
    };
    const correct = vi.fn(async (input: unknown) => {
      void input;
      return stored;
    });
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetSleepLogPage: async () => ({
            status: "ready",
            empty: false,
            message: "1 local sleep entry stored on this device.",
            entries: [stored],
            total: 1,
            page: 0,
            pageSize: 50,
          }),
          CorrectSleepEntry: correct,
        },
      },
    };

    render(<SleepLogPanel />);
    fireEvent.click(await screen.findByRole("button", { name: /^Edit / }));
    const form = screen.getByRole("button", { name: "Save correction" }).closest("form")!;
    const wake = within(form).getByLabelText("Wake time");
    expect(wake).toHaveAttribute("type", "datetime-local");
    expect(wake).toHaveValue("2026-10-25T14:00");

    fireEvent.change(wake, { target: { value: "2026-10-25T05:00" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save correction" }));
    expect(within(form).getByRole("alert")).toHaveTextContent(
      "Wake time must be after sleep start.",
    );
    expect(correct).not.toHaveBeenCalled();

    fireEvent.change(wake, { target: { value: "2026-10-25T13:00" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save correction" }));
    await waitFor(() => expect(correct).toHaveBeenCalledTimes(1));
    expect(correct.mock.calls[0]?.[0]).toMatchObject({
      observationId: "observation_1",
      startLocal: "2026-10-25T06:00:00-04:00",
      endLocal: "2026-10-25T13:00:00-04:00",
      zoneId: "America/New_York",
    });
  });

  it("takes back a night's last edit in one click", async () => {
    const night = (number: number, canUndo: boolean) => ({
      observationId: `observation_${number}`,
      startLocal: "2026-03-01T22:00",
      endLocal: "2026-03-02T06:00",
      startLabel: `Start ${number}`,
      endLabel: `End ${number}`,
      zoneId: "America/New_York",
      classification: "principal",
      effectiveStartLocal: `2026-03-0${number}T22:30`,
      effectiveEndLocal: `2026-03-0${number + 1}T06:00`,
      effectiveStartLabel: `Effective start ${number}`,
      effectiveEndLabel: `Effective end ${number}`,
      effectiveClassification: "principal",
      reviewToken: `synthetic-review-token-${number}`,
      needsReview: false,
      sourceWindowLabel: "Synthetic source window",
      activeEdits: [],
      canUndo,
      durationLabel: "7 hours 30 minutes",
      suppressed: false,
      sourceLabel: "Manual sleep log",
      provenanceLabel: "manual / user reported",
      history: [],
    });
    let entries = [night(1, true), night(3, false)];
    const undone: unknown[] = [];
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetSleepLogPage: async () => ({
            status: "ready",
            empty: false,
            message: "2 local sleep entries stored on this device.",
            entries,
            total: entries.length,
            page: 0,
            pageSize: 50,
          }),
          UndoSleepCorrection: async (input: unknown) => {
            undone.push(input);
            entries = [night(1, false), night(3, false)];
            return entries[0];
          },
        },
      },
    };

    render(<SleepLogPanel />);

    // Only the night with an edit to take back offers it.
    const undo = await screen.findByRole("button", { name: /^Undo the last edit to Sun, Mar 1/ });
    expect(screen.getAllByRole("button", { name: /^Undo the last edit/ })).toHaveLength(1);

    fireEvent.click(undo);
    expect(await screen.findByText("Edit undone. It stays in the night's history.")).toBeVisible();
    expect(undone).toEqual([
      { observationId: "observation_1", reviewToken: "synthetic-review-token-1" },
    ]);
    // The log re-reads the night: with nothing left to undo, the action goes.
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /^Undo the last edit/ })).toBeNull(),
    );
  });

  it("carries the entry form and the log together", () => {
    render(<SleepLogPanel />);
    // The form for a missed night folds away under the quick buttons.
    expect(screen.getByRole("form", { name: "Add sleep entry" })).toBeInTheDocument();
    expect(screen.getByText("Add a past night")).toBeVisible();
    expect(screen.getByRole("heading", { name: "Sleep log" })).toBeVisible();
    // Source configuration stayed on Data Sources.
    expect(screen.queryByRole("heading", { name: "Connected" })).toBeNull();
  });

  // ADR-0072 and ADR-0073: a night Health Connect deleted stays in the log and
  // says why it no longer counts, or that it still does; a night recorded on
  // another clock says how far from home.
  it("says when a night was removed at its source or recorded away", async () => {
    const night = (number: number, extra: Record<string, unknown>) => ({
      observationId: `observation_${number}`,
      startLocal: `2026-09-0${number}T22:00:00-04:00`,
      endLocal: `2026-09-0${number + 1}T06:00:00-04:00`,
      startLabel: "Start",
      endLabel: "End",
      zoneId: "America/New_York",
      classification: "principal",
      effectiveStartLocal: `2026-09-0${number}T22:00:00-04:00`,
      effectiveEndLocal: `2026-09-0${number + 1}T06:00:00-04:00`,
      effectiveStartLabel: "Sep 1, 10:00 PM",
      effectiveEndLabel: "Sep 2, 6:00 AM",
      effectiveClassification: "principal",
      reviewToken: "synthetic-review-token",
      needsReview: false,
      sourceWindowLabel: "Synthetic source window",
      activeEdits: [],
      durationLabel: "8 hours 0 minutes",
      suppressed: false,
      sourceLabel: "Health Connect",
      provenanceLabel: "health connect / directly observed",
      history: [],
      ...extra,
    });
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetSleepLogPage: async () => ({
            status: "ready",
            empty: false,
            message: "3 local sleep entries stored on this device.",
            entries: [
              night(1, { removedAtSource: true, suppressed: true }),
              night(3, { removedAtSource: true }),
              night(5, { recordedAway: "Recorded 6 hours ahead of home" }),
            ],
            total: 3,
            page: 0,
            pageSize: 50,
          }),
        },
      },
    };

    render(<SleepLogPanel />);

    const nights = (await screen.findAllByRole("listitem")).filter((item) =>
      item.classList.contains("sleep-night"),
    );
    expect(nights).toHaveLength(3);
    const [gone, kept, away] = nights as [HTMLElement, HTMLElement, HTMLElement];
    expect(within(gone).getByText("Removed at the source")).toBeVisible();
    expect(
      within(gone).getByText(
        "Health Connect no longer has this night, so it is left out of estimates. Edit it to count it again.",
      ),
    ).toBeVisible();
    // The owner's correction stands on it, so it counts and is not called excluded.
    expect(within(kept).queryByText("Removed at the source")).toBeNull();
    expect(
      within(kept).getByText(
        "Health Connect no longer has this night. It still counts, because your correction stands.",
      ),
    ).toBeVisible();
    expect(within(away).getByText(/Recorded 6 hours ahead of home/)).toBeVisible();
    expect(within(away).queryByText(/Health Connect no longer has/)).toBeNull();
  });

  it("opens the past-night form when it is the address", () => {
    window.location.hash = "#/log/sleep";
    const { container } = render(<SleepLogPanel />);
    const fold = container.querySelector<HTMLDetailsElement>("details.sleep-add")!;
    expect(fold.open).toBe(false);

    // Home's "Add a past night", followed while Log is already open.
    window.location.hash = "#/log/sleep/add";
    fireEvent(window, new HashChangeEvent("hashchange"));
    expect(fold.open).toBe(true);
    expect(fold.contains(document.activeElement)).toBe(true);
    window.location.hash = "";
  });
});

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { BackendSyncStatus } from "../data/backendSync";
import { DataSourceStatusPanel } from "./DataSourceStatusPanel";

const lastSync = new Date();
lastSync.setHours(14, 51, 0, 0);
const sync = {
  enabled: true,
  status: "connected",
  pushedCount: 3,
  pulledCount: 2,
  lastSyncAt: lastSync.toISOString(),
} as BackendSyncStatus;

describe("DataSourceStatusPanel", () => {
  // Found in the running app: the row read "last Last synced Sep 27, 2:51 PM".
  it("says when sync last ran, once, as the app says times", () => {
    const { unmount } = render(<DataSourceStatusPanel syncStatus={sync} />);
    expect(
      screen.getByText("Your own server · 3 sent, 2 received · last synced today at 2:51 PM"),
    ).toBeVisible();
    unmount();
    render(<DataSourceStatusPanel syncStatus={{ ...sync, lastSyncAt: undefined }} />);
    expect(screen.getByText("Your own server · 3 sent, 2 received · not synced yet")).toBeVisible();
  });

  it("counts sleep records only once it has read them", () => {
    const { rerender } = render(<DataSourceStatusPanel />);
    expect(screen.queryByRole("heading", { name: "Sleep records" })).toBeNull();

    rerender(
      <DataSourceStatusPanel
        sleepSources={{
          status: "empty",
          message: "No sleep entries yet.",
          total: 0,
          correctedCount: 0,
          suppressedCount: 0,
          sources: [],
          overlapCount: 0,
          overlaps: [],
        }}
      />,
    );
    expect(screen.getByText("None yet. Log a night or import a file.")).toBeVisible();
  });

  it("points to the nights with more than one record", () => {
    const sources = {
      status: "ready" as const,
      message: "3 local sleep entries stored on this device.",
      total: 3,
      correctedCount: 0,
      suppressedCount: 1,
      sources: [
        {
          source: "Manual sleep log",
          provenance: "manual / user reported",
          total: 3,
          corrected: 0,
          suppressed: 1,
        },
      ],
      overlapCount: 2,
      overlaps: [],
    };
    const { rerender } = render(<DataSourceStatusPanel sleepSources={sources} />);
    // An excluded night is said the way the rest of the app says it.
    expect(screen.getByText("3 records · manual / user reported · 1 excluded")).toBeVisible();
    expect(screen.getByText(/2 nights have more than one record\./)).toBeVisible();
    expect(
      screen.getByRole("link", { name: "See how the estimator combines them" }),
    ).toHaveAttribute("href", "#/rhythm/sources");

    rerender(<DataSourceStatusPanel sleepSources={{ ...sources, overlapCount: 0 }} />);
    expect(screen.queryByText(/more than one record/)).toBeNull();
  });
});

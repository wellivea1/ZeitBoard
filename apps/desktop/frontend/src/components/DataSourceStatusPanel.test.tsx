import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { BackendSyncStatus } from "../data/backendSync";
import { DataSourceStatusPanel } from "./DataSourceStatusPanel";

const sync = {
  enabled: true,
  status: "connected",
  pushedCount: 3,
  pulledCount: 2,
  lastSyncLabel: "Last synced Sep 27, 2:51 PM",
} as BackendSyncStatus;

describe("DataSourceStatusPanel", () => {
  // Found in the running app: the row read "last Last synced Sep 27, 2:51 PM".
  it("says when sync last ran once", () => {
    render(<DataSourceStatusPanel syncStatus={sync} />);
    expect(
      screen.getByText("Your own server · 3 sent, 2 received · last synced Sep 27, 2:51 PM"),
    ).toBeVisible();
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
        }}
      />,
    );
    expect(screen.getByText("None yet. Log a night or import a file.")).toBeVisible();
  });
});

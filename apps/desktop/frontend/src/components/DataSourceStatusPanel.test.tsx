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

  it("says what the phone found when it last read Health Connect", () => {
    const since = new Date();
    since.setHours(9, 5, 0, 0);
    render(
      <DataSourceStatusPanel
        sourceStatuses={[
          {
            source: "health_connect",
            outcome: "permission_missing",
            reportedAt: since.toISOString(),
            since: since.toISOString(),
            backgroundReads: false,
            held: 0,
          },
        ]}
      />,
    );
    const row = screen
      .getByRole("heading", { name: "Health Connect on your phone" })
      .closest("article");
    expect(row).toHaveTextContent("Permission off");
    expect(row).toHaveTextContent(
      "The phone has not been allowed to read sleep since today at 9:05 AM. Allow it in ZeitBoard on the phone.",
    );
  });

  // Desktop activity is said in the words Settings uses, rather than the fixed
  // "Not connected" the row once read, and is left out until it is known.
  it("says whether this computer records its own activity", () => {
    const activity = {
      enabled: false,
      running: false,
      supported: true,
      zoneId: "UTC",
      recordCount: 0,
      lastError: "",
      powerLog: { supported: true, enabled: false, entries: 0, sinceLabel: "", lastError: "" },
    };
    const row = () => screen.getByRole("heading", { name: "Desktop activity" }).closest("article");
    const { rerender } = render(<DataSourceStatusPanel />);
    expect(screen.queryByRole("heading", { name: "Desktop activity" })).toBeNull();

    rerender(<DataSourceStatusPanel activity={activity} />);
    expect(row()).toHaveTextContent("Off unless you turn it on in Settings.");
    expect(screen.getByRole("link", { name: "Settings" })).toHaveAttribute(
      "href",
      "#/settings/computer",
    );

    rerender(
      <DataSourceStatusPanel
        activity={{
          ...activity,
          enabled: true,
          running: true,
          recordCount: 1,
          powerLog: { ...activity.powerLog, enabled: true },
        }}
      />,
    );
    expect(row()).toHaveTextContent(
      "1 record of when this computer was active, idle or locked, and sleep and wake read from Windows",
    );
    expect(row()).toHaveTextContent("Running");

    rerender(
      <DataSourceStatusPanel
        activity={{
          ...activity,
          enabled: true,
          recordCount: 6,
          lastError: "Records could not be saved.",
        }}
      />,
    );
    expect(row()).toHaveTextContent(
      "Records could not be saved. 6 records kept. Retry in Settings.",
    );
    expect(row()).toHaveTextContent("Stopped");

    rerender(<DataSourceStatusPanel activity={{ ...activity, supported: false }} />);
    expect(row()).toHaveTextContent("This system cannot record it.");
    expect(row()).toHaveTextContent("Unavailable");
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

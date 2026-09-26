import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { readRouteFromHash } from "./AppShell";
import { ScreenTabs } from "./ScreenTabs";

describe("readRouteFromHash", () => {
  it("defaults to home", () => {
    expect(readRouteFromHash("")).toMatchObject({ screen: "home" });
    expect(readRouteFromHash("#/")).toMatchObject({ screen: "home" });
    expect(readRouteFromHash("#/nonsense")).toMatchObject({ screen: "home" });
  });

  it("reads a screen and its tab from the address", () => {
    expect(readRouteFromHash("#/plan/tasks")).toMatchObject({ screen: "plan", planTab: "tasks" });
    expect(readRouteFromHash("#/log/context")).toMatchObject({ screen: "log", logTab: "context" });
    expect(readRouteFromHash("#/rhythm/drift")).toMatchObject({
      screen: "rhythm",
      rhythmTab: "drift",
    });
  });

  it("falls back to the first tab when the second segment is not one", () => {
    expect(readRouteFromHash("#/plan/wat")).toMatchObject({ screen: "plan", planTab: "tasks" });
    // Approvals is part of Tasks now; an old link lands where its decisions are.
    expect(readRouteFromHash("#/plan/approvals")).toMatchObject({
      screen: "plan",
      planTab: "tasks",
    });
    expect(readRouteFromHash("#/log")).toMatchObject({ screen: "log", logTab: "sleep" });
  });

  // The consolidation removed destinations that are still written down — in
  // the verification record and in whatever the user bookmarked.
  it.each([
    ["#/overview", { screen: "home" }],
    ["#/timeline", { screen: "rhythm" }],
    ["#/calendar", { screen: "plan", planTab: "week" }],
    ["#/plan/calendar", { screen: "plan", planTab: "week" }],
    ["#/tasks", { screen: "plan", planTab: "tasks" }],
    ["#/approvals", { screen: "plan", planTab: "tasks" }],
    ["#/medications", { screen: "log", logTab: "medications" }],
  ])("keeps the legacy route %s working", (hash, expected) => {
    expect(readRouteFromHash(hash)).toMatchObject(expected);
  });

  it("keeps the utility destinations addressable", () => {
    expect(readRouteFromHash("#/data-sources")).toMatchObject({ screen: "data-sources" });
    expect(readRouteFromHash("#/settings")).toMatchObject({
      screen: "settings",
      settingsTab: "display",
    });
    // A settings section can be linked to, as Data Sources does for sync.
    expect(readRouteFromHash("#/settings/sync")).toMatchObject({
      screen: "settings",
      settingsTab: "sync",
    });
  });
});

const tabs = [
  { id: "one" as const, label: "One" },
  { id: "two" as const, label: "Two", badge: 3 },
  { id: "three" as const, label: "Three" },
];

describe("ScreenTabs", () => {
  it("marks one tab selected and takes one tab stop", () => {
    render(
      <ScreenTabs name="demo" label="Demo views" tabs={tabs} active="two" onSelect={() => {}} />,
    );
    const stops = screen.getAllByRole("tab").filter((tab) => tab.getAttribute("tabindex") !== "-1");
    expect(stops).toHaveLength(1);
    expect(stops[0]).toHaveAccessibleName(/Two/);
    expect(stops[0]).toHaveAttribute("aria-selected", "true");
  });

  // Arrow keys are the part people forget, and the part somebody navigating by
  // keyboard because they are too tired to aim a mouse depends on.
  it("moves with the arrow keys and wraps", () => {
    const onSelect = vi.fn();
    render(
      <ScreenTabs name="demo" label="Demo views" tabs={tabs} active="three" onSelect={onSelect} />,
    );
    const list = screen.getByRole("tablist");

    fireEvent.keyDown(list, { key: "ArrowRight" });
    expect(onSelect).toHaveBeenLastCalledWith("one");
    expect(screen.getByRole("tab", { name: "One" })).toHaveFocus();

    fireEvent.keyDown(list, { key: "ArrowLeft" });
    expect(onSelect).toHaveBeenLastCalledWith("two");
    expect(screen.getByRole("tab", { name: /Two/ })).toHaveFocus();

    fireEvent.keyDown(list, { key: "Home" });
    expect(onSelect).toHaveBeenLastCalledWith("one");

    fireEvent.keyDown(list, { key: "End" });
    expect(onSelect).toHaveBeenLastCalledWith("three");
  });

  it("ignores keys that are not navigation", () => {
    const onSelect = vi.fn();
    render(
      <ScreenTabs name="demo" label="Demo views" tabs={tabs} active="one" onSelect={onSelect} />,
    );
    fireEvent.keyDown(screen.getByRole("tablist"), { key: "a" });
    expect(onSelect).not.toHaveBeenCalled();
  });

  // A badge that is always there is a badge nobody reads.
  it("shows a count only where there is one", () => {
    render(
      <ScreenTabs
        name="demo"
        label="Demo views"
        tabs={[{ id: "one" as const, label: "One", badge: 0 }, tabs[1]!]}
        active="one"
        onSelect={() => {}}
      />,
    );
    expect(screen.getByLabelText("3 pending")).toBeVisible();
    expect(screen.queryByLabelText("0 pending")).toBeNull();
  });
});

import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { AppearanceProvider } from "./theme/AppearanceProvider";

beforeEach(() => {
  window.location.hash = "#/overview";
  localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-reduced");
  delete (globalThis as { go?: unknown }).go;
  const metaTheme =
    document.querySelector('meta[name="theme-color"]') ??
    document.head.appendChild(document.createElement("meta"));
  metaTheme.setAttribute("name", "theme-color");
  metaTheme.setAttribute("content", "#f3f0e9");
});

describe("desktop navigation", () => {
  it("exposes every requested screen", () => {
    render(<App />);
    const navigation = screen.getByRole("navigation", { name: "Primary navigation" });

    for (const label of [
      "Overview",
      "Calendar",
      "Tasks",
      "Approvals",
      "Rhythm",
      "Medications",
      "Sharing",
      "Data Sources",
    ]) {
      expect(navigation).toHaveTextContent(label);
    }
    expect(screen.getByRole("link", { name: "Settings" })).toBeVisible();
    expect(screen.getByText("Sample data")).toBeVisible();
  });

  it("renders approval proposals with explicit actions", () => {
    window.location.hash = "#/approvals";
    render(<App />);

    expect(screen.getByRole("heading", { name: "Approvals" })).toBeVisible();
    expect(screen.getByText("Email Dr. Okafor")).toBeVisible();
    expect(screen.getAllByRole("button", { name: "Accept proposal" })).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "Reject proposal" })).toHaveLength(2);
  });

  it("approves a proposal, updates the queue and badge, and supports undo", () => {
    window.location.hash = "#/approvals";
    render(<App />);

    expect(screen.getByLabelText("2 pending")).toBeVisible();
    fireEvent.click(screen.getAllByRole("button", { name: "Accept proposal" })[0] as HTMLElement);

    expect(screen.queryByText("Email Dr. Okafor")).toBeNull();
    expect(screen.getAllByRole("button", { name: "Accept proposal" })).toHaveLength(1);
    expect(screen.getByLabelText("1 pending")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(screen.getByText("Email Dr. Okafor")).toBeVisible();
    expect(screen.getAllByRole("button", { name: "Accept proposal" })).toHaveLength(2);
  });

  it("shows the empty state once every proposal is decided", () => {
    window.location.hash = "#/approvals";
    render(<App />);

    fireEvent.click(screen.getAllByRole("button", { name: "Accept proposal" })[0] as HTMLElement);
    fireEvent.click(screen.getAllByRole("button", { name: "Reject proposal" })[0] as HTMLElement);

    expect(screen.getByRole("heading", { name: "Nothing waiting for approval" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Accept proposal" })).toBeNull();
  });

  it("switches rhythm tabs between actogram and source review", () => {
    window.location.hash = "#/rhythm";
    render(<App />);

    expect(screen.getByRole("heading", { name: "Rhythm" })).toBeVisible();
    // Actogram is the default tab; correction/source review lives under Sources.
    expect(screen.getByRole("heading", { name: "Double-plot actogram" })).toBeVisible();
    expect(
      screen.getByText(
        "Approximate. Forecast widens with time and is shown as ranges, not hard lines.",
      ),
    ).toBeVisible();
    expect(
      screen.getByRole("img", {
        name: "Predicted sleep window: Jun 18, Jun 18, 11:21 PM earliest to Jun 19, 5:27 AM latest, 6 hr 6 min window, Forecast cycle 3, Low confidence",
      }),
    ).toBeVisible();
    expect(screen.queryByRole("heading", { name: "Correction inspector" })).toBeNull();

    fireEvent.click(screen.getByRole("tab", { name: "Sources" }));

    expect(screen.getByRole("heading", { name: "Correction inspector" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Undo correction" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Source conflicts and missingness" })).toBeVisible();
    expect(screen.getByText("Wearable sleep overlaps desktop activity")).toBeVisible();
  });

  it("renders the rhythm drift visualizer instead of a placeholder", () => {
    window.location.hash = "#/rhythm";
    render(<App />);

    fireEvent.click(screen.getByRole("tab", { name: "Drift" }));

    expect(screen.getByRole("heading", { name: "Sleep-onset drift" })).toBeVisible();
    expect(screen.getAllByText("+48 min per cycle").length).toBeGreaterThan(0);
    expect(screen.getByText("Theil-Sen fit")).toBeVisible();
    expect(
      screen.getByText(
        "Y-axis is unwrapped so the free-running trend stays readable across midnight.",
      ),
    ).toBeVisible();
    expect(
      screen.queryByText("The sleep-onset drift chart arrives with the sleep visualizer"),
    ).toBeNull();
  });

  it("keeps the legacy timeline route usable as Rhythm", () => {
    window.location.hash = "#/timeline";
    render(<App />);

    expect(screen.getByRole("heading", { name: "Rhythm" })).toBeVisible();
  });

  it("validates manual sleep entry civil ranges", () => {
    window.location.hash = "#/data-sources";
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          ListSleepEntries: async () => ({
            status: "empty",
            empty: true,
            message: "No sleep entries yet.",
            entries: [],
          }),
        },
      },
    };
    render(<App />);

    fireEvent.change(screen.getByLabelText("Sleep start"), {
      target: { value: "2026-03-02T08:00" },
    });
    fireEvent.change(screen.getByLabelText("Wake time"), {
      target: { value: "2026-03-02T07:00" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save sleep entry" }));

    expect(screen.getByRole("alert")).toHaveTextContent("Wake time must be after sleep start.");
  });

  it("submits a manual sleep entry through Wails and reloads the local log", async () => {
    window.location.hash = "#/data-sources";
    const entry = {
      observationId: "obs_sleep_01",
      startLocal: "2026-03-01T22:00",
      endLocal: "2026-03-02T06:00",
      startLabel: "Sun Mar 1, 10:00 PM EST",
      endLabel: "Mon Mar 2, 6:00 AM EST",
      zoneId: "America/New_York",
      classification: "principal",
      effectiveStartLocal: "2026-03-01T22:00",
      effectiveEndLocal: "2026-03-02T06:00",
      effectiveStartLabel: "Sun Mar 1, 10:00 PM EST",
      effectiveEndLabel: "Mon Mar 2, 6:00 AM EST",
      effectiveClassification: "principal",
      durationLabel: "8 hours 0 minutes",
      suppressed: false,
      sourceLabel: "Manual sleep log",
      provenanceLabel: "manual / user reported",
      history: [],
    };
    let saved = false;
    const addSleep = vi.fn(async () => {
      saved = true;
      return entry;
    });
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          ListSleepEntries: async () =>
            saved
              ? {
                  status: "ready",
                  empty: false,
                  message: "1 local sleep entry stored on this device.",
                  entries: [entry],
                }
              : {
                  status: "empty",
                  empty: true,
                  message: "No sleep entries yet.",
                  entries: [],
                },
          AddSleepEntry: addSleep,
          GetProposals: async () =>
            saved
              ? {
                  fixtureMode: false,
                  status: "estimated",
                  proposals: [
                    {
                      id: "proposal-local-entry",
                      origin: "scheduler",
                      kind: "Place",
                      title: "Local sleep data follow-up",
                      to: "Mon Mar 2, 10:00 AM to 10:30 AM",
                      rhythmContext: "at the start of a predicted waking window",
                      confidence: "High",
                      explanationCodes: ["within_predicted_waking_window"],
                      reasonLabels: ["In a predicted waking window"],
                      createdLabel: "Proposed by Scheduler from local sleep entries",
                      expiresLabel: "valid for the current estimate",
                    },
                  ],
                  unplaced: [],
                }
              : {
                  fixtureMode: false,
                  status: "empty",
                  refusal: {
                    code: "estimate_unavailable",
                    message: "Add sleep entries.",
                  },
                  proposals: [],
                  unplaced: [
                    {
                      title: "Local sleep data follow-up",
                      reason: "No current estimate to plan against",
                      reasonCode: "estimate_unavailable",
                      nextAction: "Add sleep entries.",
                    },
                  ],
                },
        },
      },
    };

    render(<App />);

    expect(await screen.findByRole("heading", { name: "No sleep entries yet" })).toBeVisible();
    fireEvent.change(screen.getByLabelText("Sleep start"), {
      target: { value: "2026-03-01T22:00" },
    });
    fireEvent.change(screen.getByLabelText("Wake time"), {
      target: { value: "2026-03-02T06:00" },
    });
    fireEvent.change(screen.getByLabelText("Time zone"), {
      target: { value: "America/New_York" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save sleep entry" }));

    expect(await screen.findByText("Sleep entry saved locally.")).toBeVisible();
    expect(addSleep).toHaveBeenCalledWith({
      startLocal: "2026-03-01T22:00",
      endLocal: "2026-03-02T06:00",
      zoneId: "America/New_York",
      classification: "principal",
    });
    expect(screen.getByText(/Sun Mar 1, 10:00 PM EST to Mon Mar 2, 6:00 AM EST/)).toBeVisible();

    fireEvent.click(screen.getByRole("link", { name: /Approvals/ }));
    expect(await screen.findByText("Local sleep data follow-up")).toBeVisible();
  });

  it("applies Settings appearance controls through the visible UI", () => {
    window.location.hash = "#/settings";
    render(
      <AppearanceProvider>
        <App />
      </AppearanceProvider>,
    );

    const appearance = screen.getByRole("combobox", { name: "Appearance" });
    const reduced = screen.getByRole("checkbox", { name: /Reduced stimulation/ });

    expect(appearance).toHaveValue("auto");
    expect(reduced).not.toBeChecked();

    fireEvent.change(appearance, { target: { value: "dark" } });
    fireEvent.click(reduced);

    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    expect(document.documentElement).toHaveAttribute("data-reduced", "true");
    expect(document.querySelector('meta[name="theme-color"]')).toHaveAttribute(
      "content",
      "#161b19",
    );
    expect(localStorage.getItem("zeitboard-theme")).toBe("dark");
    expect(localStorage.getItem("zeitboard-reduced")).toBe("true");

    fireEvent.change(appearance, { target: { value: "light" } });

    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    expect(document.querySelector('meta[name="theme-color"]')).toHaveAttribute(
      "content",
      "#f3f0e9",
    );
  });
});

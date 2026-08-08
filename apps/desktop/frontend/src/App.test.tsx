import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { AppearanceProvider } from "./theme/AppearanceProvider";

beforeEach(() => {
  window.location.hash = "#/home";
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
  // Slice U-H: five primary destinations and a separate utility group. Eight
  // equal-weight entries was too much undifferentiated navigation for someone
  // operating under fatigue, and the count is the whole point of the change.
  it("offers five primary destinations and keeps utilities apart", () => {
    render(<App />);
    const navigation = screen.getByRole("navigation", { name: "Primary navigation" });

    expect(within(navigation).getAllByRole("link")).toHaveLength(5);
    for (const label of ["Home", "Plan", "Rhythm", "Log", "Sharing"]) {
      expect(navigation).toHaveTextContent(label);
    }
    // The pending count rides on Plan rather than on a destination that is
    // empty most of the time.
    expect(screen.getByRole("link", { name: "Plan, 2 pending" })).toBeVisible();

    const utilities = screen.getByRole("navigation", { name: "Settings and sources" });
    for (const label of ["Data Sources", "Settings"]) {
      expect(utilities).toHaveTextContent(label);
    }
    expect(screen.getByText("Sample data")).toBeVisible();
  });

  // Old hashes are written down in this app's own links, in the runbook, and in
  // whatever the user bookmarked. A dead link is a worse answer than a redirect.
  it.each([
    ["#/overview", "Home"],
    ["#/calendar", "Plan"],
    ["#/tasks", "Plan"],
    ["#/approvals", "Plan"],
    ["#/medications", "Log"],
    ["#/timeline", "Rhythm"],
  ])("redirects the legacy route %s", async (hash, heading) => {
    window.location.hash = hash;
    render(<App />);
    expect(await screen.findByRole("heading", { level: 1, name: heading })).toBeVisible();
  });

  it("opens the tab named in the address", async () => {
    window.location.hash = "#/log/medications";
    render(<App />);
    expect(await screen.findByRole("heading", { level: 1, name: "Log" })).toBeVisible();
    expect(screen.getByRole("tab", { name: /Medications/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("renders Overview as one rhythm-first surface instead of metric cards", () => {
    const { container } = render(<App />);

    expect(screen.getByRole("heading", { name: "Likely awake" })).toBeVisible();
    expect(screen.getByText("Sample date · Jun 16")).toBeVisible();
    expect(screen.getByText("Today in your cycle")).toBeVisible();
    expect(screen.getByText("Today, 10:15 PM to 1:27 AM")).toBeVisible();
    expect(screen.getByText("+48 min per cycle")).toBeVisible();
    expect(container.querySelectorAll(".overview-surface")).toHaveLength(1);
    expect(container.querySelector(".overview-surface .panel")).toBeNull();
    expect(container.querySelector(".metric-card")).toBeNull();
  });

  it("renders approval proposals with explicit actions", async () => {
    window.location.hash = "#/plan/approvals";
    const { container } = render(<App />);

    expect(await screen.findByRole("tab", { name: /Approvals/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByText("Email Dr. Okafor")).toBeVisible();
    expect(screen.getAllByRole("button", { name: "Accept proposal" })).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "Reject proposal" })).toHaveLength(2);
    expect(screen.getByText("Medium")).toBeVisible();
    expect(container.querySelector(".approval-filter.panel")).toBeNull();
    expect(container.querySelectorAll(".proposal-stack > .proposal-card")).toHaveLength(2);
    expect(container.querySelector(".proposal-stack > .panel")).toBeNull();
  });

  it("keeps task proposals and no-safe-window context in one approval surface", async () => {
    window.location.hash = "#/plan/tasks";
    const { container } = render(<App />);

    await screen.findByRole("heading", { level: 3, name: "Call service provider" });
    expect(container.querySelector(".approval-summary > .proposal-card")).not.toBeNull();
    expect(container.querySelector(".approval-summary > .unplaced-row")).not.toBeNull();
    expect(container.querySelector(".screen-grid > .unplaced-panel")).toBeNull();
    expect(screen.getByRole("heading", { level: 3, name: "Call service provider" })).toBeVisible();
  });

  it("approves a proposal, updates the queue and badge, and supports undo", async () => {
    window.location.hash = "#/plan/approvals";
    render(<App />);

    // The count now appears twice on purpose: on the Plan destination, so it is
    // visible from anywhere, and on the Approvals tab once you are here.
    const navigation = () => screen.getByRole("navigation", { name: "Primary navigation" });
    const accepts = await screen.findAllByRole("button", { name: "Accept proposal" });
    expect(accepts).toHaveLength(2);
    expect(within(navigation()).getByLabelText("2 pending")).toBeVisible();

    fireEvent.click(accepts[0] as HTMLElement);

    expect(screen.getByText("Email Dr. Okafor")).toBeVisible();
    expect(screen.getByText("approved")).toBeVisible();
    expect(screen.getAllByRole("button", { name: "Accept proposal" })).toHaveLength(1);
    expect(within(navigation()).getByLabelText("1 pending")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(screen.getByText("Email Dr. Okafor")).toBeVisible();
    expect(screen.getAllByRole("button", { name: "Accept proposal" })).toHaveLength(2);
  });

  it("shows the empty state once every proposal is decided", async () => {
    window.location.hash = "#/plan/approvals";
    render(<App />);

    fireEvent.click(
      (await screen.findAllByRole("button", { name: "Accept proposal" }))[0] as HTMLElement,
    );
    fireEvent.click(screen.getAllByRole("button", { name: "Reject proposal" })[0] as HTMLElement);

    expect(screen.getByRole("heading", { name: "Nothing waiting for approval" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Accept proposal" })).toBeNull();
  });

  it("switches rhythm tabs between actogram and source review", async () => {
    window.location.hash = "#/rhythm";
    render(<App />);

    expect(await screen.findByRole("heading", { name: "Rhythm" })).toBeVisible();
    // Actogram is the default tab; correction/source review lives under Sources.
    expect(screen.getByRole("heading", { name: "Double-plot actogram" })).toBeVisible();
    expect(
      screen.getByText(
        "Approximate. Forecast widens with time and is shown as ranges, not hard lines.",
      ),
    ).toBeVisible();
    expect(screen.getByLabelText("Show forecast")).not.toBeChecked();
    expect(
      screen.queryByRole("img", {
        name: "Predicted sleep window: Jun 18, Jun 18, 11:21 PM earliest to Jun 19, 5:27 AM latest, 6 hr 6 min window, Forecast cycle 3, Low confidence",
      }),
    ).toBeNull();
    fireEvent.click(screen.getByLabelText("Show forecast"));
    expect(
      screen.getByRole("img", {
        name: "Predicted sleep window: Jun 18, Jun 18, 11:21 PM earliest to Jun 19, 5:27 AM latest, 6 hr 6 min window, Forecast cycle 3, Low confidence",
      }),
    ).toBeVisible();
    expect(screen.queryByRole("heading", { name: "Correction inspector" })).toBeNull();

    fireEvent.click(screen.getByRole("tab", { name: "Sources" }));

    expect(screen.getByRole("heading", { name: "Correction inspector" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Undo correction" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Source conflicts and missingness" })).toBeVisible();
    expect(screen.getByText("Wearable sleep overlaps desktop activity")).toBeVisible();
  });

  it("appends and permanently erases context through the real desktop bridge", async () => {
    window.location.hash = "#/rhythm";
    const marker = {
      markerId: "marker_local_01",
      kind: "travel",
      kindLabel: "Travel / time-zone context",
      startAt: "2026-07-22T13:00:00Z",
      zoneId: "America/New_York",
      civilDate: "2026-07-22",
      hour: 9,
      startLabel: "Jul 22, 2026, 9:00 AM",
      rangeLabel: "Jul 22, 2026, 9:00 AM onward",
      note: "Arrival context",
      recordedLabel: "Jul 22, 2026, 12:00 PM",
    };
    const empty = {
      status: "empty",
      empty: true,
      message: "No context markers yet.",
      markers: [],
      fixtureMode: false,
      updatedLabel: "Updated Jul 22, 12:00 PM",
    };
    const ready = {
      status: "ready",
      empty: false,
      message: "1 self-reported context marker. It does not establish cause.",
      markers: [marker],
      fixtureMode: false,
      updatedLabel: "Updated Jul 22, 12:00 PM",
    };
    let current: typeof empty | typeof ready = empty;
    const add = vi.fn(async () => {
      current = ready;
      return current;
    });
    const erase = vi.fn(async () => {
      current = empty;
      return current;
    });
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetRhythmMarkers: async () => current,
          AddRhythmMarker: add,
          DeleteRhythmMarker: erase,
        },
      },
    };
    window.location.hash = "#/log/markers";
    render(<App />);
    expect(await screen.findByText("No markers recorded")).toBeVisible();

    fireEvent.change(screen.getByLabelText("Started"), {
      target: { value: "2026-07-22T09:00" },
    });
    fireEvent.change(screen.getByLabelText("IANA time zone"), {
      target: { value: "America/New_York" },
    });
    fireEvent.change(screen.getByLabelText("Private note (optional)"), {
      target: { value: "Arrival context" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Append marker" }));
    expect(await screen.findByText("Arrival context")).toBeVisible();
    expect(add).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "travel",
        startLocal: "2026-07-22T09:00",
        zoneId: "America/New_York",
        note: "Arrival context",
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Erase" }));
    fireEvent.change(screen.getByLabelText("Type DELETE"), { target: { value: "DELETE" } });
    fireEvent.click(screen.getByRole("button", { name: "Permanently erase" }));
    expect(await screen.findByText("No markers recorded")).toBeVisible();
    expect(erase).toHaveBeenCalledWith({ markerId: "marker_local_01", confirmation: "DELETE" });
  });

  it("drives the Sources tab from real local data instead of fixtures", async () => {
    window.location.hash = "#/rhythm";
    const correctedEntry = {
      observationId: "obs_sleep_01",
      startLocal: "2026-03-01T22:00",
      endLocal: "2026-03-02T06:00",
      startLabel: "Sun Mar 1, 10:00 PM EST",
      endLabel: "Mon Mar 2, 6:00 AM EST",
      zoneId: "America/New_York",
      classification: "principal",
      effectiveStartLocal: "2026-03-01T22:30",
      effectiveEndLocal: "2026-03-02T06:00",
      effectiveStartLabel: "Sun Mar 1, 10:30 PM EST",
      effectiveEndLabel: "Mon Mar 2, 6:00 AM EST",
      effectiveClassification: "principal",
      durationLabel: "7 hours 30 minutes",
      suppressed: false,
      sourceLabel: "Manual sleep log",
      provenanceLabel: "manual / user reported",
      history: [
        {
          correctionId: "corr_sleep_01",
          createdLabel: "Mar 2, 6:15 AM",
          reason: "user edit",
          summary: "start moved to Mar 1, 10:30 PM",
        },
      ],
    };
    const suppressedEntry = {
      ...correctedEntry,
      observationId: "obs_sleep_02",
      suppressed: true,
      history: [],
    };
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetRhythm: async () => ({
            estimateSource: "local",
            fixtureMode: false,
            status: "refused",
            refusal: {
              code: "insufficient_data",
              message: "Add at least seven principal sleep episodes.",
            },
            observedRows: [],
            forecastRows: [],
            driftPoints: [],
            now: {
              day: "Jul 4",
              civilDate: "2026-07-04",
              zoneId: "America/New_York",
              hour: 12,
              label: "Now",
            },
            actogramSummary: "No chart yet",
            driftTitle: "Sleep-onset drift",
            slopeLabel: "n/a",
            driftConfidence: "Low",
            driftSummary: "Not enough usable sleep data.",
            yMinHour: 0,
            yMaxHour: 24,
          }),
          ListSleepEntries: async () => ({
            status: "ready",
            empty: false,
            message: "2 local sleep entries stored on this device.",
            entries: [correctedEntry, suppressedEntry],
          }),
        },
      },
    };
    render(<App />);

    await screen.findByText("Local data");
    fireEvent.click(screen.getByRole("tab", { name: "Sources" }));

    // Real refusal, not the refusal fixture.
    expect(
      await screen.findByRole("heading", { name: "The estimator is refusing, not guessing" }),
    ).toBeVisible();
    expect(screen.getByText("insufficient_data")).toBeVisible();

    // Real correction history drives the inspector; the fixture undo button is gone.
    expect(await screen.findByRole("heading", { name: "1 corrected entry" })).toBeVisible();
    expect(screen.getByText("start moved to Mar 1, 10:30 PM")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Undo correction" })).toBeNull();
    expect(screen.queryByText("Wearable sleep overlaps desktop activity")).toBeNull();

    // Real per-source composition with suppression counts.
    expect(screen.getByRole("heading", { name: "What the estimator sees" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Manual sleep log" })).toBeVisible();
    expect(
      screen.getByText(/2 entries, 1 corrected, 1 suppressed from estimates/, { selector: "p" }),
    ).toBeVisible();
  });

  it("renders the rhythm drift visualizer instead of a placeholder", async () => {
    window.location.hash = "#/rhythm";
    render(<App />);

    fireEvent.click(await screen.findByRole("tab", { name: "Drift" }));

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

  it("keeps the legacy timeline route usable as Rhythm", async () => {
    window.location.hash = "#/timeline";
    render(<App />);

    expect(await screen.findByRole("heading", { name: "Rhythm" })).toBeVisible();
  });

  it("validates manual sleep entry civil ranges", async () => {
    window.location.hash = "#/log/sleep";
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

    fireEvent.change(await screen.findByLabelText("Sleep start"), {
      target: { value: "2026-03-02T08:00" },
    });
    fireEvent.change(screen.getByLabelText("Wake time"), {
      target: { value: "2026-03-02T07:00" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save sleep entry" }));

    expect(screen.getByRole("alert")).toHaveTextContent("Wake time must be after sleep start.");
  });

  it("submits a manual sleep entry through Wails and reloads the local log", async () => {
    window.location.hash = "#/log/sleep";
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

    fireEvent.click(screen.getByRole("link", { name: /^Plan/ }));
    fireEvent.click(await screen.findByRole("tab", { name: /Approvals/ }));
    expect(await screen.findByText("Local sleep data follow-up")).toBeVisible();
  });

  it("hard-deletes a sleep entry through the sleep log confirmation flow", async () => {
    window.location.hash = "#/log/sleep";
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
      history: [
        {
          correctionId: "corr_sleep_01",
          createdLabel: "Mar 2, 6:15 AM",
          reason: "user edit",
          summary: "excluded from estimates",
        },
      ],
    };
    const deleteSleep = vi.fn(async () => ({
      status: "empty",
      empty: true,
      message: "No sleep entries yet.",
      entries: [],
    }));
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          ListSleepEntries: async () => ({
            status: "ready",
            empty: false,
            message: "1 local sleep entry stored on this device.",
            entries: [entry],
          }),
          DeleteSleepObservation: deleteSleep,
        },
      },
    };

    render(<App />);

    expect(
      await screen.findByText(/Sun Mar 1, 10:00 PM EST to Mon Mar 2, 6:00 AM EST/),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "Suppress from estimates" })).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Delete permanently" }));
    const eraseButton = screen.getByRole("button", { name: "Erase entry" });
    expect(eraseButton).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Deletion confirmation"), {
      target: { value: "DELETE" },
    });
    fireEvent.click(eraseButton);

    expect(await screen.findByText("Sleep entry erased permanently.")).toBeVisible();
    expect(deleteSleep).toHaveBeenCalledWith({
      observationId: "obs_sleep_01",
      confirmation: "DELETE",
    });
    expect(screen.getByRole("heading", { name: "No sleep entries yet" })).toBeVisible();
  });

  it("preserves task form input when saving fails", async () => {
    window.location.hash = "#/plan/tasks";
    const addTask = vi.fn(async () => {
      throw new Error("Task storage is unavailable.");
    });
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          ListTasks: async () => ({ status: "ok", tasks: [] }),
          AddTask: addTask,
        },
      },
    };

    render(<App />);

    const taskInput = await screen.findByLabelText("Task");
    fireEvent.change(taskInput, { target: { value: "Call clinic" } });
    fireEvent.click(screen.getByRole("button", { name: "Save task" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Task storage is unavailable.");
    expect(taskInput).toHaveValue("Call clinic");
    expect(addTask).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Call clinic", durationMinutes: 45 }),
    );
  });

  it("applies Settings appearance controls through the visible UI", async () => {
    window.location.hash = "#/settings";
    render(
      <AppearanceProvider>
        <App />
      </AppearanceProvider>,
    );

    const auto = await screen.findByRole("radio", { name: /Auto/ });
    const dark = screen.getByRole("radio", { name: /Dark/ });
    const paper = screen.getByRole("radio", { name: /Paper/ });
    const reduced = screen.getByRole("checkbox", { name: /Reduced stimulation/ });

    expect(auto).toBeChecked();
    expect(reduced).not.toBeChecked();

    fireEvent.click(dark);
    fireEvent.click(reduced);

    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    expect(document.documentElement).toHaveAttribute("data-reduced", "true");
    expect(document.querySelector('meta[name="theme-color"]')).toHaveAttribute(
      "content",
      "#161b19",
    );
    expect(localStorage.getItem("zeitboard-theme")).toBe("dark");
    expect(localStorage.getItem("zeitboard-reduced")).toBe("true");

    fireEvent.click(paper);

    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    expect(document.querySelector('meta[name="theme-color"]')).toHaveAttribute(
      "content",
      "#f3f0e9",
    );
  });

  it("enables, syncs, and disables backend sync from Settings", async () => {
    window.location.hash = "#/settings";
    const offStatus = {
      enabled: false,
      status: "off",
      backendUrl: "",
      deviceId: "",
      insecureSkipVerify: false,
      lastSyncLabel: "Not synced yet",
      lastError: "",
      pendingPushCount: 0,
      pushedCount: 0,
      pulledCount: 0,
      cursor: 0,
    };
    const connectedStatus = {
      ...offStatus,
      enabled: true,
      status: "connected",
      backendUrl: "https://localhost:8443",
      deviceId: "device_desktop",
      insecureSkipVerify: true,
      lastSyncLabel: "Not synced yet",
      pendingPushCount: 1,
    };
    const getStatus = vi.fn(async () => offStatus);
    const configure = vi.fn(async () => connectedStatus);
    const sync = vi.fn(async () => ({
      ...connectedStatus,
      pendingPushCount: 0,
      pushedCount: 1,
      pulledCount: 2,
      lastSyncLabel: "Last synced Mar 2, 6:00 AM",
    }));
    const disable = vi.fn(async () => offStatus);
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetBackendSyncStatus: getStatus,
          ConfigureBackendSync: configure,
          SyncNow: sync,
          DisableBackendSync: disable,
        },
      },
    };

    render(
      <AppearanceProvider>
        <App />
      </AppearanceProvider>,
    );

    expect(await screen.findByRole("heading", { name: "Self-hosted server" })).toBeVisible();
    fireEvent.change(screen.getByLabelText("Backend URL"), {
      target: { value: "https://localhost:8443" },
    });
    fireEvent.change(screen.getByLabelText("Enrollment secret"), {
      target: { value: "enroll-secret" },
    });
    fireEvent.change(screen.getByLabelText("Device label"), {
      target: { value: "Desktop test" },
    });
    fireEvent.click(screen.getByLabelText(/Allow self-signed localhost TLS/));
    fireEvent.click(screen.getByRole("button", { name: "Enable backend sync" }));

    expect(await screen.findByText(/Backend sync enabled/)).toBeVisible();
    expect(configure).toHaveBeenCalledWith({
      enabled: true,
      backendUrl: "https://localhost:8443",
      enrollmentSecret: "enroll-secret",
      deviceLabel: "Desktop test",
      insecureSkipVerify: true,
    });
    expect(screen.getByText("Connected")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Sync now" }));
    expect(await screen.findByText("Sync complete: 1 pushed, 2 pulled.")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Disable sync" }));
    expect(await screen.findByText(/Backend sync disabled/)).toBeVisible();
    expect(disable).toHaveBeenCalled();
  });

  it("exports and erases all local sleep data from Settings", async () => {
    window.location.hash = "#/settings";
    const exportSleep = vi.fn(async () => ({
      fileName: "zeitboard-sleep-export-20260302-060000.json",
      json: '{"schema_version":"v1","observation_set":{"observations":[]},"correction_set":{"corrections":[]}}',
      generatedLabel: "Mar 2, 2026, 6:00 AM",
      observationCount: 1,
      correctionCount: 1,
    }));
    const deleteAll = vi.fn(async () => ({
      status: "empty",
      empty: true,
      message: "No sleep entries yet.",
      entries: [],
    }));
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          ExportSleepData: exportSleep,
          DeleteAllSleepData: deleteAll,
        },
      },
    };

    render(
      <AppearanceProvider>
        <App />
      </AppearanceProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: "Export sleep data" }));

    expect(await screen.findByText(/1 observation and 1 correction/)).toBeVisible();
    expect(screen.getByLabelText("Sleep data export JSON preview")).toHaveTextContent(
      '{"schema_version":"v1","observation_set":{"observations":[]},"correction_set":{"corrections":[]}}',
    );
    expect(screen.getByText("zeitboard-sleep-export-20260302-060000.json")).toBeVisible();
    expect(screen.getByText("Mar 2, 2026, 6:00 AM")).toBeVisible();
    expect(screen.getByText("1 observation, 1 correction")).toBeVisible();
    const eraseAll = screen.getByRole("button", { name: "Erase all sleep data" });
    expect(eraseAll).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/Type DELETE to erase all local sleep data/), {
      target: { value: "DELETE" },
    });
    fireEvent.click(eraseAll);

    expect(
      await screen.findByText("All local sleep observations and correction history were erased."),
    ).toBeVisible();
    expect(deleteAll).toHaveBeenCalledWith({ confirmation: "DELETE" });
  });
});

describe("assistant rail", () => {
  it("opens propose-only chat with honest offline state and disclaimer", async () => {
    render(<App />);

    const toggle = screen.getByRole("button", { name: "Assistant", pressed: false });
    fireEvent.click(toggle);

    expect(await screen.findByRole("complementary", { name: "Assistant" })).toBeVisible();
    expect(
      screen.getByText("Manages your schedule via approvals. Not medical advice."),
    ).toBeVisible();
    // Browser preview has no desktop bridge: the rail says so and disables input.
    expect(await screen.findByText(/desktop app/)).toBeVisible();
    expect(screen.getByLabelText("Message the assistant")).toBeDisabled();
    expect(screen.getByText("Offline")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Close assistant" }));
    expect(screen.queryByRole("complementary", { name: "Assistant" })).toBeNull();
  });

  it("sends a message and renders the propose-only action card with decisions", async () => {
    const decide = vi.fn(async () => ({ status: "ok", proposals: [] }));
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetAssistantStatus: async () => ({
            enabled: true,
            configured: true,
            provider: "anthropic",
            model: "claude-sonnet-5",
          }),
          GetBackendProposals: async () => ({ status: "ok", proposals: [] }),
          SendAssistantMessage: async () => ({
            available: true,
            result: "proposal_pending",
            answer: "I found a window inside your predicted waking time.",
            configured: true,
            provider: "anthropic",
            proposals: [
              {
                proposalId: "proposal_srv_01",
                action: "propose_place_task",
                status: "pending",
                title: "Place task “Call clinic”",
                window: "Thu Jul 10, 11:00 AM to 11:45 AM EDT",
                confidence: "Medium",
                reasonLabels: ["Fits the predicted waking window"],
                createdLabel: "Proposed Jul 10, 8:00 AM",
                expiresLabel: "expires Jul 10, 8:15 AM",
                decisionToken: "one-use-token",
              },
            ],
          }),
          DecideBackendProposal: decide,
        },
      },
    };
    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: "Assistant", pressed: false }));
    expect(await screen.findByText("Connected: anthropic")).toBeVisible();

    // Example chips appear on the empty state; clicking one sends it.
    fireEvent.click(await screen.findByRole("button", { name: "What's my next good window?" }));

    expect(
      await screen.findByText("I found a window inside your predicted waking time."),
    ).toBeVisible();
    expect(screen.getByText("Place task “Call clinic”")).toBeVisible();
    expect(screen.getByText("Thu Jul 10, 11:00 AM to 11:45 AM EDT")).toBeVisible();
    expect(screen.getByRole("link", { name: "View in Approvals" })).toBeVisible();

    // Approve goes through the same one-use-token queue decision.
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await screen.findByText("approved");
    expect(decide).toHaveBeenCalledWith({
      proposalId: "proposal_srv_01",
      decision: "approved",
      token: "one-use-token",
    });
  });
});

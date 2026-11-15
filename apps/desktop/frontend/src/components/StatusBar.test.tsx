import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { normalizeReportedStatus } from "../data/status";
import { StatusBar } from "./StatusBar";

const nothing = {
  status: "ok",
  reported: "",
  shared: false,
  delivery: "this_computer",
  settings: { shareByDefault: false, followSleepTaps: false },
  settingsRevision: 0,
};
const standing = {
  ...nothing,
  reported: "trying_to_sleep",
  reportedLabel: "Trying to sleep",
  reportedAt: "2026-11-14T03:00:00Z",
  expiresAt: "2026-11-14T13:00:00Z",
  saidLabel: "Sat 3:00 AM",
  untilLabel: "Sat 1:00 PM",
  delivery: "waiting",
};

function install(app: Record<string, unknown>) {
  (globalThis as { go?: unknown }).go = { main: { App: app } };
}

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("the status the owner reports", () => {
  it("is not offered where it cannot be kept", () => {
    const { container } = render(<StatusBar />);
    expect(container).toBeEmptyDOMElement();
  });

  it("reports a status kept to the owner's devices unless they tick otherwise", async () => {
    const ReportStatus = vi.fn(async () => standing);
    install({ GetReportedStatus: async () => nothing, ReportStatus });
    render(<StatusBar />);

    expect(await screen.findByText(/Nothing reported/)).toBeVisible();
    const share = screen.getByLabelText("Let my share links show the next one");
    expect(share).not.toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "Trying to sleep" }));

    await waitFor(() =>
      expect(ReportStatus).toHaveBeenCalledWith({
        status: "trying_to_sleep",
        minutes: 0,
        shared: false,
      }),
    );
    const said = await screen.findByRole("status");
    expect(said).toHaveTextContent("Trying to sleep");
    expect(said).toHaveTextContent("Said Sat 3:00 AM, until Sat 1:00 PM.");
    expect(said).toHaveTextContent("Kept to your own devices.");
    // Where it has got to is said, not assumed.
    expect(said).toHaveTextContent("Not yet on your server");
    expect(screen.getByRole("button", { name: "Trying to sleep" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  it("shares one report when ticked, and takes a report back", async () => {
    const ReportStatus = vi.fn(async () => ({ ...standing, shared: true, delivery: "server" }));
    const ClearReportedStatus = vi.fn(async () => nothing);
    install({ GetReportedStatus: async () => nothing, ReportStatus, ClearReportedStatus });
    render(<StatusBar />);

    fireEvent.click(await screen.findByLabelText("Let my share links show the next one"));
    // And for how long, where the usual length is wrong for this one.
    const lasting = screen.getByLabelText("The next one lasts");
    expect(lasting).toHaveValue("0");
    fireEvent.change(lasting, { target: { value: "60" } });
    fireEvent.click(screen.getByRole("button", { name: "Do not disturb" }));
    await waitFor(() =>
      expect(ReportStatus).toHaveBeenCalledWith({
        status: "do_not_disturb",
        minutes: 60,
        shared: true,
      }),
    );
    // Both choices were for that report, and go back to the usual ones.
    await waitFor(() => expect(screen.getByLabelText("The next one lasts")).toHaveValue("0"));
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Share links you allowed may show it. It has reached your server.",
    );

    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    await waitFor(() => expect(ClearReportedStatus).toHaveBeenCalled());
    expect(await screen.findByText(/Nothing reported/)).toBeVisible();
    expect(screen.queryByRole("button", { name: "Clear" })).toBeNull();
  });

  it("says why a report was not kept", async () => {
    install({
      GetReportedStatus: async () => nothing,
      ReportStatus: async () => {
        throw "the store is closed";
      },
    });
    render(<StatusBar />);
    fireEvent.click(await screen.findByRole("button", { name: "Awake" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The store is closed.");
  });
});

describe("reading a status from the desktop", () => {
  it("refuses a standing status that does not say when it was said", () => {
    expect(normalizeReportedStatus(standing)).toMatchObject({ reported: "trying_to_sleep" });
    expect(normalizeReportedStatus({ ...standing, saidLabel: "" })).toBeUndefined();
    expect(normalizeReportedStatus({ ...standing, reported: "asleep" })).toBeUndefined();
    expect(normalizeReportedStatus({ ...nothing, delivery: "somewhere" })).toBeUndefined();
  });
});

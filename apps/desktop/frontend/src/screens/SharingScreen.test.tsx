import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SharingScreen } from "./SharingScreen";

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
  vi.restoreAllMocks();
});

const activeLink = {
  profileId: "prof_abc123",
  label: "Mum",
  state: "active",
  stateLabel: "Working now",
  createdLabel: "Created Aug 1, 2026",
  expiresLabel: "Expires Sep 1, 2026",
  grants: { wakingWindows: true, allowRequests: true, allowMessages: false },
  grantSummary: "Shows when you are likely awake; can ask for a time",
  access: [
    {
      event: "availability_read",
      label: "Opened the page",
      count: 4,
      lastLabel: "last Aug 3, 9:12 AM",
    },
  ],
};

function connected(overrides: Record<string, unknown> = {}) {
  return {
    status: "ok",
    disclosure:
      "Anyone with this link and passcode can see broad windows when you are likely awake.",
    links: [activeLink],
    minPasscodeLength: 6,
    maxDays: 90,
    ...overrides,
  };
}

function mount(app: Record<string, unknown>) {
  (globalThis as { go?: unknown }).go = { main: { App: app } };
}

describe("SharingScreen", () => {
  // The defect this slice closes: the screen claimed the system could not do
  // something it had been able to do since P5-a. Honest about the user's
  // experience, wrong about the system — and that is the worse way round.
  it("no longer claims link creation is unbuilt", async () => {
    mount({ GetBackendShareLinks: async () => connected() });
    render(<SharingScreen />);

    await screen.findByText("Mum");
    expect(screen.queryByText(/not connected in this build/i)).toBeNull();
    expect(screen.queryByText("Example only")).toBeNull();
    expect(screen.getByText("One link is working right now")).toBeVisible();
  });

  // "Sync is off" and "your server's portal is off" are different problems with
  // different fixes, and the screen has to say which.
  it.each([
    ["off", "Turn on backend sync", "Sync is off"],
    ["unavailable", "portal is switched off", "Portal switched off"],
  ])("distinguishes the %s state", async (status, message, transport) => {
    mount({
      GetBackendShareLinks: async () => ({
        status,
        message: `Sharing runs on your own server. ${message}.`,
        disclosure: "",
        links: [],
        minPasscodeLength: 6,
        maxDays: 90,
      }),
    });
    render(<SharingScreen />);

    expect(await screen.findByText(new RegExp(message, "i"))).toBeVisible();
    expect(screen.getByText(transport)).toBeVisible();
    // No create form when there is nowhere to create it.
    expect(screen.queryByRole("button", { name: "Create link" })).toBeNull();
  });

  // Exposure gate item 7: an owner must not be able to create a link without
  // having seen what it reveals.
  it("shows the disclosure above the create button", async () => {
    mount({ GetBackendShareLinks: async () => connected() });
    const { container } = render(<SharingScreen />);

    const disclosure = await screen.findByRole("note");
    const button = screen.getByRole("button", { name: "Create link" });
    expect(container).toContainElement(disclosure);
    expect(disclosure.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(
      0,
    );
  });

  // The server keeps only a hash of the address, so this is genuinely the one
  // and only showing and the screen must say so.
  it("says the new link cannot be shown again", async () => {
    const create = vi.fn(async () => ({
      status: "ok",
      linkUrl: "https://share.example.test/p/opaque-token",
      expiresLabel: "Expires Sep 1, 2026",
      links: connected(),
    }));
    mount({ GetBackendShareLinks: async () => connected(), CreateBackendShareLink: create });
    render(<SharingScreen />);

    fireEvent.change(await screen.findByLabelText(/Name/), { target: { value: "Clinician" } });
    fireEvent.change(screen.getByLabelText(/Passcode/), { target: { value: "long-enough" } });
    fireEvent.click(screen.getByRole("button", { name: "Create link" }));

    expect(await screen.findByText(/can show it again/i)).toBeVisible();
    expect(screen.getByText("https://share.example.test/p/opaque-token")).toBeVisible();
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({ label: "Clinician", passcode: "long-enough" }),
    );
  });

  // Revoking stops access. Erasing removes the record that the link existed,
  // and must not be reachable by the same single click.
  it("asks for the link id before erasing the record", async () => {
    const erase = vi.fn(async () => connected({ links: [] }));
    mount({ GetBackendShareLinks: async () => connected(), EraseBackendShareLink: erase });
    render(<SharingScreen />);

    fireEvent.click(await screen.findByRole("button", { name: "Erase record" }));
    const panel = screen.getByRole("group", { name: /Erase Mum/ });
    const confirm = within(panel).getByRole("button", { name: "Erase permanently" });
    expect(confirm).toBeDisabled();

    fireEvent.change(within(panel).getByLabelText("Link id"), { target: { value: "wrong" } });
    expect(confirm).toBeDisabled();

    fireEvent.change(within(panel).getByLabelText("Link id"), { target: { value: "prof_abc123" } });
    expect(confirm).toBeEnabled();
    fireEvent.click(confirm);
    await waitFor(() =>
      expect(erase).toHaveBeenCalledWith({
        profileId: "prof_abc123",
        confirmation: "prof_abc123",
      }),
    );
  });

  it("revokes without a confirmation step", async () => {
    const revoke = vi.fn(async () =>
      connected({ links: [{ ...activeLink, state: "revoked", stateLabel: "Revoked" }] }),
    );
    mount({ GetBackendShareLinks: async () => connected(), RevokeBackendShareLink: revoke });
    render(<SharingScreen />);

    fireEvent.click(await screen.findByRole("button", { name: "Revoke" }));
    await waitFor(() => expect(revoke).toHaveBeenCalledWith({ profileId: "prof_abc123" }));
    expect(await screen.findByText("Revoked")).toBeVisible();
  });

  // Threads are P5-c. The grant exists so a link does not need re-issuing, and
  // the label says plainly that nothing is delivered yet.
  it("does not imply messaging works", async () => {
    mount({ GetBackendShareLinks: async () => connected() });
    render(<SharingScreen />);
    expect(await screen.findByText(/Send a short message \(not delivered yet\)/)).toBeVisible();
  });
});

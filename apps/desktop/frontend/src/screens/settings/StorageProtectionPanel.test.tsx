import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StorageProtectionPanel } from "./StorageProtectionPanel";

const report = (encryption: Record<string, unknown>) => ({
  state: "ok",
  headline: "Local files are restricted to your account.",
  detail:
    "These files are restricted to your account, not encrypted. Anyone who can read this disk from another operating system, and any program running as you, can still read them.",
  files: [{ name: "Local database and backend credential", ownerOnly: true, inherited: false }],
  encryption,
});

function install(methods: Record<string, (input?: unknown) => Promise<unknown>>) {
  (globalThis as unknown as { go?: unknown }).go = { main: { App: methods } };
}

afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

describe("StorageProtectionPanel", () => {
  // What could lose the files is said before the choice, and the choice says
  // when it takes effect.
  it("warns about the key before encrypting, then says when it happens", async () => {
    const choose = vi.fn(async () =>
      report({ supported: true, chosen: true, encryptedFiles: 0, files: 4, pending: true }),
    );
    install({
      GetStorageProtection: async () =>
        report({ supported: true, chosen: false, encryptedFiles: 0, files: 4, pending: false }),
      SetStorageEncryption: choose,
    });
    render(<StorageProtectionPanel />);

    const button = await screen.findByRole("button", { name: "Encrypt with Windows" });
    expect(screen.getByText(/without it, a password reset by an administrator/)).toBeVisible();
    expect(screen.getByText("Not encrypted.")).toBeVisible();
    expect(choose).not.toHaveBeenCalled();

    fireEvent.click(button);
    expect(
      await screen.findByText(/Windows encrypts the files when ZeitBoard next starts/),
    ).toBeVisible();
    expect(choose).toHaveBeenCalledWith({ encrypt: true });
    expect(screen.getByRole("button", { name: "Stop encrypting" })).toBeEnabled();
  });

  it("names each encrypted file, and says when all of them are", async () => {
    install({
      GetStorageProtection: async () => ({
        ...report({ supported: true, chosen: true, encryptedFiles: 4, files: 4, pending: false }),
        files: [
          {
            name: "Local database and backend credential",
            ownerOnly: true,
            inherited: false,
            encrypted: true,
          },
        ],
      }),
    });
    render(<StorageProtectionPanel />);
    expect(await screen.findByText("Windows encrypts all 4 files.")).toBeVisible();
    expect(screen.getByText("restricted to your account, encrypted by Windows")).toBeVisible();
  });

  it("points elsewhere where Windows cannot encrypt files", async () => {
    install({
      GetStorageProtection: async () => report({ supported: false, platform: "windows" }),
    });
    render(<StorageProtectionPanel />);
    expect(await screen.findByText(/This edition of Windows cannot encrypt files/)).toBeVisible();
    expect(screen.queryByRole("button", { name: "Encrypt with Windows" })).toBeNull();
  });

  // On another system the disk encryption it has is the way, and Windows is
  // not mentioned as though it were there.
  it.each([
    ["macos", /FileVault, in System Settings › Privacy & Security/],
    ["linux", /encrypting the whole disk, which most systems offer when they are installed/],
  ])("points %s to its own disk encryption", async (platform, copy) => {
    install({
      GetStorageProtection: async () => report({ supported: false, platform }),
    });
    render(<StorageProtectionPanel />);
    expect(await screen.findByText(copy)).toBeVisible();
    expect(screen.getByRole("heading", { name: "Encryption" })).toBeVisible();
    expect(screen.queryByText(/This edition of Windows/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Encrypt with Windows" })).toBeNull();
  });
});

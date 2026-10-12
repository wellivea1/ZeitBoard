import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ClaudePackagesCard } from "./ClaudePackagesCard";

function install(methods: Record<string, () => Promise<unknown>>) {
  (globalThis as unknown as { go?: unknown }).go = { main: { App: methods } };
}

afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

describe("ClaudePackagesCard", () => {
  it("says where the data goes before anything is saved, then saves each file", async () => {
    const extension = vi.fn(async () => ({ fileName: "ZeitBoard.mcpb", saved: true }));
    const skill = vi.fn(async () => ({ fileName: "zeitboard-skill.zip", saved: true }));
    install({ SaveClaudeExtension: extension, SaveClaudeSkill: skill });
    render(<ClaudePackagesCard />);

    expect(screen.getByText(/goes to the model Claude uses, in Anthropic's cloud/)).toBeVisible();
    expect(extension).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Save the Claude Desktop extension" }));
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Saved ZeitBoard.mcpb. Open it to add ZeitBoard to Claude Desktop.",
    );
    fireEvent.click(screen.getByRole("button", { name: "Save the skill" }));
    expect(await screen.findByText(/Saved zeitboard-skill.zip/)).toBeVisible();
    expect(skill).toHaveBeenCalledTimes(1);
  });

  it("says nothing when the dialog is closed, and why when saving fails", async () => {
    install({
      SaveClaudeExtension: async () => ({ fileName: "", saved: false }),
      SaveClaudeSkill: async () => {
        throw new Error("The local connector is not installed beside ZeitBoard.");
      },
    });
    render(<ClaudePackagesCard />);
    fireEvent.click(screen.getByRole("button", { name: "Save the Claude Desktop extension" }));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(screen.queryByRole("status")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Save the skill" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("not installed beside ZeitBoard");
  });

  it("cannot save outside the desktop app", () => {
    render(<ClaudePackagesCard />);
    expect(screen.getByRole("button", { name: "Save the skill" })).toBeDisabled();
    expect(screen.getByText("Saving them needs the ZeitBoard desktop app.")).toBeVisible();
  });
});

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ShareLink } from "../data/sharing";
import { ShareLinkShown } from "./ShareLinkShown";

const link: ShareLink = {
  profileId: "profile_synthetic",
  label: "Sam (synthetic)",
  state: "active",
  stateLabel: "Working",
  createdLabel: "Created Nov 14, 2026",
  expiresLabel: "Expires Nov 28, 2026",
  grants: {
    wakingWindows: true,
    allowRequests: false,
    allowMessages: false,
    skippedDoses: false,
    reportedStatus: "",
  },
  grantSummary: "when you are likely awake",
  access: [],
};

describe("what an existing link shows", () => {
  it("starts from what the link shows now, and says what each level means", () => {
    render(<ShareLinkShown link={link} busy={false} onSave={() => {}} onCancel={() => {}} />);
    expect(screen.getByLabelText("When you are likely awake")).toBeChecked();
    expect(screen.getByRole("combobox")).toHaveValue("");
    expect(screen.getByText(/nothing you report/)).toBeVisible();

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "contact" } });
    expect(screen.getByText(/Not which, and not since when/)).toBeVisible();
  });

  it("saves the level chosen, with the rest as it stood", () => {
    const onSave = vi.fn();
    render(<ShareLinkShown link={link} busy={false} onSave={onSave} onCancel={() => {}} />);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "full" } });
    fireEvent.click(screen.getByRole("button", { name: "Save what it shows" }));
    expect(onSave).toHaveBeenCalledWith({
      wakingWindows: true,
      skippedDoses: false,
      reportedStatus: "full",
    });
  });

  it("will not save a link that shows nothing", () => {
    const onSave = vi.fn();
    render(<ShareLinkShown link={link} busy={false} onSave={onSave} onCancel={() => {}} />);
    fireEvent.click(screen.getByLabelText("When you are likely awake"));
    expect(screen.getByRole("alert")).toHaveTextContent("Revoke it instead");
    expect(screen.getByRole("button", { name: "Save what it shows" })).toBeDisabled();
    expect(onSave).not.toHaveBeenCalled();
  });
});

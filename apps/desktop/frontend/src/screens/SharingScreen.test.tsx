import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { SharingScreen } from "./SharingScreen";

describe("SharingScreen", () => {
  it("presents honest default-deny examples without fake active profiles", () => {
    const { container } = render(<SharingScreen />);
    const table = screen.getByRole("table", { name: "Sharing templates" });

    expect(screen.getByRole("heading", { name: "No trusted view is being shared" })).toBeVisible();
    expect(within(table).getAllByRole("row")).toHaveLength(5);
    expect(within(table).getByRole("cell", { name: "Clinician" })).toBeVisible();
    expect(screen.getAllByText("Example only")).toHaveLength(4);
    expect(screen.queryByText("Active")).toBeNull();
    expect(container.querySelector(".panel, .avatar")).toBeNull();
  });
});

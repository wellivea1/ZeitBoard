import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import App from "./App";

beforeEach(() => {
  window.history.replaceState(null, "", "/?profile=planning");
});

describe("trusted profile selection", () => {
  it("shows only the selected projection fields", () => {
    render(<App />);
    fireEvent.click(screen.getByRole("button", { name: "Availability only" }));

    expect(screen.getByText("1 allowlisted field")).toBeVisible();
    expect(screen.queryByText("Predicted sleep window")).not.toBeInTheDocument();
    expect(screen.queryByText("Predicted waking window")).not.toBeInTheDocument();
  });

  it("always renders the fixed notice for an active projection", () => {
    render(<App />);
    expect(
      screen.getByText("Estimated windows are uncertain and are not medical advice."),
    ).toBeVisible();
  });

  it("renders the same safe content for expired and revoked profiles", () => {
    const { container } = render(<App />);
    fireEvent.click(screen.getByRole("button", { name: "Expired view" }));
    const expired = container.querySelector(".status-panel")?.innerHTML;

    fireEvent.click(screen.getByRole("button", { name: "Revoked view" }));
    const revoked = container.querySelector(".status-panel")?.innerHTML;

    expect(revoked).toBe(expired);
    expect(screen.getByRole("heading", { name: "Link unavailable" })).toBeVisible();
  });
});

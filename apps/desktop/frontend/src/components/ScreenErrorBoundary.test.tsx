import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ScreenErrorBoundary } from "./ScreenErrorBoundary";

describe("view recovery", () => {
  it("keeps recovery controls visible after a render failure and can retry", () => {
    let failed = true;
    function BrokenView() {
      if (failed) throw new Error("synthetic rendering failure");
      return <h1>Recovered rhythm</h1>;
    }
    const log = vi.spyOn(console, "error").mockImplementation(() => {});
    try {
      render(
        <ScreenErrorBoundary>
          <BrokenView />
        </ScreenErrorBoundary>,
      );
      expect(screen.getByRole("alert")).toHaveTextContent("This view could not open");
      expect(screen.getByRole("link", { name: "Go to Home" })).toHaveAttribute("href", "#/home");
      failed = false;
      fireEvent.click(screen.getByRole("button", { name: "Try again" }));
      expect(screen.getByRole("heading", { name: "Recovered rhythm" })).toBeVisible();
    } finally {
      log.mockRestore();
    }
  });
});

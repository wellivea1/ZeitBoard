import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { HomeSetupGuide } from "./HomeSetupGuide";

function install(steps: { id: string; done: boolean }[]) {
  (globalThis as unknown as { go?: unknown }).go = {
    main: { App: { GetSetupGuide: async () => ({ steps }) } },
  };
}

beforeEach(() => {
  window.localStorage.clear();
});

afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

describe("HomeSetupGuide", () => {
  it("lists each step with where it is done, and marks the ones that are", async () => {
    install([
      { id: "phone", done: false },
      { id: "reaching", done: true },
      { id: "calendar", done: false },
      { id: "startup", done: false },
    ]);
    render(<HomeSetupGuide />);

    const note = await screen.findByRole("complementary", { name: "More you can set up" });
    expect(note).toHaveTextContent("1 of 4 done.");
    expect(screen.getByRole("link", { name: "Set up sync" })).toHaveAttribute(
      "href",
      "#/settings/sync",
    );
    expect(screen.getByRole("heading", { name: "Who you need to reach, done" })).toBeVisible();
    // A done step drops its explanation; what is left keeps one.
    expect(note).not.toHaveTextContent("Home marks when a clinic");
    expect(note).toHaveTextContent("On Android, the companion reads Health Connect");
    expect(screen.getByRole("link", { name: "Change" })).toHaveAttribute(
      "href",
      "#/settings/reaching",
    );
    expect(screen.getByRole("link", { name: "Start at sign-in" })).toBeInTheDocument();
  });

  it("goes by itself once every step is done", async () => {
    install([
      { id: "phone", done: true },
      { id: "reaching", done: true },
    ]);
    const { container } = render(<HomeSetupGuide />);
    // Give the read time to land; the note must still not appear.
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(container).toBeEmptyDOMElement();
  });

  it("shows nothing outside the desktop app", async () => {
    const { container } = render(<HomeSetupGuide />);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(container).toBeEmptyDOMElement();
  });

  // Closed, it stays closed, and Settings › Display can show it again.
  it("closes for good", async () => {
    install([{ id: "calendar", done: false }]);
    const { unmount } = render(<HomeSetupGuide />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Hide the note “More you can set up”" }),
    );
    expect(screen.queryByRole("complementary")).toBeNull();
    unmount();

    const { container } = render(<HomeSetupGuide />);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(container).toBeEmptyDOMElement();
    expect(window.localStorage.getItem("zeitboard.notices.hidden")).toContain("home.setup");
  });
});

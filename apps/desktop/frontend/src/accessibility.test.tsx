import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import App from "./App";

// Every screen, as a screen reader meets it: each control says what it is,
// ids are unique, and each page has one top heading. Structural only; the
// pass with real assistive technology is recorded in the completion plan.
const routes = [
  "#/home",
  "#/plan/tasks",
  "#/plan/week",
  "#/rhythm/actogram",
  "#/rhythm/drift",
  "#/rhythm/sources",
  "#/log/sleep",
  "#/log/medications",
  "#/log/context",
  "#/sharing",
  "#/data-sources",
  "#/settings/display",
  "#/settings/reaching",
  "#/settings/sync",
  "#/settings/computer",
  "#/settings/data",
];

const controls = [
  "button",
  "link",
  "textbox",
  "combobox",
  "checkbox",
  "radio",
  "tab",
  "switch",
  "spinbutton",
  "slider",
  "searchbox",
];

afterEach(() => {
  window.location.hash = "";
});

describe("every screen, as a screen reader meets it", () => {
  for (const route of routes) {
    it(`${route} names every control, repeats no id, and has one top heading`, async () => {
      window.location.hash = route;
      const { container } = render(<App />);
      await screen.findByRole("heading", { level: 1 });
      // Let lazy panels and loaders settle.
      await new Promise((resolve) => setTimeout(resolve, 50));

      const unnamed: string[] = [];
      let inspected = 0;
      for (const role of controls) {
        inspected += screen.queryAllByRole(role).length;
        // An empty name matches exactly the controls a screen reader cannot name.
        for (const element of screen.queryAllByRole(role, { name: "" })) {
          unnamed.push(`${role}: ${element.outerHTML.slice(0, 160)}`);
        }
      }
      // The navigation alone has a dozen: a page that rendered nothing fails.
      expect(inspected).toBeGreaterThanOrEqual(12);
      expect(unnamed).toEqual([]);

      const ids = [...container.querySelectorAll("[id]")].map((element) => element.id);
      const repeated = ids.filter((id, index) => ids.indexOf(id) !== index);
      expect(repeated).toEqual([]);

      expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    });
  }
});

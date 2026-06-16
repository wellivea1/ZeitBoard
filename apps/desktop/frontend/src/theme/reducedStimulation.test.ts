import { beforeEach, describe, expect, it } from "vitest";
import {
  applyReducedStimulationAttribute,
  getStoredReducedStimulation,
  storeReducedStimulation,
} from "./reducedStimulation";

describe("reducedStimulation module", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-reduced");
  });

  it("defaults to false when no preference is stored", () => {
    expect(getStoredReducedStimulation()).toBe(false);
  });

  it("stores and retrieves the enabled state", () => {
    storeReducedStimulation(true);
    expect(getStoredReducedStimulation()).toBe(true);
    expect(localStorage.getItem("zeitboard-reduced")).toBe("true");
  });

  it("stores false as the string 'false'", () => {
    storeReducedStimulation(false);
    expect(getStoredReducedStimulation()).toBe(false);
    expect(localStorage.getItem("zeitboard-reduced")).toBe("false");
  });

  it("applies the reduced-stimulation attribute when enabled", () => {
    applyReducedStimulationAttribute(true);
    expect(document.documentElement.getAttribute("data-reduced")).toBe("true");
  });

  it("removes the reduced-stimulation attribute when disabled", () => {
    document.documentElement.setAttribute("data-reduced", "true");
    applyReducedStimulationAttribute(false);
    expect(document.documentElement.hasAttribute("data-reduced")).toBe(false);
  });
});

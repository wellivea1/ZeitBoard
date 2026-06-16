import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  applyThemeAttribute,
  getStoredTheme,
  resolveEffectiveTheme,
  storeTheme,
  type ThemePreference,
} from "./theme";

function createMediaQueryList(matches: boolean): MediaQueryList {
  return {
    addEventListener: vi.fn(),
    addListener: vi.fn(),
    dispatchEvent: vi.fn(),
    matches,
    media: "(prefers-color-scheme: dark)",
    onchange: null,
    removeEventListener: vi.fn(),
    removeListener: vi.fn(),
  };
}

describe("theme module", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
    document.head.innerHTML = '<meta name="theme-color" content="#f3f0e9" />';
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: vi.fn().mockReturnValue(createMediaQueryList(false)),
      writable: true,
    });
  });

  it("defaults to auto when no preference is stored", () => {
    expect(getStoredTheme()).toBe("auto");
  });

  it("stores and retrieves theme preferences", () => {
    storeTheme("dark");
    expect(getStoredTheme()).toBe("dark");
    expect(localStorage.getItem("zeitboard-theme")).toBe("dark");
  });

  it("falls back to auto for invalid stored values", () => {
    localStorage.setItem("zeitboard-theme", "neon");
    expect(getStoredTheme()).toBe("auto");
  });

  it("resolves light and dark preferences directly", () => {
    expect(resolveEffectiveTheme("light")).toBe("light");
    expect(resolveEffectiveTheme("dark")).toBe("dark");
  });

  it("applies the resolved theme to the document element", () => {
    applyThemeAttribute("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("keeps the browser theme color in sync with live theme changes", () => {
    applyThemeAttribute("dark");
    expect(document.querySelector('meta[name="theme-color"]')).toHaveAttribute(
      "content",
      "#161b19",
    );

    applyThemeAttribute("light");

    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    expect(document.querySelector('meta[name="theme-color"]')).toHaveAttribute(
      "content",
      "#f3f0e9",
    );
  });

  it("resolves auto from the OS color scheme", () => {
    vi.mocked(window.matchMedia).mockReturnValue(createMediaQueryList(true));

    expect(resolveEffectiveTheme("auto")).toBe("dark");
  });

  it("round-trips each valid theme preference", () => {
    const preferences: ThemePreference[] = ["auto", "light", "dark"];
    for (const preference of preferences) {
      storeTheme(preference);
      expect(getStoredTheme()).toBe(preference);
    }
  });
});

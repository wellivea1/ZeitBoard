export type ThemePreference = "auto" | "light" | "dark";

const STORAGE_KEY = "zeitboard-theme";
const THEME_COLOR: Record<"light" | "dark", string> = {
  light: "#f3f0e9",
  dark: "#161b19",
};

export function getStoredTheme(): ThemePreference {
  try {
    const value = typeof window !== "undefined" ? localStorage.getItem(STORAGE_KEY) : null;
    if (value === "auto" || value === "light" || value === "dark") return value;
  } catch {
    // Restricted storage falls back to Auto.
  }
  return "auto";
}

export function storeTheme(preference: ThemePreference): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(STORAGE_KEY, preference);
  } catch {
    // Restricted storage should not make appearance controls unusable.
  }
}

export function resolveEffectiveTheme(preference: ThemePreference): "light" | "dark" {
  if (preference !== "auto") return preference;
  if (typeof window === "undefined" || !window.matchMedia) return "light";
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function applyThemeAttribute(preference: ThemePreference): void {
  if (typeof document === "undefined") return;
  const effective = resolveEffectiveTheme(preference);
  document.documentElement.setAttribute("data-theme", effective);
  document
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute("content", THEME_COLOR[effective]);
}

export function loadAndApplyTheme(): ThemePreference {
  const preference = getStoredTheme();
  applyThemeAttribute(preference);
  return preference;
}

export function listenForSystemThemeChanges(callback: () => void): () => void {
  if (typeof window === "undefined" || !window.matchMedia) return () => {};
  const query = window.matchMedia("(prefers-color-scheme: dark)");
  const handler = () => callback();
  query.addEventListener("change", handler);
  return () => query.removeEventListener("change", handler);
}

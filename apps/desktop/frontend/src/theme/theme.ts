export type ThemePreference = "auto" | "light" | "dark" | "black" | "amber" | "contrast";
export type EffectiveTheme = Exclude<ThemePreference, "auto">;

// Preset registry (ui-refactor-plan.md). "Amber" targets dark amber
// blue-blocking glasses: every foreground is long-wavelength so contrast
// survives the lens; it doubles as a zero-blue-emission evening mode.
export const themePresets: { id: ThemePreference; label: string; hint: string }[] = [
  { id: "auto", label: "Auto", hint: "Follows the system light/dark setting." },
  { id: "light", label: "Paper", hint: "Warm light theme for daytime." },
  { id: "dark", label: "Dark", hint: "Low-glare evening theme." },
  { id: "black", label: "Pitch black", hint: "True black for OLED and night logging." },
  {
    id: "amber",
    label: "Amber (glasses)",
    hint: "Stays high-contrast through dark amber blue-blockers; emits no blue.",
  },
  { id: "contrast", label: "High contrast", hint: "Maximum contrast, strong borders." },
];

const STORAGE_KEY = "zeitboard-theme";
const PREFERENCES: ThemePreference[] = ["auto", "light", "dark", "black", "amber", "contrast"];
const THEME_COLOR: Record<EffectiveTheme, string> = {
  light: "#f3f0e9",
  dark: "#161b19",
  black: "#000000",
  amber: "#000000",
  contrast: "#000000",
};

export function getStoredTheme(): ThemePreference {
  try {
    const value = typeof window !== "undefined" ? localStorage.getItem(STORAGE_KEY) : null;
    if (value && (PREFERENCES as string[]).includes(value)) return value as ThemePreference;
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

export function resolveEffectiveTheme(preference: ThemePreference): EffectiveTheme {
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

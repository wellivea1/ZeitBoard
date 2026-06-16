import { useCallback, useEffect, useState } from "react";
import {
  applyThemeAttribute,
  getStoredTheme,
  listenForSystemThemeChanges,
  storeTheme,
  type ThemePreference,
} from "./theme";
import {
  applyReducedStimulationAttribute,
  getStoredReducedStimulation,
  storeReducedStimulation,
} from "./reducedStimulation";

export interface AppearanceState {
  theme: ThemePreference;
  effectiveTheme: "light" | "dark";
  reducedStimulation: boolean;
  setTheme: (preference: ThemePreference) => void;
  setReducedStimulation: (enabled: boolean) => void;
}

export function useAppearance(): AppearanceState {
  const [theme, setThemeState] = useState<ThemePreference>(getStoredTheme);
  const [effectiveTheme, setEffectiveTheme] = useState<"light" | "dark">(() => {
    if (theme !== "auto") return theme;
    if (typeof window === "undefined" || !window.matchMedia) return "light";
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  });
  const [reducedStimulation, setReducedStimulationState] = useState<boolean>(
    getStoredReducedStimulation,
  );

  const updateEffectiveTheme = useCallback((preference: ThemePreference) => {
    applyThemeAttribute(preference);
    if (preference !== "auto") {
      setEffectiveTheme(preference);
      return;
    }
    const effective =
      typeof window !== "undefined" && window.matchMedia
        ? window.matchMedia("(prefers-color-scheme: dark)").matches
          ? "dark"
          : "light"
        : "light";
    setEffectiveTheme(effective);
  }, []);

  const setTheme = useCallback(
    (preference: ThemePreference) => {
      storeTheme(preference);
      setThemeState(preference);
      updateEffectiveTheme(preference);
    },
    [updateEffectiveTheme],
  );

  const setReducedStimulation = useCallback((enabled: boolean) => {
    storeReducedStimulation(enabled);
    setReducedStimulationState(enabled);
    applyReducedStimulationAttribute(enabled);
  }, []);

  useEffect(() => {
    applyThemeAttribute(theme);
    applyReducedStimulationAttribute(reducedStimulation);
    // Intentionally run once on mount to sync the DOM with stored preferences.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    return listenForSystemThemeChanges(() => {
      if (theme === "auto") {
        updateEffectiveTheme("auto");
      }
    });
  }, [theme, updateEffectiveTheme]);

  return {
    theme,
    effectiveTheme,
    reducedStimulation,
    setTheme,
    setReducedStimulation,
  };
}

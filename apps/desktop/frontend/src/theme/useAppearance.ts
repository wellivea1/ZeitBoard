import { useCallback, useEffect, useState } from "react";
import {
  applyThemeAttribute,
  getStoredTheme,
  listenForSystemThemeChanges,
  storeTheme,
  type EffectiveTheme,
  type ThemePreference,
} from "./theme";
import {
  applyReducedStimulationAttribute,
  getStoredReducedStimulation,
  storeReducedStimulation,
} from "./reducedStimulation";
import {
  evaluateNightWindow,
  getStoredNightRule,
  loadAppearanceClock,
  storeNightRule,
  type AppearanceClock,
  type NightRule,
} from "./nightMode";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";

export interface AppearanceState {
  theme: ThemePreference;
  effectiveTheme: EffectiveTheme;
  reducedStimulation: boolean;
  setTheme: (preference: ThemePreference) => void;
  setReducedStimulation: (enabled: boolean) => void;
  nightRule: NightRule;
  setNightRule: (rule: NightRule) => void;
  nightActive: boolean;
  nightSource: "forecast" | "civil" | null;
  clockStatus: string;
}

export function useAppearance(): AppearanceState {
  const [theme, setThemeState] = useState<ThemePreference>(getStoredTheme);
  const [effectiveTheme, setEffectiveTheme] = useState<EffectiveTheme>(() => {
    if (theme !== "auto") return theme;
    if (typeof window === "undefined" || !window.matchMedia) return "light";
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  });
  const [reducedStimulation, setReducedStimulationState] = useState<boolean>(
    getStoredReducedStimulation,
  );
  const [nightRule, setNightRuleState] = useState<NightRule>(getStoredNightRule);
  const [clock, setClock] = useState<AppearanceClock>({ status: "unavailable" });
  const [now, setNow] = useState<Date>(() => new Date());

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

  const night = evaluateNightWindow(nightRule, clock, now);

  const setNightRule = useCallback((rule: NightRule) => {
    storeNightRule(rule);
    setNightRuleState(rule);
  }, []);

  // The night window overrides what the DOM shows without touching the
  // stored preference (ADR-0021: local, reversible display action).
  useEffect(() => {
    if (night.active) {
      applyThemeAttribute(nightRule.preset);
      return;
    }
    applyThemeAttribute(theme);
  }, [night.active, nightRule.preset, theme]);

  // Forecast times move with the user's data: refresh on data changes and a
  // slow tick so engagement happens without a reload.
  useEffect(() => {
    if (!nightRule.enabled) return;
    let current = true;
    const refresh = () =>
      void loadAppearanceClock().then((loaded) => {
        if (current) setClock(loaded);
      });
    refresh();
    window.addEventListener(sleepDataChangedEvent, refresh);
    const tick = window.setInterval(() => {
      setNow(new Date());
      refresh();
    }, 60_000);
    return () => {
      current = false;
      window.removeEventListener(sleepDataChangedEvent, refresh);
      window.clearInterval(tick);
    };
  }, [nightRule.enabled]);

  return {
    theme,
    effectiveTheme: night.active ? nightRule.preset : effectiveTheme,
    reducedStimulation,
    setTheme,
    setReducedStimulation,
    nightRule,
    setNightRule,
    nightActive: night.active,
    nightSource: night.source,
    clockStatus: clock.status,
  };
}

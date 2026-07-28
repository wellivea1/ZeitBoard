import { useCallback, useEffect, useRef, useState } from "react";
import {
  listenForAppearanceCommands,
  loadLocalAppearanceState,
  saveLocalAppearanceState,
  type LocalAppearanceState,
} from "../data/localAgent";
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
import { createCoalescedRefresh } from "../utils/coalescedRefresh";
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
  // The stored snapshot is read once and never replaced, so it is held as
  // lazily-initialized state rather than a ref: state is safe to read during
  // render and stable enough to sit in an effect dependency list.
  const [initialAppearance] = useState<LocalAppearanceState>(() => ({
    theme: getStoredTheme(),
    reducedStimulation: getStoredReducedStimulation(),
    nightRule: getStoredNightRule(),
  }));

  const [theme, setThemeState] = useState<ThemePreference>(initialAppearance.theme);
  const [effectiveTheme, setEffectiveTheme] = useState<EffectiveTheme>(() => {
    if (initialAppearance.theme !== "auto") return initialAppearance.theme;
    if (typeof window === "undefined" || !window.matchMedia) return "light";
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  });
  const [reducedStimulation, setReducedStimulationState] = useState<boolean>(
    initialAppearance.reducedStimulation,
  );
  const [nightRule, setNightRuleState] = useState<NightRule>(initialAppearance.nightRule);
  const [clock, setClock] = useState<AppearanceClock>({ status: "unavailable" });
  const [now, setNow] = useState<Date>(() => new Date());
  const desiredStateRef = useRef<LocalAppearanceState>(initialAppearance);
  const revisionRef = useRef(0);
  const editVersionRef = useRef(0);
  const desktopLoadedRef = useRef(false);
  const preloadPatchRef = useRef<Partial<LocalAppearanceState>>({});
  const saveRequestedRef = useRef(false);
  const saveLoopRunningRef = useRef(false);

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

  const applyState = useCallback(
    (state: LocalAppearanceState) => {
      desiredStateRef.current = state;
      storeTheme(state.theme);
      storeReducedStimulation(state.reducedStimulation);
      storeNightRule(state.nightRule);
      setThemeState(state.theme);
      setReducedStimulationState(state.reducedStimulation);
      setNightRuleState(state.nightRule);
      updateEffectiveTheme(state.theme);
      applyReducedStimulationAttribute(state.reducedStimulation);
    },
    [updateEffectiveTheme],
  );

  const flushDesktopSave = useCallback(() => {
    if (!desktopLoadedRef.current || saveLoopRunningRef.current) return;
    saveLoopRunningRef.current = true;
    void (async () => {
      try {
        while (desktopLoadedRef.current && saveRequestedRef.current) {
          saveRequestedRef.current = false;
          const state = desiredStateRef.current;
          const version = editVersionRef.current;
          const baseRevision = revisionRef.current;
          let envelope;
          try {
            envelope = await saveLocalAppearanceState(state, baseRevision);
          } catch {
            continue;
          }
          if (envelope.revision < revisionRef.current) continue;
          revisionRef.current = envelope.revision;
          if (editVersionRef.current === version) {
            applyState(envelope.state);
          }
        }
      } finally {
        saveLoopRunningRef.current = false;
      }
    })();
  }, [applyState]);

  const requestDesktopSave = useCallback(() => {
    if (!desktopLoadedRef.current) return;
    saveRequestedRef.current = true;
    flushDesktopSave();
  }, [flushDesktopSave]);

  const setTheme = useCallback(
    (preference: ThemePreference) => {
      editVersionRef.current += 1;
      if (!desktopLoadedRef.current) {
        preloadPatchRef.current = { ...preloadPatchRef.current, theme: preference };
      }
      applyState({ ...desiredStateRef.current, theme: preference });
      requestDesktopSave();
    },
    [applyState, requestDesktopSave],
  );

  const setReducedStimulation = useCallback(
    (enabled: boolean) => {
      editVersionRef.current += 1;
      if (!desktopLoadedRef.current) {
        preloadPatchRef.current = { ...preloadPatchRef.current, reducedStimulation: enabled };
      }
      applyState({ ...desiredStateRef.current, reducedStimulation: enabled });
      requestDesktopSave();
    },
    [applyState, requestDesktopSave],
  );

  const setNightRule = useCallback(
    (rule: NightRule) => {
      editVersionRef.current += 1;
      if (!desktopLoadedRef.current) {
        preloadPatchRef.current = { ...preloadPatchRef.current, nightRule: rule };
      }
      applyState({ ...desiredStateRef.current, nightRule: rule });
      requestDesktopSave();
    },
    [applyState, requestDesktopSave],
  );

  useEffect(() => {
    let current = true;
    const dispose = listenForAppearanceCommands((envelope) => {
      if (!current || envelope.revision <= revisionRef.current) return;
      revisionRef.current = envelope.revision;
      desktopLoadedRef.current = true;
      preloadPatchRef.current = {};
      saveRequestedRef.current = false;
      editVersionRef.current += 1;
      applyState(envelope.state);
    });

    const loadVersion = editVersionRef.current;
    void loadLocalAppearanceState(initialAppearance)
      .then((envelope) => {
        if (!current) return;
        if (desktopLoadedRef.current && envelope.revision <= revisionRef.current) return;
        revisionRef.current = envelope.revision;
        desktopLoadedRef.current = true;
        const pending = preloadPatchRef.current;
        preloadPatchRef.current = {};
        if (Object.keys(pending).length > 0) {
          applyState({ ...envelope.state, ...pending });
          requestDesktopSave();
          return;
        }
        if (editVersionRef.current === loadVersion) {
          editVersionRef.current += 1;
          applyState(envelope.state);
        }
      })
      .catch(() => {
        // Local storage remains usable when desktop persistence cannot be read.
      });

    return () => {
      current = false;
      dispose();
    };
  }, [applyState, initialAppearance, requestDesktopSave]);

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
    const refresh = createCoalescedRefresh(loadAppearanceClock, setClock);
    const request = () => refresh.request();
    const requestIfVisible = () => {
      if (document.visibilityState === "hidden") return;
      setNow(new Date());
      request();
    };
    const onVisibilityChange = () => requestIfVisible();
    request();
    window.addEventListener(sleepDataChangedEvent, request);
    document.addEventListener("visibilitychange", onVisibilityChange);
    const tick = window.setInterval(requestIfVisible, 60_000);
    return () => {
      window.removeEventListener(sleepDataChangedEvent, request);
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.clearInterval(tick);
      refresh.dispose();
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

import { createContext, useContext, type ReactNode } from "react";
import { useAppearance, type AppearanceState } from "./useAppearance";

const AppearanceContext = createContext<AppearanceState | null>(null);

export function AppearanceProvider({ children }: { children: ReactNode }) {
  const state = useAppearance();
  return <AppearanceContext.Provider value={state}>{children}</AppearanceContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAppearanceContext(): AppearanceState {
  const context = useContext(AppearanceContext);
  if (!context) {
    throw new Error("useAppearanceContext must be used within AppearanceProvider");
  }
  return context;
}

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import App from "./App";
import { AppearanceProvider } from "./theme/AppearanceProvider";
import { loadAndApplyReducedStimulation } from "./theme/reducedStimulation";
import { loadAndApplyTheme } from "./theme/theme";
import "./styles.css";

loadAndApplyTheme();
loadAndApplyReducedStimulation();

const root = document.getElementById("root");

if (!root) throw new Error("Application root is missing");

createRoot(root).render(
  <StrictMode>
    <AppearanceProvider>
      <App />
    </AppearanceProvider>
  </StrictMode>,
);

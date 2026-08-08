import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import App from "./App";
import { AppearanceProvider } from "./theme/AppearanceProvider";
import { loadAndApplyReducedStimulation } from "./theme/reducedStimulation";
import { loadAndApplyTheme } from "./theme/theme";
import "./styles.css";
import "./styles/outlook.css";
import "./styles/overview.css";
import "./styles/rhythm.css";
import "./styles/rhythm-markers.css";
import "./styles/settings.css";
import "./styles/data-sources.css";
import "./styles/sharing.css";
import "./styles/calendar.css";
import "./styles/medications.css";
import "./styles/medication-report.css";
import "./styles/medication-schedules.css";

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

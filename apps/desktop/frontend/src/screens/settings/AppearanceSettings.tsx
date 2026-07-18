import { useAppearanceContext } from "../../theme/AppearanceProvider";
import { themePresets, type ThemePreference } from "../../theme/theme";
import type { NightPreset } from "../../theme/nightMode";

const nightPresets: { id: NightPreset; label: string }[] = [
  { id: "amber", label: "Amber (glasses)" },
  { id: "black", label: "Pitch black" },
  { id: "dark", label: "Dark" },
];

export function AppearanceSettings() {
  const {
    theme,
    reducedStimulation,
    setTheme,
    setReducedStimulation,
    nightRule,
    setNightRule,
    nightActive,
    nightSource,
    clockStatus,
  } = useAppearanceContext();

  const nightStatus = !nightRule.enabled
    ? ""
    : nightActive
      ? nightSource === "forecast"
        ? "On now — engaged by your predicted sleep window."
        : "On now — using your fixed evening times (no current estimate)."
      : nightSource === "forecast"
        ? "Waiting for the lead window before your predicted sleep."
        : nightSource === "civil"
          ? "Waiting for your fixed evening time (no current estimate)."
          : clockStatus === "estimated"
            ? "Waiting for the next forecast."
            : "No estimate and no fixed times set — the rule is inactive until one exists.";

  return (
    <section className="settings-section">
      <div>
        <p className="section-kicker">Display</p>
        <h2>Time and appearance</h2>
      </div>
      <fieldset className="appearance-picker">
        <legend>Appearance preset</legend>
        <div className="appearance-options">
          {themePresets.map((preset) => (
            <label
              className="appearance-option"
              data-preset={preset.id}
              data-selected={theme === preset.id || undefined}
              key={preset.id}
            >
              <input
                type="radio"
                name="appearance"
                value={preset.id}
                checked={theme === preset.id}
                onChange={(event) => setTheme(event.target.value as ThemePreference)}
              />
              <span className="appearance-preview" aria-hidden="true">
                <i className="appearance-preview-rail" />
                <i className="appearance-preview-content">
                  <b />
                  <b />
                  <b />
                </i>
              </span>
              <span className="appearance-option-copy">
                <strong>{preset.label}</strong>
                <small>{preset.hint}</small>
              </span>
            </label>
          ))}
        </div>
      </fieldset>
      <label className="toggle-row settings-row">
        <span>
          <strong>Reduced stimulation</strong>
          <small>Soften motion, saturation, and contrast. Works in every appearance preset.</small>
        </span>
        <input
          type="checkbox"
          checked={reducedStimulation}
          onChange={(event) => setReducedStimulation(event.target.checked)}
        />
      </label>

      <label className="toggle-row settings-row">
        <span>
          <strong>Night appearance follows your rhythm</strong>
          <small>
            Switch to a night preset before your predicted sleep onset and back after wake. The
            trigger is the forecast, so it drifts with you; without an estimate it honestly falls
            back to fixed times. A local display setting — nothing goes through approvals.
          </small>
        </span>
        <input
          type="checkbox"
          checked={nightRule.enabled}
          onChange={(event) => setNightRule({ ...nightRule, enabled: event.target.checked })}
        />
      </label>
      {nightRule.enabled && (
        <div className="night-rule-fields">
          <label>
            Night preset
            <select
              value={nightRule.preset}
              onChange={(event) =>
                setNightRule({ ...nightRule, preset: event.target.value as NightPreset })
              }
            >
              {nightPresets.map((preset) => (
                <option value={preset.id} key={preset.id}>
                  {preset.label}
                </option>
              ))}
            </select>
          </label>
          <label>
            Hours before predicted sleep
            <input
              type="number"
              min={0}
              max={12}
              value={nightRule.leadHours}
              onChange={(event) =>
                setNightRule({ ...nightRule, leadHours: Number(event.target.value) })
              }
            />
          </label>
          <label>
            Fallback start (no estimate)
            <input
              type="time"
              value={nightRule.fallbackStartLocal}
              onChange={(event) =>
                setNightRule({ ...nightRule, fallbackStartLocal: event.target.value })
              }
            />
          </label>
          <label>
            Fallback end
            <input
              type="time"
              value={nightRule.fallbackEndLocal}
              onChange={(event) =>
                setNightRule({ ...nightRule, fallbackEndLocal: event.target.value })
              }
            />
          </label>
          <p className="settings-copy" role="status">
            {nightStatus}
          </p>
        </div>
      )}
    </section>
  );
}

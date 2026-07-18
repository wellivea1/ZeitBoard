import { useAppearanceContext } from "../../theme/AppearanceProvider";
import { themePresets, type ThemePreference } from "../../theme/theme";

export function AppearanceSettings() {
  const { theme, reducedStimulation, setTheme, setReducedStimulation } = useAppearanceContext();

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
    </section>
  );
}

import { useEffect, useState } from "react";
import {
  loadClockSetting,
  saveClockSetting,
  type ClockPreference,
  type ClockSetting,
} from "../../data/clock";
import { timeOfDay } from "../../utils/relativeTime";
import { setWindowClock, type Clock } from "../../utils/windowClock";

// Which clock times are written on (ADR-0057). Each choice shows the same
// afternoon on its own clock, so the difference is read, not described.

const sample = (clock: Clock) => timeOfDay(15, 40, clock);

interface Choice {
  preference: ClockPreference;
  title: string;
  hint: string;
}

const FIXED_CHOICES: Choice[] = [
  { preference: "12", title: "12-hour", hint: "With AM and PM." },
  { preference: "24", title: "24-hour", hint: "From 00:00 to 23:59." },
];

// Only Windows has a regional format ZeitBoard reads. Elsewhere this choice is
// the clock ZeitBoard writes until another is chosen, and says so.
function systemChoice({ platform, system }: ClockSetting): Choice {
  const windows = platform === "windows";
  return {
    preference: "system",
    title: windows ? "As Windows does" : "ZeitBoard's default",
    hint: windows
      ? `Follows the regional format, now the ${system}-hour clock.`
      : `The ${system}-hour clock, until you choose another.`,
  };
}

export function ClockSettings() {
  const [setting, setSetting] = useState<ClockSetting>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  useEffect(() => {
    let current = true;
    void loadClockSetting().then((loaded) => {
      if (!current) return;
      // A desktop that does not answer is not stood in for, as the browser's
      // clock would claim to be Windows' on any system.
      if (loaded) setSetting(loaded);
      else setError("The clock setting could not be read. The window keeps the clock it has.");
    });
    return () => {
      current = false;
    };
  }, []);

  const choose = async (preference: ClockPreference) => {
    if (!setting || busy || preference === setting.preference) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const saved = await saveClockSetting(preference, setting.revision);
      setSetting(saved);
      setWindowClock(saved.clock);
      if (saved.conflict) {
        setMessage("The clock was changed elsewhere, so that choice was kept and yours was not.");
      }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The clock could not be saved.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-section" aria-labelledby="clock-title">
      <div>
        <h2 id="clock-title">Clock</h2>
        <p className="settings-copy">
          How every time is written, in the window and in what ZeitBoard prepares for you. Records
          are kept the same either way.
        </p>
      </div>
      {setting && (
        <fieldset className="clock-picker" disabled={busy}>
          <legend className="sr-only">Clock</legend>
          <div className="clock-options">
            {[systemChoice(setting), ...FIXED_CHOICES].map((choice) => {
              const clock = choice.preference === "system" ? setting.system : choice.preference;
              const selected = setting.preference === choice.preference;
              return (
                <label
                  className="clock-option"
                  data-selected={selected || undefined}
                  key={choice.preference}
                >
                  <input
                    type="radio"
                    name="clock"
                    value={choice.preference}
                    checked={selected}
                    onChange={() => void choose(choice.preference)}
                  />
                  <span className="clock-option-sample" aria-hidden="true">
                    {sample(clock)}
                  </span>
                  <strong>{choice.title}</strong>
                  <small>{choice.hint}</small>
                </label>
              );
            })}
          </div>
        </fieldset>
      )}
      {message && (
        <p className="settings-copy" role="status">
          {message}
        </p>
      )}
      {(error || setting?.message) && (
        <p className="form-error" role="alert">
          {error || setting?.message}
        </p>
      )}
    </section>
  );
}

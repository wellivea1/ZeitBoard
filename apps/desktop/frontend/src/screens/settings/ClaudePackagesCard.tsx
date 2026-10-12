import { useState } from "react";
import {
  canSaveClaudePackages,
  saveClaudeExtension,
  saveClaudeSkill,
  type SavedPackage,
} from "../../data/claudePackages";

// Two files that add ZeitBoard to Claude (ADR-0061). Where the data goes once
// they are used is said here, before either is saved.

export function ClaudePackagesCard() {
  const available = canSaveClaudePackages();
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");

  const run = async (save: () => Promise<SavedPackage>, next: string) => {
    setBusy(true);
    setStatus("");
    setError("");
    try {
      const saved = await save();
      if (saved.saved) setStatus(`Saved ${saved.fileName}. ${next}`);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "It could not be saved.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="data-control-card" aria-labelledby="claude-packages-title">
      <div>
        <h3 id="claude-packages-title">Use ZeitBoard with Claude</h3>
        <p>
          Two files add ZeitBoard to Claude Desktop: an extension holding this computer&apos;s
          connector, and a skill that tells Claude how to use it. Neither holds your records or a
          password.
        </p>
      </div>
      <p className="settings-copy">
        Once they are added, what ZeitBoard&apos;s tools return goes to the model Claude uses, in
        Anthropic&apos;s cloud, under your Anthropic account. It is only what Data and action
        boundaries lists: no titles, notes or raw records.
      </p>
      <div className="claude-package-actions">
        <button
          type="button"
          className="button secondary"
          disabled={!available || busy}
          onClick={() =>
            void run(saveClaudeExtension, "Open it to add ZeitBoard to Claude Desktop.")
          }
        >
          Save the Claude Desktop extension
        </button>
        <button
          type="button"
          className="button secondary"
          disabled={!available || busy}
          onClick={() =>
            void run(
              saveClaudeSkill,
              "Add it to your skills in Claude's settings, or unpack it into Claude Code's skills folder.",
            )
          }
        >
          Save the skill
        </button>
      </div>
      {!available && <p>Saving them needs the ZeitBoard desktop app.</p>}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {status && (
        <p className="form-status" role="status">
          {status}
        </p>
      )}
    </section>
  );
}

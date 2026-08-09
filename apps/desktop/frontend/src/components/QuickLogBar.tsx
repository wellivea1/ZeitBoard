import { useEffect, useState } from "react";
import { Icon } from "./Icon";
import {
  beginQuickSleep,
  completeQuickSleep,
  confirmQuickSleep,
  discardQuickSleep,
  loadQuickLogState,
  quickLogUnavailable,
  type QuickLogResult,
  type QuickLogState,
} from "../data/quickLog";

// Two taps instead of a four-field form. The app records the night when the
// pair is plausible and asks one question when it is not — it never fills in a
// boundary nobody reported.

// The fields are controlled rather than defaulted. React reuses the same input
// element when one question replaces another, and an uncontrolled field keeps
// whatever was typed into the previous one — which would put a time from a
// different question in front of someone about to record it as fact.
interface Question {
  reason: string;
  startLocal: string;
  endLocal: string;
  isPrediction: boolean;
  offerNap: boolean;
  nap: boolean;
}

function questionFrom(result: QuickLogResult): Question | null {
  if (!result.outcome.startsWith("confirm_")) return null;
  return {
    reason: result.reason,
    startLocal: result.suggestedStartLocal ?? "",
    endLocal: result.suggestedEndLocal ?? "",
    isPrediction: result.suggestionIsPrediction,
    offerNap: result.outcome === "confirm_short",
    nap: false,
  };
}

export function QuickLogBar() {
  const [state, setState] = useState<QuickLogState>(quickLogUnavailable);
  const [question, setQuestion] = useState<Question | null>(null);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let current = true;
    void loadQuickLogState().then((loaded) => {
      if (current) setState(loaded);
    });
    return () => {
      current = false;
    };
  }, []);

  const run = async (action: () => Promise<QuickLogResult>) => {
    setBusy(true);
    setError("");
    try {
      const result = await action();
      setState(result.state);
      setQuestion(questionFrom(result));
      setNotice(result.outcome.startsWith("confirm_") ? "" : result.reason);
      if (result.outcome === "reject") setError(result.reason);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "That did not work.");
    } finally {
      setBusy(false);
    }
  };

  if (state.status !== "ok") return null;

  return (
    <section className="quick-log" aria-label="Log sleep">
      <div className="quick-log-actions">
        <button
          className="button secondary"
          type="button"
          disabled={busy}
          onClick={() => void run(beginQuickSleep)}
        >
          <Icon name="moon" />
          {state.pending ? "Going to sleep (again)" : "I am going to sleep"}
        </button>
        <button
          className="button primary"
          type="button"
          disabled={busy}
          onClick={() => void run(completeQuickSleep)}
        >
          <Icon name="focus" />I woke up
        </button>
        {state.pending && (
          <button
            className="button secondary compact"
            type="button"
            disabled={busy}
            onClick={() => void run(discardQuickSleep)}
          >
            Discard
          </button>
        )}
      </div>

      {state.pending && (
        <p className="quick-log-pending" data-stale={state.pendingStale || undefined}>
          <strong>{state.pendingLabel}</strong>
          <small>
            {state.pendingStale
              ? "That was a while ago, so tapping “I woke up” will ask for the real times rather than assume now."
              : "Tap “I woke up” when you get up and the night is recorded."}
          </small>
        </p>
      )}

      {/* The pending block is the state readout, so a notice repeating it would
          say the same thing twice. What is left to report is what happened
          instead: recorded, or discarded. */}
      {notice && !question && !state.pending && (
        <p className="quick-log-notice" role="status">
          {notice}
        </p>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}

      {question && (
        <form
          className="quick-log-question"
          aria-label="Confirm the sleep times"
          onSubmit={(event) => {
            event.preventDefault();
            void run(() =>
              confirmQuickSleep({
                startLocal: question.startLocal,
                endLocal: question.endLocal,
                classification: question.nap ? "nap" : "principal",
              }),
            );
          }}
        >
          <p>{question.reason}</p>
          <label>
            <span>
              Fell asleep
              {question.isPrediction && (
                /* Never let a forecast pass for a record. */
                <em className="quick-log-predicted"> (predicted — check this)</em>
              )}
            </span>
            <input
              name="startLocal"
              type="datetime-local"
              value={question.startLocal}
              required
              onChange={(event) =>
                setQuestion({ ...question, startLocal: event.target.value, isPrediction: false })
              }
            />
          </label>
          <label>
            <span>Woke up</span>
            <input
              name="endLocal"
              type="datetime-local"
              value={question.endLocal}
              required
              onChange={(event) => setQuestion({ ...question, endLocal: event.target.value })}
            />
          </label>
          {question.offerNap && (
            <label className="quick-log-nap">
              <input
                name="nap"
                type="checkbox"
                checked={question.nap}
                onChange={(event) => setQuestion({ ...question, nap: event.target.checked })}
              />
              <span>This was a nap</span>
            </label>
          )}
          <div className="quick-log-question-actions">
            <button className="button primary compact" type="submit" disabled={busy}>
              Record it
            </button>
            <button
              className="button secondary compact"
              type="button"
              disabled={busy}
              onClick={() => setQuestion(null)}
            >
              Not now
            </button>
          </div>
        </form>
      )}
    </section>
  );
}

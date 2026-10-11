import { calendarDataChangedEvent } from "../data/calendar";
import { loadSetupGuide, setupStepWords, type SetupStep } from "../data/setupGuide";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import { useLoaded } from "../state/useLoaded";
import { Icon } from "./Icon";
import { Notice } from "./Notice";

// Home after the first forecast: the optional steps that let it say more, on
// a note that closes for good. A done step keeps its place, marked and without
// its explanation, so the note reads as progress and what is left stands out.
// The note goes by itself once every step is done.

function SetupStepEntry({ step, index }: { step: SetupStep; index: number }) {
  const words = setupStepWords[step.id];
  return (
    <li className="home-way" data-done={step.done || undefined}>
      <span className="home-way-number" aria-hidden="true">
        {step.done ? <Icon name="check" /> : index + 1}
      </span>
      <h3>
        {words.title}
        {step.done && <span className="sr-only">, done</span>}
      </h3>
      {!step.done && <p>{words.body}</p>}
      <a className="home-more" href={words.link.href}>
        {step.done ? "Change" : words.link.label}
      </a>
    </li>
  );
}

export function HomeSetupGuide() {
  const { data: steps } = useLoaded(loadSetupGuide, {
    events: [sleepDataChangedEvent, calendarDataChangedEvent],
    fallback: () => [],
  });
  if (!steps || steps.length === 0 || steps.every((step) => step.done)) return null;
  const done = steps.filter((step) => step.done).length;
  return (
    <Notice id="home.setup" className="home-setup-guide">
      <p>
        <span className="count">{done}</span> of {steps.length} done. Each is optional, and each
        lets Home say more.
      </p>
      <ol className="home-ways">
        {steps.map((step, index) => (
          <SetupStepEntry key={step.id} step={step} index={index} />
        ))}
      </ol>
    </Notice>
  );
}

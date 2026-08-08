import type { KeyboardEvent } from "react";

// The tablist behind Plan, Log and Rhythm (UI slice U-H).
//
// It exists because those three screens each hold several related views, and
// three hand-rolled tablists would drift: the keyboard handling here is the
// part people forget, and the part somebody navigating by keyboard because
// they are too tired to aim a mouse depends on.

export interface ScreenTab<Id extends string> {
  id: Id;
  label: string;

  /** Rendered as a count beside the label. Omitted when zero or absent. */
  badge?: number;
}

interface ScreenTabsProps<Id extends string> {
  /** Prefix for the generated tab and panel ids, e.g. "plan". */
  name: string;
  label: string;
  tabs: readonly ScreenTab<Id>[];
  active: Id;
  onSelect: (id: Id) => void;
}

export function tabId(name: string, id: string) {
  return `${name}-tab-${id}`;
}

export function panelId(name: string, id: string) {
  return `${name}-panel-${id}`;
}

export function ScreenTabs<Id extends string>({
  name,
  label,
  tabs,
  active,
  onSelect,
}: ScreenTabsProps<Id>) {
  // Roving tabindex: one stop for the whole group, arrows to move within it.
  // Tabbing through every view of a screen to reach its content is exactly the
  // kind of small tax that adds up when concentration is the scarce resource.
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const index = tabs.findIndex((item) => item.id === active);
    let nextIndex: number;
    if (event.key === "ArrowRight") nextIndex = (index + 1) % tabs.length;
    else if (event.key === "ArrowLeft") nextIndex = (index - 1 + tabs.length) % tabs.length;
    else if (event.key === "Home") nextIndex = 0;
    else if (event.key === "End") nextIndex = tabs.length - 1;
    else return;
    event.preventDefault();
    const next = tabs[nextIndex];
    if (next) onSelect(next.id);
  };

  return (
    <div className="screen-tabs" role="tablist" aria-label={label} onKeyDown={onKeyDown}>
      {tabs.map((item) => (
        <button
          key={item.id}
          className={`filter${active === item.id ? " active" : ""}`}
          type="button"
          role="tab"
          id={tabId(name, item.id)}
          aria-selected={active === item.id}
          aria-controls={panelId(name, item.id)}
          tabIndex={active === item.id ? 0 : -1}
          onClick={() => onSelect(item.id)}
        >
          {item.label}
          {item.badge ? (
            <span className="screen-tab-badge" aria-label={`${item.badge} pending`}>
              {item.badge}
            </span>
          ) : null}
        </button>
      ))}
    </div>
  );
}

export function ScreenTabPanel<Id extends string>({
  name,
  id,
  active,
  children,
}: {
  name: string;
  id: Id;
  active: Id;
  children: React.ReactNode;
}) {
  if (id !== active) return null;
  return (
    <div
      className="screen-tab-panel"
      role="tabpanel"
      id={panelId(name, id)}
      aria-labelledby={tabId(name, id)}
    >
      {children}
    </div>
  );
}

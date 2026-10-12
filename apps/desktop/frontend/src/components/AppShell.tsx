import { lazy, Suspense, useEffect, useState, type ReactNode } from "react";
import { primaryNavigation, utilityNavigation, type NavItem } from "../navigation";
import { usePendingApprovalsCount } from "../state/approvalQueue";
import type { ScreenId } from "../types";
import { appLocale } from "../utils/appLocale";

const AssistantRail = lazy(() =>
  import("./AssistantRail").then((module) => ({ default: module.AssistantRail })),
);

function NavigationLink({ item, active }: { item: NavItem; active: boolean }) {
  return (
    <a
      className="nav-link"
      data-active={active}
      href={`#/${item.id}`}
      aria-label={item.badge ? `${item.label}, ${item.badge} pending` : item.label}
      aria-current={active ? "page" : undefined}
    >
      {item.label}
      {item.badge && (
        <span className="nav-badge" aria-hidden="true">
          {item.badge}
        </span>
      )}
    </a>
  );
}

function useToday() {
  const [today, setToday] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setToday(new Date()), 60_000);
    return () => window.clearInterval(timer);
  }, []);
  return today.toLocaleDateString(appLocale, { weekday: "long", day: "numeric", month: "long" });
}

export function AppShell({ screen, children }: { screen: ScreenId; children: ReactNode }) {
  const pendingCount = usePendingApprovalsCount();
  const today = useToday();
  const [assistantOpen, setAssistantOpen] = useState(false);
  const [assistantLoaded, setAssistantLoaded] = useState(false);

  const toggleAssistant = () => {
    if (!assistantOpen) setAssistantLoaded(true);
    setAssistantOpen((open) => !open);
  };

  return (
    <div className="app-shell" data-assistant-open={assistantOpen || undefined}>
      {/* A masthead over one sheet: the name, the day, and the destinations in
          small capitals. The sidebar it replaces spent a fixed column on seven
          links and a privacy blurb. */}
      <header className="masthead">
        <a className="wordmark" href="#/home" aria-label="ZeitBoard home">
          ZeitBoard
        </a>
        <span className="masthead-date">{today}</span>
        <nav className="primary-nav" aria-label="Primary navigation">
          {primaryNavigation.map((item) => (
            <NavigationLink
              key={item.id}
              item={
                item.id === "plan" && pendingCount > 0
                  ? { ...item, badge: String(pendingCount) }
                  : item
              }
              active={screen === item.id}
            />
          ))}
        </nav>
        <nav className="utility-nav" aria-label="Settings and sources">
          {utilityNavigation.map((item) => (
            <NavigationLink key={item.id} item={item} active={screen === item.id} />
          ))}
          <button
            className="assistant-toggle"
            type="button"
            data-active={assistantOpen || undefined}
            aria-pressed={assistantOpen}
            onClick={toggleAssistant}
          >
            Assistant
          </button>
        </nav>
      </header>

      <main className="main-content" id="main-content" tabIndex={-1}>
        {children}
      </main>

      {assistantLoaded && (
        <Suspense
          fallback={
            assistantOpen ? (
              <div className="assistant-rail" role="status">
                Opening the assistant…
              </div>
            ) : null
          }
        >
          <AssistantRail open={assistantOpen} onClose={() => setAssistantOpen(false)} />
        </Suspense>
      )}
    </div>
  );
}

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  level = "page",
}: {
  eyebrow?: string;
  title: string;
  /**
   * Optional. Most screens are clear from their title; a sentence restating
   * the title on every page was part of what made the app read as prose.
   */
  description?: string;
  actions?: ReactNode;

  /**
   * `panel` is for a screen rendered inside a tab (slice U-H). The tab already
   * names the view and labels the panel, so repeating the title as a heading
   * would be noise — but the description and the actions are not noise, and
   * dropping the header entirely would have taken a calendar's date controls
   * with it.
   */
  level?: "page" | "panel";
}) {
  if (level === "panel") {
    return (
      <header className="panel-header">
        {description && <p>{description}</p>}
        {actions && <div className="page-actions">{actions}</div>}
      </header>
    );
  }
  return (
    <header className="page-header">
      <div>
        {eyebrow && <p className="eyebrow">{eyebrow}</p>}
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className="page-actions">{actions}</div>}
    </header>
  );
}

export function PlaceholderNotice({ children }: { children: ReactNode }) {
  return <p className="placeholder-notice">{children}</p>;
}

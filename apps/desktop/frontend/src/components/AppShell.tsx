import { useEffect, useState, type ReactNode } from "react";
import { Icon, type IconName } from "./Icon";
import type { ScreenId } from "../types";

interface NavItem {
  id: ScreenId;
  label: string;
  icon: IconName;
}

const primaryNavigation: NavItem[] = [
  { id: "overview", label: "Overview", icon: "overview" },
  { id: "calendar", label: "Calendar", icon: "calendar" },
  { id: "tasks", label: "Tasks", icon: "tasks" },
  { id: "timeline", label: "Timeline", icon: "timeline" },
  { id: "medications", label: "Medications", icon: "medications" },
  { id: "sharing", label: "Sharing", icon: "sharing" },
  { id: "data-sources", label: "Data Sources", icon: "sources" },
];

const screenIds = new Set<ScreenId>([...primaryNavigation.map((item) => item.id), "settings"]);

function readScreenFromHash(): ScreenId {
  const candidate = window.location.hash.replace(/^#\/?/, "") as ScreenId;
  return screenIds.has(candidate) ? candidate : "overview";
}

// eslint-disable-next-line react-refresh/only-export-components
export function useScreenNavigation() {
  const [screen, setScreen] = useState<ScreenId>(readScreenFromHash);

  useEffect(() => {
    const onHashChange = () => setScreen(readScreenFromHash());
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  return screen;
}

function NavigationLink({ item, active }: { item: NavItem; active: boolean }) {
  return (
    <a
      className="nav-link"
      data-active={active}
      href={`#/${item.id}`}
      aria-current={active ? "page" : undefined}
    >
      <Icon name={item.icon} />
      <span>{item.label}</span>
    </a>
  );
}

export function AppShell({ screen, children }: { screen: ScreenId; children: ReactNode }) {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="#/overview" aria-label="Non-24 Planner overview">
          <span className="brand-mark" aria-hidden="true">
            <span />
          </span>
          <span>
            <strong>Non-24</strong>
            <small>Planner</small>
          </span>
        </a>

        <nav className="primary-nav" aria-label="Primary navigation">
          {primaryNavigation.map((item) => (
            <NavigationLink key={item.id} item={item} active={screen === item.id} />
          ))}
        </nav>

        <div className="sidebar-footer">
          <NavigationLink
            item={{ id: "settings", label: "Settings", icon: "settings" }}
            active={screen === "settings"}
          />
          <div className="privacy-note">
            <Icon name="shield" />
            <span>
              <strong>Local-first</strong>
              <small>Your data stays on this device.</small>
            </span>
          </div>
        </div>
      </aside>

      <main className="main-content" id="main-content">
        {children}
      </main>
    </div>
  );
}

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
}: {
  eyebrow?: string;
  title: string;
  description: string;
  actions?: ReactNode;
}) {
  return (
    <header className="page-header">
      <div>
        {eyebrow && <p className="eyebrow">{eyebrow}</p>}
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {actions && <div className="page-actions">{actions}</div>}
    </header>
  );
}

export function PlaceholderNotice({ children }: { children: ReactNode }) {
  return <p className="placeholder-notice">{children}</p>;
}

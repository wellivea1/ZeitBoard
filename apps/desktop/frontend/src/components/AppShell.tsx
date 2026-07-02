import { useEffect, useState, type ReactNode } from "react";
import { Icon, type IconName } from "./Icon";
import { useApprovals } from "../state/approvals";
import type { ScreenId } from "../types";

interface NavItem {
  id: ScreenId;
  label: string;
  icon: IconName;
  badge?: string;
}

const primaryNavigation: NavItem[] = [
  { id: "overview", label: "Overview", icon: "overview" },
  { id: "calendar", label: "Calendar", icon: "calendar" },
  { id: "tasks", label: "Tasks", icon: "tasks" },
  { id: "approvals", label: "Approvals", icon: "approvals" },
  { id: "rhythm", label: "Rhythm", icon: "timeline" },
  { id: "medications", label: "Medications", icon: "medications" },
  { id: "sharing", label: "Sharing", icon: "sharing" },
  { id: "data-sources", label: "Data Sources", icon: "sources" },
];

const screenIds = new Set<ScreenId>([...primaryNavigation.map((item) => item.id), "settings"]);

function readScreenFromHash(): ScreenId {
  const candidate = window.location.hash.replace(/^#\/?/, "");
  if (candidate === "timeline") return "rhythm";
  return screenIds.has(candidate as ScreenId) ? (candidate as ScreenId) : "overview";
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
      {item.badge && (
        <span className="nav-badge" aria-label={`${item.badge} pending`}>
          {item.badge}
        </span>
      )}
    </a>
  );
}

export function AppShell({ screen, children }: { screen: ScreenId; children: ReactNode }) {
  const { pendingCount } = useApprovals();
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="#/overview" aria-label="ZeitBoard overview">
          <span className="brand-mark" aria-hidden="true">
            <span />
          </span>
          <span>
            <strong>ZeitBoard</strong>
            <small>Planner</small>
          </span>
        </a>

        <nav className="primary-nav" aria-label="Primary navigation">
          {primaryNavigation.map((item) => (
            <NavigationLink
              key={item.id}
              item={
                item.id === "approvals" && pendingCount > 0
                  ? { ...item, badge: String(pendingCount) }
                  : item
              }
              active={screen === item.id}
            />
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
              <strong>Private by design</strong>
              <small>Data stays local; syncs only to your own server if you turn sync on.</small>
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

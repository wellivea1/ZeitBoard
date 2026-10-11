import type { ReactNode } from "react";
import { noticeCatalog, useNotice, type NoticeId } from "../state/notices";
import { Icon } from "./Icon";

// An explanation set apart from the page on a sheet of its own, with an × to
// close it for good. Closed notes are listed in Settings › Display.

export function Notice({
  id,
  tone = "info",
  className,
  children,
}: {
  id: NoticeId;
  tone?: "info" | "caution";
  /** Places the note on its page; the sheet itself looks the same everywhere. */
  className?: string;
  children: ReactNode;
}) {
  const { hidden, hide } = useNotice(id);
  if (hidden) return null;
  const { title } = noticeCatalog[id];
  return (
    <aside
      className={className ? `notice ${className}` : "notice"}
      data-tone={tone}
      aria-label={title}
    >
      <p className="notice-title">{title}</p>
      <div className="notice-body">{children}</div>
      <button
        className="notice-hide"
        type="button"
        aria-label={`Hide the note “${title}”`}
        title="Hide this note. Settings › Display lists hidden notes."
        onClick={hide}
      >
        <Icon name="close" />
      </button>
    </aside>
  );
}

import type { ReactNode, SVGProps } from "react";

export type IconName =
  | "overview"
  | "calendar"
  | "tasks"
  | "approvals"
  | "timeline"
  | "medications"
  | "sharing"
  | "sources"
  | "settings"
  | "moon"
  | "clock"
  | "trend"
  | "focus"
  | "shield"
  | "chevron";

const paths: Record<IconName, ReactNode> = {
  overview: (
    <>
      <rect x="3" y="3" width="7" height="7" rx="2" />
      <rect x="14" y="3" width="7" height="7" rx="2" />
      <rect x="3" y="14" width="7" height="7" rx="2" />
      <rect x="14" y="14" width="7" height="7" rx="2" />
    </>
  ),
  calendar: (
    <>
      <path d="M6 2v4M18 2v4M3 9h18" />
      <rect x="3" y="4" width="18" height="17" rx="3" />
      <path d="M8 13h3M14 13h2M8 17h2" />
    </>
  ),
  tasks: (
    <>
      <path d="m4 7 2 2 4-4M4 15l2 2 4-4M13 7h7M13 15h7" />
    </>
  ),
  approvals: (
    <>
      <rect x="4" y="4" width="16" height="16" rx="3" />
      <path d="m8 12 3 3 5-6" />
    </>
  ),
  timeline: (
    <>
      <path d="M4 19V5M4 12h5l2-5 3 10 2-5h4" />
    </>
  ),
  medications: (
    <>
      <path
        d="m8 16 8-8a4.2 4.2 0 0 1 6 6l-8 8a4.2 4.2 0 0 1-6-6Z"
        transform="scale(.75) translate(2 2)"
      />
      <path d="m9 10 5 5" />
    </>
  ),
  sharing: (
    <>
      <circle cx="18" cy="5" r="3" />
      <circle cx="6" cy="12" r="3" />
      <circle cx="18" cy="19" r="3" />
      <path d="m8.6 10.5 6.8-4M8.6 13.5l6.8 4" />
    </>
  ),
  sources: (
    <>
      <ellipse cx="12" cy="5" rx="8" ry="3" />
      <path d="M4 5v7c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 12v7c0 1.7 3.6 3 8 3s8-1.3 8-3v-7" />
    </>
  ),
  settings: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4V21a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9A1.7 1.7 0 0 0 3 14H2.8v-4H3a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1a1.7 1.7 0 0 0 1.9.3A1.7 1.7 0 0 0 10 3V2.8h4V3a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1Z" />
    </>
  ),
  moon: <path d="M20.5 15.3A9 9 0 0 1 8.7 3.5 9 9 0 1 0 20.5 15.3Z" />,
  clock: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  trend: (
    <>
      <path d="M3 17 9 11l4 4 8-9" />
      <path d="M16 6h5v5" />
    </>
  ),
  focus: (
    <>
      <path d="M8 3H5a2 2 0 0 0-2 2v3M16 3h3a2 2 0 0 1 2 2v3M8 21H5a2 2 0 0 1-2-2v-3M16 21h3a2 2 0 0 0 2-2v-3" />
      <circle cx="12" cy="12" r="3" />
    </>
  ),
  shield: <path d="M12 3 20 6v5c0 5.2-3.4 8.6-8 10-4.6-1.4-8-4.8-8-10V6l8-3Z" />,
  chevron: <path d="m9 18 6-6-6-6" />,
};

export function Icon({ name, ...props }: SVGProps<SVGSVGElement> & { name: IconName }) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      {paths[name]}
    </svg>
  );
}

import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ShareLinksData } from "../data/sharing";
import { previewNote, sharingLine } from "../data/homeSharing";
import { HomeSharing } from "./HomeSharing";

const current = {
  state: "current" as const,
  reason: "",
  explanation: "Based on recent records.",
  ageLabel: "Newest record 2 hours ago",
  trusted: true,
};
const withheld = {
  state: "withheld" as const,
  reason: "evidence_stale",
  explanation: "The newest record is too old.",
  ageLabel: "Newest record 3 days ago",
  trusted: false,
};

function links(...states: string[]): ShareLinksData {
  return {
    status: "ok",
    disclosure: "",
    minPasscodeLength: 8,
    maxDays: 90,
    links: states.map((state, index) => ({
      profileId: `profile_${index}`,
      label: `Synthetic ${index}`,
      state,
      stateLabel: state,
      createdLabel: "",
      expiresLabel: "",
      grants: {} as ShareLinksData["links"][number]["grants"],
      grantSummary: "",
      access: [],
    })),
  };
}

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("what Home says about sharing", () => {
  it("says nothing when no link is working or sharing is not set up", () => {
    expect(sharingLine(links(), current)).toBeUndefined();
    expect(sharingLine(links("revoked", "expired"), current)).toBeUndefined();
    expect(sharingLine({ ...links("active"), status: "off" }, current)).toBeUndefined();
  });

  it("counts the working links, and says when they show no times", () => {
    expect(sharingLine(links("active", "revoked"), current)).toEqual({
      working: "One share link is working",
    });
    const stale = sharingLine(links("active", "active"), withheld);
    expect(stale?.working).toBe("2 share links are working");
    expect(stale?.note).toMatch(/^They show no times right now, because your newest sleep record/);
    expect(sharingLine(links("active"), withheld)?.note).toMatch(/^It shows no times right now/);
  });

  it("tells the owner why a link's reader sees no times, and that the link is fine", () => {
    expect(previewNote("Sam (synthetic)", current)).toBeUndefined();
    expect(previewNote("Sam (synthetic)", undefined)).toBeUndefined();
    const note = previewNote("Sam (synthetic)", withheld);
    expect(note).toContain("Sam (synthetic) sees no times right now");
    expect(note).toContain("(newest record 3 days ago)");
    expect(note).toContain("The link itself is fine.");
  });

  it("links to Sharing from the count, and draws nothing in the read-only preview", async () => {
    const { container } = render(<HomeSharing freshness={current} />);
    // No desktop, so no links to count.
    await vi.waitFor(() => expect(container).toBeEmptyDOMElement());

    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetBackendShareLinks: async () => ({
            status: "ok",
            disclosure: "",
            minPasscodeLength: 8,
            maxDays: 90,
            links: [],
          }),
        },
      },
    };
    render(<HomeSharing freshness={current} />);
    await vi.waitFor(() => expect(screen.queryByRole("link")).toBeNull());
  });
});

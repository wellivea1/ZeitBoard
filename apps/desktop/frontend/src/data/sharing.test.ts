import { describe, expect, it } from "vitest";
import {
  loadShareLinks,
  normalizeCreatedShareLink,
  normalizeShareLinks,
  shareLinksUnavailable,
} from "./sharing";

const link = {
  profileId: "prof_abc",
  label: "Mum",
  state: "active",
  stateLabel: "Working now",
  createdLabel: "Created Aug 1, 2026",
  expiresLabel: "Expires Sep 1, 2026",
  grants: { wakingWindows: true, allowRequests: false, allowMessages: false },
  grantSummary: "Shows when you are likely awake",
  access: [{ event: "availability_read", label: "Opened the page", count: 2 }],
};

function payload(overrides: Record<string, unknown> = {}) {
  return {
    status: "ok",
    disclosure: "Anyone with this link and passcode can see broad windows.",
    links: [link],
    minPasscodeLength: 6,
    maxDays: 90,
    ...overrides,
  };
}

describe("normalizeShareLinks", () => {
  it("accepts a complete list", () => {
    const data = normalizeShareLinks(payload());
    expect(data?.status).toBe("ok");
    expect(data?.links[0]?.profileId).toBe("prof_abc");
    expect(data?.links[0]?.access[0]?.count).toBe(2);
  });

  // `off` and `unavailable` are different problems with different fixes, so an
  // unrecognised status must not be quietly rendered as one of them.
  it("rejects a status outside the closed set", () => {
    expect(normalizeShareLinks(payload({ status: "maybe" }))).toBeUndefined();
  });

  it("rejects a link that is missing its identity", () => {
    expect(
      normalizeShareLinks(payload({ links: [{ ...link, profileId: undefined }] })),
    ).toBeUndefined();
  });

  // A grant flag that fails to parse must read as *not granted*. The opposite
  // default would show more than the owner chose.
  it("treats an unreadable grant as withheld", () => {
    const data = normalizeShareLinks(
      payload({ links: [{ ...link, grants: { wakingWindows: "yes" } }] }),
    );
    expect(data?.links[0]?.grants).toEqual({
      wakingWindows: false,
      allowRequests: false,
      allowMessages: false,
    });
  });
});

describe("normalizeCreatedShareLink", () => {
  it("keeps the address and the list it came back with", () => {
    const created = normalizeCreatedShareLink({
      status: "ok",
      linkUrl: "https://share.example.test/p/token",
      expiresLabel: "Expires Sep 1, 2026",
      links: payload(),
    });
    expect(created?.linkUrl).toBe("https://share.example.test/p/token");
    expect(created?.links.links).toHaveLength(1);
  });

  it("rejects a result whose link list did not parse", () => {
    expect(
      normalizeCreatedShareLink({ status: "ok", links: { status: "nonsense" } }),
    ).toBeUndefined();
  });
});

describe("loadShareLinks", () => {
  it("reports off rather than empty when the bridge is absent", async () => {
    await expect(loadShareLinks({})).resolves.toEqual(shareLinksUnavailable);
  });

  it("reads a valid payload", async () => {
    const root = { go: { main: { App: { GetBackendShareLinks: async () => payload() } } } };
    const data = await loadShareLinks(root);
    expect(data.status).toBe("ok");
    expect(data.disclosure).toContain("passcode");
  });

  // A half-understood list is treated as no list: a sharing screen that renders
  // the fields that happened to parse could show a revoked link as working.
  it("falls back when the payload is only partly understood", async () => {
    const root = {
      go: { main: { App: { GetBackendShareLinks: async () => ({ status: "ok" }) } } },
    };
    await expect(loadShareLinks(root)).resolves.toEqual(shareLinksUnavailable);
  });
});

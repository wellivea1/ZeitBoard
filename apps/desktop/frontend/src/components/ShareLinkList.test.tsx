import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ShareLinksData, ShareLinksStatus } from "../data/sharing";
import { ShareLinkList } from "./ShareLinkList";

function data(status: ShareLinksStatus): ShareLinksData {
  return { status, disclosure: "", links: [], minPasscodeLength: 8, maxDays: 30 };
}

describe("ShareLinkList", () => {
  // Found walking the app: with sync set up and the server's machine off, the
  // list said links would appear "once sync is set up".
  it("says why the links are not shown, for each reason", () => {
    const cases: [ShareLinksStatus, string][] = [
      ["off", "Links you make appear here once sync is set up."],
      ["unavailable", "Links you make appear here once your server's portal is on."],
      ["error", "Links you have made appear here when ZeitBoard's server answers."],
    ];
    for (const [status, sentence] of cases) {
      const { unmount } = render(
        <ShareLinkList data={data(status)} busy={false} onRevoke={vi.fn()} onErase={vi.fn()} />,
      );
      expect(screen.getByText(sentence)).toBeVisible();
      unmount();
    }
  });
});

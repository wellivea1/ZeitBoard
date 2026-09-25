import { describe, expect, it } from "vitest";
import { blankSleepLogHTML } from "./blankSleepLog";

describe("the blank sleep log", () => {
  const page = new DOMParser().parseFromString(blankSleepLogHTML(), "text/html");

  it("is a month of days from 6 PM to 6 PM, like the clinician chart", () => {
    const hours = [...page.querySelectorAll("thead th")].map((cell) => cell.textContent);
    expect(hours[0]).toBe("Date");
    expect(hours.slice(1, 25)).toEqual([
      "6p",
      "7p",
      "8p",
      "9p",
      "10p",
      "11p",
      "12a",
      "1a",
      "2a",
      "3a",
      "4a",
      "5a",
      "6a",
      "7a",
      "8a",
      "9a",
      "10a",
      "11a",
      "12p",
      "1p",
      "2p",
      "3p",
      "4p",
      "5p",
    ]);
    expect(hours.at(-1)).toBe("Notes");

    const rows = page.querySelectorAll("tbody tr");
    expect(rows).toHaveLength(31);
    for (const row of rows) expect(row.querySelectorAll("td")).toHaveLength(25);
    // Midnight and noon are drawn heavier, to find one's place on paper.
    expect(
      page.querySelectorAll("tbody tr:first-child .midnight, tbody tr:first-child .noon"),
    ).toHaveLength(2);
  });

  it("runs nothing, fetches nothing and asks for nothing personal", () => {
    const html = blankSleepLogHTML();
    expect(page.querySelector("script")).toBeNull();
    expect(
      page.querySelector('meta[http-equiv="Content-Security-Policy"]')?.getAttribute("content"),
    ).toBe("default-src 'none'; style-src 'unsafe-inline'");
    expect(html).not.toMatch(/https?:/);
    expect(page.querySelector(".fields")?.textContent).toBe("Month Time zone");
    expect(page.querySelector("footer")?.textContent).toMatch(/does not read this page/);
  });
});

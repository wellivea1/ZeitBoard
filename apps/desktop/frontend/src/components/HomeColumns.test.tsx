import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { loadMedications, type MedicationsData } from "../data/medications";
import { Diary, Doses } from "./HomeColumns";

// Home's diary and doses said there was nothing while they were still being
// read, and went on saying it when the read failed. Beside them, "Waiting on
// you" already said "Checking…" until its sources answered.
describe("Home's columns before and after a read", () => {
  it("says whether the diary is being read, could not be, or is empty", () => {
    const { rerender } = render(<Diary days={undefined} />);
    const diary = screen.getByRole("region", { name: "In the diary" });
    expect(diary).toHaveTextContent("Checking…");
    expect(diary).not.toHaveTextContent("Nothing is fixed");

    rerender(<Diary days={null} />);
    expect(diary).toHaveTextContent("The calendar could not be read just now.");
    rerender(<Diary days={[]} />);
    expect(diary).toHaveTextContent("Nothing is fixed in the next three days.");
  });

  it("says whether the doses are being read, could not be, or none are listed", async () => {
    const doses = (data: MedicationsData | null | undefined) => (
      <Doses data={data} busy={false} available={false} onLog={vi.fn()} />
    );
    const { rerender } = render(doses(undefined));
    const column = screen.getByRole("region", { name: "Doses and notes" });
    expect(column).toHaveTextContent("Checking…");
    expect(column).not.toHaveTextContent("No medications listed");

    rerender(doses(null));
    expect(column).toHaveTextContent("Your medications could not be read just now.");
    // What the window reads with no desktop behind it: nothing listed.
    rerender(doses(await loadMedications()));
    expect(column).toHaveTextContent("No medications listed. Add one in Log.");
  });
});

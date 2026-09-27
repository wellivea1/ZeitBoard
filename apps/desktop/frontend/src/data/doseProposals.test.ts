import { describe, expect, it, vi } from "vitest";
import {
  decideDoseProposal,
  doseIsWaiting,
  loadDoseProposals,
  normalizeDoseProposals,
  proposedDoseWording,
} from "./doseProposals";

const dose = {
  proposalId: "dose_proposal_01",
  title: "Record dose",
  medicationId: "med_synthetic",
  medicationLabel: "Synthetic evening tablet",
  status: "taken",
  doseAt: "2026-09-27T21:10:00Z",
  zoneId: "UTC",
  state: "pending",
  createdAt: "2026-09-27T21:12:00Z",
  expiresAt: "2026-09-28T21:12:00Z",
};

describe("proposed doses", () => {
  it("reads only well-formed queues", () => {
    const queue = { pending: [dose], history: [], nextExpiryAt: dose.expiresAt };
    expect(normalizeDoseProposals(queue)?.pending[0]?.medicationLabel).toBe(
      "Synthetic evening tablet",
    );
    // A medication without a label on this computer still reads.
    expect(
      normalizeDoseProposals({ ...queue, pending: [{ ...dose, medicationLabel: "" }] }),
    ).toBeDefined();
    for (const broken of [
      { ...queue, pending: [{ ...dose, state: "approved" }] },
      { ...queue, pending: [{ ...dose, status: "two tablets" }] },
      { ...queue, pending: [{ ...dose, doseAt: "tonight" }] },
      { ...queue, pending: [{ ...dose, medicationLabel: undefined }] },
      { ...queue, nextExpiryAt: "soon" },
      { pending: [dose] },
    ]) {
      expect(normalizeDoseProposals(broken)).toBeUndefined();
    }
  });

  it("is off without the desktop, and an error when the desktop lacks the service", async () => {
    expect((await loadDoseProposals({})).status).toBe("off");
    expect((await loadDoseProposals({ go: { main: { App: {} } } })).status).toBe("error");
    const malformed = { go: { main: { App: { GetDoseProposals: async () => ({ pending: 3 }) } } } };
    expect((await loadDoseProposals(malformed)).status).toBe("error");
  });

  it("records on accept and records nothing on decline", async () => {
    const DecideDoseProposal = vi.fn(async () => ({ pending: [], history: [], nextExpiryAt: "" }));
    const root = { go: { main: { App: { DecideDoseProposal } } } };
    await decideDoseProposal("dose_proposal_01", "approved", root);
    await decideDoseProposal("dose_proposal_01", "rejected", root);
    expect(DecideDoseProposal.mock.calls).toEqual([
      [{ proposalId: "dose_proposal_01", decision: "record" }],
      [{ proposalId: "dose_proposal_01", decision: "discard" }],
    ]);
  });

  it("waits until it lapses, and reads on this computer's clock", () => {
    const proposal = normalizeDoseProposals({ pending: [dose], history: [], nextExpiryAt: "" })!
      .pending[0]!;
    expect(doseIsWaiting(proposal, Date.parse("2026-09-28T21:11:59Z"))).toBe(true);
    expect(doseIsWaiting(proposal, Date.parse(dose.expiresAt))).toBe(false);
    const at = new Date(dose.doseAt);
    const clock = at.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit" });
    // "today" or "tonight", by the hour on this computer.
    expect(proposedDoseWording(proposal, at)).toMatch(
      new RegExp(`^taken (today|tonight) at ${clock}$`),
    );
  });
});

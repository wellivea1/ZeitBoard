import { describe, expect, it, vi } from "vitest";
import {
  decideAgentProposal,
  loadAgentProposals,
  normalizeAgentProposals,
  proposalIsWaiting,
  proposalSubject,
  proposedDoseWording,
  proposedTaskWording,
} from "./agentProposals";

const base = {
  state: "pending",
  createdAt: "2026-09-27T21:12:00Z",
  expiresAt: "2026-09-28T21:12:00Z",
};
const dose = {
  ...base,
  proposalId: "agent_proposal_dose",
  title: "Record dose",
  dose: {
    medicationId: "med_synthetic",
    medicationLabel: "Synthetic evening tablet",
    status: "taken",
    doseAt: "2026-09-27T21:10:00Z",
    zoneId: "UTC",
  },
};
const task = {
  ...base,
  proposalId: "agent_proposal_task",
  title: "Add task",
  task: { title: "Call the pharmacy", durationMinutes: 15, latestFinishAt: "2026-09-30T21:00:00Z" },
};

describe("assistant proposals", () => {
  it("reads only well-formed queues of doses and tasks", () => {
    const queue = { pending: [dose, task], history: [], nextExpiryAt: base.expiresAt };
    const read = normalizeAgentProposals(queue);
    expect(read?.pending.map((item) => item.kind)).toEqual(["dose", "task"]);
    // A medication without a label on this computer still reads.
    expect(
      normalizeAgentProposals({
        ...queue,
        pending: [{ ...dose, dose: { ...dose.dose, medicationLabel: "" } }],
      }),
    ).toBeDefined();
    for (const broken of [
      { ...queue, pending: [{ ...dose, state: "recorded" }] },
      { ...queue, pending: [{ ...dose, dose: { ...dose.dose, status: "two tablets" } }] },
      { ...queue, pending: [{ ...dose, dose: { ...dose.dose, doseAt: "tonight" } }] },
      { ...queue, pending: [{ ...task, task: { ...task.task, durationMinutes: "15" } }] },
      { ...queue, pending: [{ ...task, task: { ...task.task, latestFinishAt: "Friday" } }] },
      // Exactly one kind.
      { ...queue, pending: [{ ...dose, task: task.task }] },
      { ...queue, pending: [{ ...base, proposalId: "agent_proposal_none", title: "Record dose" }] },
      { ...queue, nextExpiryAt: "soon" },
      { pending: [dose] },
    ]) {
      expect(normalizeAgentProposals(broken)).toBeUndefined();
    }
  });

  it("is off without the desktop, and an error when the desktop lacks the service", async () => {
    expect((await loadAgentProposals({})).status).toBe("off");
    expect((await loadAgentProposals({ go: { main: { App: {} } } })).status).toBe("error");
    const malformed = {
      go: { main: { App: { GetAgentProposals: async () => ({ pending: 3 }) } } },
    };
    expect((await loadAgentProposals(malformed)).status).toBe("error");
  });

  it("sends the decision as it is: approved or rejected", async () => {
    const DecideAgentProposal = vi.fn(async () => ({
      pending: [],
      history: [],
      nextExpiryAt: "",
    }));
    const root = { go: { main: { App: { DecideAgentProposal } } } };
    await decideAgentProposal("agent_proposal_dose", "approved", root);
    await decideAgentProposal("agent_proposal_task", "rejected", root);
    expect(DecideAgentProposal.mock.calls).toEqual([
      [{ proposalId: "agent_proposal_dose", decision: "approved" }],
      [{ proposalId: "agent_proposal_task", decision: "rejected" }],
    ]);
  });

  it("waits until it lapses, and reads on this computer's clock", () => {
    const [readDose, readTask] = normalizeAgentProposals({
      pending: [dose, task],
      history: [],
      nextExpiryAt: "",
    })!.pending;
    expect(proposalIsWaiting(readDose!, Date.parse("2026-09-28T21:11:59Z"))).toBe(true);
    expect(proposalIsWaiting(readDose!, Date.parse(base.expiresAt))).toBe(false);
    const at = new Date(dose.dose.doseAt);
    const clock = at.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit" });
    // "today" or "tonight", by the hour on this computer.
    expect(proposedDoseWording(dose.dose as never, at)).toMatch(
      new RegExp(`^taken (today|tonight) at ${clock}$`),
    );
    expect(proposedTaskWording(task.task, at)).toMatch(/^15 minutes, by \w+ at /);
    expect(proposedTaskWording({ title: "Taxes", durationMinutes: 120 }, at)).toBe("2 hours");
    expect(proposalSubject(readDose!)).toBe("the dose of Synthetic evening tablet");
    expect(proposalSubject(readTask!)).toBe("the task “Call the pharmacy”");
  });
});

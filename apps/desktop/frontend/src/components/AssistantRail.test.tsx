import type { ReactNode } from "react";
import { fireEvent, render as renderInto, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AgentProposalsProvider } from "../state/agentProposals";
import { ApprovalQueueProvider } from "../state/approvalQueue";
import { ApprovalsProvider } from "../state/approvals";
import { BackendProposalsProvider } from "../state/backendProposals";
import { VisitorRequestsProvider } from "../state/visitorRequests";
import { AssistantRail } from "./AssistantRail";

function Providers({ children }: { children: ReactNode }) {
  return (
    <ApprovalsProvider>
      <BackendProposalsProvider>
        <VisitorRequestsProvider>
          <AgentProposalsProvider>
            <ApprovalQueueProvider>{children}</ApprovalQueueProvider>
          </AgentProposalsProvider>
        </VisitorRequestsProvider>
      </BackendProposalsProvider>
    </ApprovalsProvider>
  );
}

const render = (ui: ReactNode) => renderInto(<Providers>{ui}</Providers>);

// The rail over a synthetic desktop whose assistant runs on the owner's
// ChatGPT plan (ADR-0066).

const planStatus = {
  enabled: true,
  configured: true,
  provider: "chatgpt",
  model: "synthetic-fast",
  auth: "chatgpt_plan",
};

function install(app: Record<string, unknown>) {
  (globalThis as unknown as { go?: unknown }).go = { main: { App: app } };
}

afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

function reply(reason: string, answer: string) {
  return {
    available: true,
    result: "answer_only",
    answer,
    configured: true,
    provider: "chatgpt",
    auth: "chatgpt_plan",
    reason,
    proposals: [],
  };
}

it("says the plan pays, and offers its usage page by the composer", async () => {
  const openPage = vi.fn(async () => undefined);
  install({ GetAssistantStatus: async () => planStatus, OpenAssistantPage: openPage });
  render(<AssistantRail open onClose={() => undefined} />);
  expect(await screen.findByText("ChatGPT plan")).toBeVisible();
  expect(screen.getByText(/Using your ChatGPT plan/)).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "Manage usage" }));
  expect(openPage).toHaveBeenCalledWith({ page: "usage" });
});

it("answers a reply that could not be given with what to do about it", async () => {
  const openPage = vi.fn(async () => undefined);
  const answers = [
    reply("usage_limit", "The assistant's service hit its usage limit."),
    reply("sign_in_ended", "The assistant's ChatGPT sign-in has ended."),
  ];
  install({
    GetAssistantStatus: async () => planStatus,
    OpenAssistantPage: openPage,
    SendAssistantMessage: async () => answers.shift(),
  });
  render(<AssistantRail open onClose={() => undefined} />);
  const composer = await screen.findByRole("textbox", { name: "Message the assistant" });

  fireEvent.change(composer, { target: { value: "Find 90 minutes for paperwork" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect(await screen.findByText("The assistant's service hit its usage limit.")).toBeVisible();
  const usage = screen.getAllByRole("button", { name: "Manage usage" });
  // One beside the reply, one by the composer.
  expect(usage).toHaveLength(2);
  fireEvent.click(usage[0]!);
  expect(openPage).toHaveBeenCalledWith({ page: "usage" });

  fireEvent.change(composer, { target: { value: "Try again" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect(await screen.findByText("The assistant's ChatGPT sign-in has ended.")).toBeVisible();
  expect(screen.getByRole("link", { name: "Sign in under Settings" })).toHaveAttribute(
    "href",
    "#/settings/sync",
  );
});

it("points an assistant not yet signed in to Settings › Sync", async () => {
  install({
    GetAssistantStatus: async () => ({
      ...planStatus,
      configured: false,
      problem: "sign_in_needed",
    }),
  });
  render(<AssistantRail open onClose={() => undefined} />);
  expect(await screen.findByText("ChatGPT · not signed in")).toBeVisible();
  expect(screen.getByRole("link", { name: "Settings › Sync" })).toHaveAttribute(
    "href",
    "#/settings/sync",
  );
  expect(screen.queryByText(/Using your ChatGPT plan/)).toBeNull();
});

it("reads the status again when the sign-in changes in Settings", async () => {
  let status: Record<string, unknown> = {
    ...planStatus,
    configured: false,
    problem: "sign_in_needed",
  };
  install({ GetAssistantStatus: async () => status });
  render(<AssistantRail open onClose={() => undefined} />);
  await screen.findByText("ChatGPT · not signed in");
  status = planStatus;
  window.dispatchEvent(new Event("zeitboard:assistant-connection-changed"));
  await waitFor(() => expect(screen.getByText("ChatGPT plan")).toBeVisible());
});

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AssistantSettings } from "./AssistantSettings";

// The card over a synthetic desktop: no server, no ChatGPT, no account.

const disconnected = { reachable: true, offered: true, state: "disconnected" };
const connected = {
  reachable: true,
  offered: true,
  state: "connected",
  email: "owner@example.com",
  model: "synthetic-fast",
};
const models = [
  { slug: "synthetic-fast", displayName: "Synthetic Fast" },
  { slug: "synthetic-deep", displayName: "Synthetic Deep" },
];

function install(app: Record<string, unknown>) {
  (globalThis as unknown as { go?: unknown }).go = { main: { App: app } };
}

beforeEach(() => window.localStorage.clear());
afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

it("signs in through the browser, waits for it, and says once that the plan now pays", async () => {
  let answer: (value: unknown) => void = () => undefined;
  const connect = vi.fn(
    () =>
      new Promise((resolve) => {
        answer = resolve;
      }),
  );
  const cancel = vi.fn(async () => undefined);
  install({
    GetAssistantConnection: async () => disconnected,
    ConnectChatGPT: connect,
    CancelChatGPTSignIn: cancel,
    ListAssistantModels: async () => models,
  });
  render(<AssistantSettings syncKey="true device" />);

  // What is sent is said before signing in, not behind a fold.
  expect(await screen.findByText(/sends planning facts only/)).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "Continue with ChatGPT" }));
  expect(connect).toHaveBeenCalledOnce();
  expect(await screen.findByText(/Finish signing in to ChatGPT in your browser/)).toBeVisible();
  expect(screen.queryByRole("button", { name: "Continue with ChatGPT" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(cancel).toHaveBeenCalledOnce();

  answer(connected);
  const welcome = await screen.findByRole("dialog", { name: "You're using your ChatGPT plan." });
  const gotIt = screen.getByRole("button", { name: "Got it" });
  expect(gotIt).toHaveFocus();
  expect(welcome).toHaveTextContent("count against your plan's limits");
  fireEvent.click(gotIt);
  expect(screen.queryByRole("dialog")).toBeNull();
  // The button that began the sign-in is gone; the card takes the focus.
  expect(screen.getByRole("heading", { name: "Your ChatGPT plan" })).toHaveFocus();
  expect(screen.getByRole("status")).toHaveTextContent(
    "The assistant is signed in to owner@example.com.",
  );
  expect(await screen.findByRole("combobox", { name: "Model" })).toHaveValue("synthetic-fast");
});

it("shows the welcome once, however often the owner signs in", async () => {
  window.localStorage.setItem("zeitboard.assistant.chatgptWelcomed", "true");
  install({
    GetAssistantConnection: async () => disconnected,
    ConnectChatGPT: async () => connected,
    ListAssistantModels: async () => models,
  });
  render(<AssistantSettings syncKey="true device" />);
  fireEvent.click(await screen.findByRole("button", { name: "Continue with ChatGPT" }));
  await screen.findByRole("combobox", { name: "Model" });
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("says why a sign-in did not finish and what ChatGPT left undone", async () => {
  install({
    GetAssistantConnection: async () => ({ ...disconnected, problem: "plan_not_shared" }),
    ConnectChatGPT: async () => {
      throw "ChatGPT did not share your plan with ZeitBoard, so it was not connected";
    },
  });
  render(<AssistantSettings syncKey="true device" />);
  expect(await screen.findByText(/allow ZeitBoard to\s+use it/)).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "Continue with ChatGPT" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "ChatGPT did not share your plan with ZeitBoard, so it was not connected.",
  );
  expect(screen.getByRole("button", { name: "Continue with ChatGPT" })).toBeEnabled();
});

it("chooses a model, opens usage, and disconnects, saying when ChatGPT did not confirm", async () => {
  const choose = vi.fn(async (input: { model: string }) => ({ ...connected, model: input.model }));
  const openPage = vi.fn(async () => undefined);
  install({
    GetAssistantConnection: async () => connected,
    ListAssistantModels: async () => models,
    ChooseAssistantModel: choose,
    OpenAssistantPage: openPage,
    DisconnectChatGPT: async () => ({ ...disconnected, revoked: false }),
  });
  render(<AssistantSettings syncKey="true device" />);
  expect(await screen.findByText(/Signed in as owner@example.com/)).toBeVisible();
  fireEvent.change(await screen.findByRole("combobox", { name: "Model" }), {
    target: { value: "synthetic-deep" },
  });
  await waitFor(() => expect(choose).toHaveBeenCalledWith({ model: "synthetic-deep" }));
  expect(await screen.findByText("The assistant now asks Synthetic Deep.")).toBeVisible();

  fireEvent.click(screen.getByRole("button", { name: "Manage usage" }));
  expect(openPage).toHaveBeenCalledWith({ page: "usage" });

  fireEvent.click(screen.getByRole("button", { name: "Disconnect" }));
  expect(await screen.findByText(/remove ZeitBoard in ChatGPT's settings too/)).toBeVisible();
  expect(await screen.findByRole("button", { name: "Continue with ChatGPT" })).toBeVisible();
});

it("offers signing in again to an ended sign-in", async () => {
  install({
    GetAssistantConnection: async () => ({
      ...disconnected,
      state: "sign_in_ended",
      email: "owner@example.com",
      problem: "sign_in_ended",
    }),
  });
  render(<AssistantSettings syncKey="true device" />);
  expect(await screen.findByText(/The sign-in to owner@example.com has ended/)).toBeVisible();
  expect(screen.getByRole("button", { name: "Sign in again" })).toBeVisible();
  expect(screen.getByRole("button", { name: "Disconnect" })).toBeVisible();
});

it("says what stands in the way when there is nothing to sign in to", async () => {
  install({
    GetAssistantConnection: async () => ({
      reachable: false,
      offered: false,
      message: "Connect your server above to set up the assistant.",
    }),
  });
  const { unmount } = render(<AssistantSettings syncKey="false " />);
  expect(screen.getByText("Checking…")).toBeVisible();
  expect(
    await screen.findByText("Connect your server above to set up the assistant."),
  ).toBeVisible();
  expect(screen.queryByRole("button", { name: "Continue with ChatGPT" })).toBeNull();
  unmount();

  install({ GetAssistantConnection: async () => ({ reachable: true, offered: false }) });
  const configured = render(<AssistantSettings syncKey="true device" />);
  expect(await screen.findByText(/set the assistant's provider to chatgpt/)).toBeVisible();
  expect(screen.queryByRole("button", { name: "Continue with ChatGPT" })).toBeNull();
  configured.unmount();

  // A server too old to offer a sign-in is not said to name a provider.
  const older =
    "Your server is from before signing in to ChatGPT. Update it to use your ChatGPT plan.";
  install({
    GetAssistantConnection: async () => ({ reachable: true, offered: false, message: older }),
  });
  render(<AssistantSettings syncKey="true device" />);
  expect(await screen.findByText(older)).toBeVisible();
  expect(screen.queryByText(/set the assistant's provider to chatgpt/)).toBeNull();
});

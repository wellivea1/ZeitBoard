import { describe, expect, it, vi } from "vitest";
import {
  appearanceCommandEvent,
  listenForAppearanceCommands,
  loadLocalAgentStatus,
  loadLocalAppearanceState,
  saveLocalAppearanceState,
  type LocalAppearanceState,
} from "./localAgent";

const appearance: LocalAppearanceState = {
  theme: "dark",
  reducedStimulation: true,
  nightRule: {
    enabled: true,
    preset: "amber",
    leadHours: 2,
    fallbackStartLocal: "21:00",
    fallbackEndLocal: "07:00",
  },
};

describe("local-agent bridge", () => {
  it("normalizes desktop-local status without exposing a credential", async () => {
    const result = await loadLocalAgentStatus({
      go: {
        main: {
          App: {
            GetLocalAgentStatus: async () => ({
              schemaVersion: "v1",
              mode: "desktop_local",
              running: true,
              endpoint: "http://127.0.0.1:43123/mcp",
              message: "ready",
              backendProposalsAvailable: false,
              localStoreAvailable: true,
              appearanceStatus: "ready",
              token: "must-not-cross-the-binding",
            }),
          },
        },
      },
    });

    expect(result).toEqual({
      schemaVersion: "v1",
      mode: "desktop_local",
      running: true,
      endpoint: "http://127.0.0.1:43123/mcp",
      message: "ready",
      backendProposalsAvailable: false,
      localStoreAvailable: true,
      appearanceStatus: "ready",
    });
    expect(result).not.toHaveProperty("token");
  });

  it("uses browser-safe status when the Wails binding is absent", async () => {
    const result = await loadLocalAgentStatus({});
    expect(result.running).toBe(false);
    expect(result.backendProposalsAvailable).toBe(false);
    expect(result.message).toMatch(/installed desktop app/i);
  });

  it("loads and saves revisioned appearance envelopes", async () => {
    const load = vi.fn(async (input: unknown) => ({ state: input, revision: 4 }));
    const save = vi.fn(async (input: unknown) => ({
      state: (input as { state: LocalAppearanceState }).state,
      revision: 5,
      conflict: false,
    }));
    const root = {
      go: { main: { App: { LoadLocalAppearanceState: load, SaveLocalAppearanceState: save } } },
    };

    await expect(loadLocalAppearanceState(appearance, root)).resolves.toEqual({
      state: appearance,
      revision: 4,
      conflict: false,
    });
    await expect(saveLocalAppearanceState(appearance, 4, root)).resolves.toEqual({
      state: appearance,
      revision: 5,
      conflict: false,
    });
    expect(load).toHaveBeenCalledWith(appearance);
    expect(save).toHaveBeenCalledWith({ state: appearance, baseRevision: 4 });
  });

  it("rejects invalid appearance envelopes", async () => {
    await expect(
      loadLocalAppearanceState(appearance, {
        go: { main: { App: { LoadLocalAppearanceState: async () => ({ state: appearance }) } } },
      }),
    ).rejects.toThrow(/invalid appearance/i);
  });

  it("normalizes runtime appearance events and disposes the listener", () => {
    let eventCallback: ((value: unknown) => void) | undefined;
    const dispose = vi.fn();
    const callback = vi.fn();
    const root = {
      runtime: {
        EventsOn: vi.fn((eventName: string, registered: (value: unknown) => void) => {
          expect(eventName).toBe(appearanceCommandEvent);
          eventCallback = registered;
          return dispose;
        }),
      },
    };

    const stop = listenForAppearanceCommands(callback, root);
    eventCallback?.({ state: appearance, revision: 7, conflict: false });
    eventCallback?.({ state: { ...appearance, theme: "invalid" }, revision: 8 });

    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith({ state: appearance, revision: 7, conflict: false });
    stop();
    expect(dispose).toHaveBeenCalledOnce();
  });
});

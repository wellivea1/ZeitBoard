import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { LocalAgentStatus } from "../../data/localAgent";
import { LocalAgentSettings } from "./LocalAgentSettings";

const readyStatus: LocalAgentStatus = {
  schemaVersion: "v1",
  mode: "desktop_local",
  running: true,
  endpoint: "http://127.0.0.1:43123/mcp",
  message: "Local agent ready.",
  backendProposalsAvailable: false,
  localStoreAvailable: true,
  appearanceStatus: "ready",
};

describe("LocalAgentSettings", () => {
  it("uses a consistent loading state until status arrives", () => {
    render(<LocalAgentSettings status={null} error="" />);

    expect(screen.getAllByText("Checking")).toHaveLength(5);
    expect(screen.queryByText("Not available")).not.toBeInTheDocument();
    expect(screen.queryByText("Ready")).not.toBeInTheDocument();
  });

  it("discloses endpoint and proposal availability without a credential", () => {
    render(<LocalAgentSettings status={readyStatus} error="" />);

    expect(screen.getByText("http://127.0.0.1:43123/mcp")).toBeInTheDocument();
    expect(screen.getByText("Unavailable while backend sync is off")).toBeInTheDocument();
    expect(screen.getByText("Local agent ready.")).toHaveAttribute("role", "status");
    expect(screen.getByText(/per-launch bearer credential/i)).toBeInTheDocument();
    expect(screen.queryByText(/token/i)).not.toBeInTheDocument();
  });

  it("shows repair and startup errors explicitly", () => {
    render(
      <LocalAgentSettings
        status={{
          ...readyStatus,
          running: false,
          endpoint: undefined,
          appearanceStatus: "error",
          message: "The endpoint is not running.",
        }}
        error="Could not read desktop-local agent status."
      />,
    );

    expect(screen.getByText("Stopped")).toBeInTheDocument();
    expect(screen.getByText("Needs repair")).toBeInTheDocument();
    expect(screen.getByText("Not available")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Could not read desktop-local agent status.",
    );
  });
});

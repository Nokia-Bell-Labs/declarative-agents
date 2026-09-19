// @vitest-environment jsdom
import { act, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { fixtures } from "../../src/fixtures";
import { eventInWindow, fleetPanel } from "../../src/panels/FleetPanel";
import { FakeEventSource, FixtureProvider } from "./support";

const agents = [
  { name: "chatbot", label: "Chatbot" },
  { name: "rag1", label: "RAG server 1" },
];

describe("FleetPanel", () => {
  it("renders one sub-panel per deployed agent and hides the absent one", async () => {
    const Mount = fleetPanel.component;
    render(
      <FixtureProvider absentAgents={["rag1"]}>
        <Mount monitoredAgents={agents} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("agent-state").textContent).toBe("AwaitingRequest"));
    expect(screen.getAllByTestId("agent-panel").map((node) => node.getAttribute("data-agent"))).toEqual(["chatbot"]);
    act(() => FakeEventSource.last!.emit("run_event", fixtures["/monitor/events/stream"][0].data));
    expect(screen.getByText("embed_query")).toBeTruthy();
  });

  it("highlights events inside the selected window", () => {
    expect(eventInWindow({ startedAt: 10, endedAt: 20 }, 15)).toBe(true);
    expect(eventInWindow({ startedAt: 10, endedAt: 20 }, 600)).toBe(false);
    expect(eventInWindow({ startedAt: 10 }, 50, 40)).toBe(true);
    expect(eventInWindow(undefined, 15)).toBe(false);
  });
});

// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { createKitClient } from "../../src/client/client";
import { KitClientProvider } from "../../src/client/context";
import { fixtureFetch } from "../../src/fixtures";
import {
  AgentCard,
  AgentCards,
  agentCardManifest,
  agentCardPanel,
  agentState,
  agentStateClass,
  NO_AGENTS_TEXT,
  orderAgents,
  toolRecords,
  yamlify,
} from "../../src/panels/AgentCard";
import { FixtureProvider } from "./support";

const Mount = agentCardPanel.component;
const emptyData = { agents: [], pods: [], deployments: [], services: [], podMetrics: [] };

afterEach(cleanup);

describe("AgentCard panel", () => {
  it("exports its manifest through definePanel", () => {
    expect(agentCardPanel.manifest).toBe(agentCardManifest);
    expect(agentCardManifest).toEqual({ id: "agent-card", title: "Agents", route: "/agents", required_endpoints: ["/monitor/fleet"], monitored_agents: [] });
  });

  it("renders one card per fleet agent, healthy and unreachable", async () => {
    render(
      <FixtureProvider>
        <Mount monitoredAgents={[]} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getAllByTestId("agent-card")).toHaveLength(2));
    const [chatbot, rag0] = screen.getAllByTestId("agent-card");
    expect(chatbot.getAttribute("data-agent")).toBe("chatbot-7d9c8b6f4-x2k9p");
    expect(chatbot.querySelector(".agent-name")?.textContent).toBe("chatbot");
    expect(chatbot.querySelector(".agent-pod")?.textContent).toBe("chatbot-7d9c8b6f4-x2k9p");
    expect(chatbot.querySelector(".agent-state")?.textContent).toBe("Serving");
    expect(chatbot.querySelector(".resources")?.textContent).toBe("cpu 2500umem 8704Ki");
    expect(chatbot.textContent).toContain("Serve chat turns.");
    expect(chatbot.textContent).toContain("invoke_llm_fast");
    expect(chatbot.textContent).toContain("Launched");
    // The pod whose monitor read failed is a card that says so.
    expect(rag0.getAttribute("data-reachable")).toBe("false");
    expect(rag0.querySelector(".agent-state")?.textContent).toBe("unreachable");
    expect(rag0.querySelector(".agent-state")?.className).toContain("state-failed");
    expect(rag0.textContent).toContain("metrics unavailable");
  });

  it("orders cards by the configured short names", async () => {
    render(
      <FixtureProvider>
        <Mount monitoredAgents={[]} config={{ order: ["rag0", "chatbot"] }} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getAllByTestId("agent-card")).toHaveLength(2));
    expect(screen.getAllByTestId("agent-card").map((card) => card.querySelector(".agent-name")?.textContent)).toEqual(["rag0", "chatbot"]);
  });

  it("renders the empty fleet as waiting for pods", async () => {
    render(
      <FixtureProvider overrides={{ "/monitor/fleet": {} }}>
        <Mount monitoredAgents={[]} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("agent-cards-empty").textContent).toBe(NO_AGENTS_TEXT));
    expect(screen.queryByTestId("agent-card")).toBeNull();
  });

  it("states why the fleet is missing when the fleet read fails", async () => {
    const fixture = fixtureFetch();
    const client = createKitClient({
      fetch: async (input, init) => (new URL(String(input), "http://x.invalid").pathname === "/monitor/fleet" ? new Response("gone", { status: 404 }) : fixture(input, init)),
    });
    render(
      <KitClientProvider client={client}>
        <Mount monitoredAgents={[]} />
      </KitClientProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("agent-cards-empty").textContent).toMatch(/^Fleet unavailable: .*404/));
  });
});

describe("AgentCard view", () => {
  it("renders degraded metrics and the fleet-only sections (observer render case)", () => {
    render(
      <AgentCard
        agent={{
          name: "chatbot",
          reachable: true,
          state: { current_state: "running" },
          machine: { purpose: "Answer questions\nsecond line", states: ["idle"], transitions: [] },
          tools: [{ name: "rag_query" }],
          events: [{ signal: "Started", timestamp: "2026-08-03T12:00:00Z" }],
        }}
      />,
    );
    expect(screen.getByText("metrics unavailable")).toBeTruthy();
    expect(screen.getByText("running").className).toContain("state-running");
    expect(screen.getByText("Answer questions")).toBeTruthy();
    expect(screen.getByText("rag_query")).toBeTruthy();
    expect(screen.getByText("Started")).toBeTruthy();
    expect(screen.queryByText("chatbot", { selector: ".agent-pod" })).toBeNull();
  });

  it("renders aggregated pod metrics through AgentCards (observer render case)", () => {
    const { container } = render(
      <AgentCards
        data={{
          ...emptyData,
          agents: [{ name: "observer-0", reachable: true, state: "AwaitingControl" }],
          podMetrics: [{ metadata: { name: "observer-0" }, containers: [{ usage: { cpu: "500000n", memory: "1Mi" } }, { usage: { cpu: "500u", memory: "512Ki" } }] }],
        }}
      />,
    );
    expect(container.textContent).toContain("cpu 1m");
    expect(container.textContent).toContain("mem 1536Ki");
    expect(container.textContent).not.toContain("metrics unavailable");
  });

  it("shows egress, the request walk, and the tool record on click", () => {
    render(
      <AgentCard
        agent={{ name: "demo-chatbot-7d9c8b6f4-x2k9p", state: "Serving", tools: [{ name: "invoke_llm", category: "llm", emits: ["Answered"] }] }}
        shortName="chatbot"
        egress={["api.cohere.com", "host.docker.internal"]}
        walk={{ service: "chatbot", steps: [{ iteration: 1, command: "embed", signal: "Embedded" }, { iteration: 2, command: "compose", signal: "ComposeFailed" }] }}
      />,
    );
    expect(screen.getByText("calls · api.cohere.com · host.docker.internal")).toBeTruthy();
    expect(screen.getByText("Last run · 2 steps")).toBeTruthy();
    expect(screen.getByText("ComposeFailed").className).toContain("walk-signal-failed");
    const tag = screen.getByText("invoke_llm");
    expect(tag.className).toContain("tool-cat-llm");
    fireEvent.click(tag);
    expect(document.querySelector(".tool-detail")?.textContent).toBe("name: invoke_llm\ncategory: llm\nemits:\n  - Answered");
    fireEvent.click(tag);
    expect(document.querySelector(".tool-detail")).toBeNull();
  });

  it("opens the supervisor machine as a transition list or through the supplied renderer", () => {
    const agent = { name: "chatbot", state: "Serving", machine: { states: ["Launching", "Serving"], transitions: [{ state: "Launching", signal: "Launched", next: "Serving", action: "launch" }] } };
    const { unmount } = render(<AgentCard agent={agent} />);
    fireEvent.click(screen.getByText("2 states · 1 transitions"));
    expect(document.querySelector(".machine-transitions")?.textContent).toBe("LaunchingLaunchedServinglaunch");
    unmount();
    render(<AgentCard agent={agent} renderMachine={(machine, current) => <span>drawn {machine.states?.length} at {current}</span>} />);
    fireEvent.click(screen.getByText("2 states · 1 transitions"));
    expect(screen.getByText("drawn 2 at Serving")).toBeTruthy();
  });

  it("lets a machine slot replace the supervisor section", () => {
    render(<AgentCard agent={{ name: "chatbot", machine: { states: ["A"] } }} machineSlot={<div>machine panel</div>} />);
    expect(screen.getByText("machine panel")).toBeTruthy();
    expect(screen.queryByText(/states ·/)).toBeNull();
  });
});

describe("card helpers", () => {
  it("reads either state shape and classes it", () => {
    expect(agentState({ state: "Serving" })).toBe("Serving");
    expect(agentState({ state: { current_state: "Done" } })).toBe("Done");
    expect(agentState({})).toBe("");
    expect(["done", "Succeeded", "FAILED", "idle", "", "Serving"].map(agentStateClass)).toEqual(["state-done", "state-done", "state-failed", "state-idle", "state-idle", "state-running"]);
  });

  it("keeps tool records from either tools shape", () => {
    expect(toolRecords({ tools: [{ name: "a", category: "control" }, "b"] }).map(({ name, category }) => ({ name, category }))).toEqual([
      { name: "a", category: "control" },
      { name: "b", category: undefined },
    ]);
    expect(toolRecords(undefined)).toEqual([]);
  });

  it("renders records as YAML, dropping nulls and marking empties", () => {
    expect(yamlify({ name: "x", tags: [], meta: {}, skip: null, list: [{ a: 1 }] })).toBe("name: x\ntags: []\nmeta:\n  {}\nlist:\n  -\n    a: 1");
  });

  it("orders by configured short names and keeps discovery order otherwise", () => {
    const agents = [{ name: "p-exporter-0" }, { name: "p-chatbot-0" }, { name: "p-extra-0" }, { name: "p-rag0-0" }];
    expect(orderAgents(agents, "p-", ["chatbot", "rag0"]).map((agent) => agent.name)).toEqual(["p-chatbot-0", "p-rag0-0", "p-exporter-0", "p-extra-0"]);
    expect(orderAgents(agents, "p-")).toBe(agents);
  });
});

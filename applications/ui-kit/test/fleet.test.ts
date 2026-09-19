import { describe, expect, it } from "vitest";
import { createKitClient } from "../src/client/client";
import { fetchFleet, fetchObserverState, fleetData, metricsByPod, objectName } from "../src/api/fleetApi";
import { fixtureFetch, fixtures } from "../src/fixtures";

// fanInItem builds one join outcome: the dispatched pod plus that pod's result.
function fanInItem(pod: string, body: unknown, signal = "AgentMonitorRead") {
  return { index: 0, input: { metadata: { name: pod } }, command_name: "read_agent", result: { signal, structured_output: { body } } };
}
function fanInLabel(items: unknown[]) {
  return { output: { items, succeeded: items.length, failed: 0, policy: "collect_all" } };
}

describe("fleetData", () => {
  it("zips the four monitor fan-in joins into one agent per pod", () => {
    const data = fleetData({
      labels: {
        agent_machine_fanin: fanInLabel([fanInItem("chatbot-0", { name: "chatbot", states: ["Idle", "Serving"], transitions: [{}, {}, {}] })]),
        agent_state_fanin: fanInLabel([fanInItem("chatbot-0", { run: { state: "Serving", status: "running" }, diagnostics: [], errors: [] })]),
        agent_tools_fanin: fanInLabel([fanInItem("chatbot-0", { tools: [{ name: "rag_query" }, { name: "invoke_llm" }] })]),
        agent_events_fanin: fanInLabel([fanInItem("chatbot-0", { recent_events: [{ signal: "Started", timestamp: "2026-08-06T12:00:00Z" }] })]),
      },
    } as never);
    expect(data.agents).toHaveLength(1);
    const agent = data.agents[0];
    expect(agent).toMatchObject({ name: "chatbot-0", reachable: true, state: "Serving" });
    expect(agent.machine?.states).toHaveLength(2);
    expect(agent.machine?.transitions).toHaveLength(3);
    expect(agent.tools).toEqual([{ name: "rag_query" }, { name: "invoke_llm" }]);
    expect(agent.events?.[0]?.signal).toBe("Started");
  });

  it("marks only the pod whose state read failed as unreachable", () => {
    const data = fleetData({
      labels: {
        agent_state_fanin: fanInLabel([fanInItem("chatbot-0", { run: { state: "Serving" } }), fanInItem("rag-0", undefined, "CommandError")]),
        agent_tools_fanin: fanInLabel([fanInItem("chatbot-0", { tools: [{ name: "rag_query" }] }), fanInItem("rag-0", undefined, "CommandError")]),
      },
    } as never);
    const byName = new Map(data.agents.map((agent) => [agent.name, agent]));
    expect(byName.get("chatbot-0")).toMatchObject({ reachable: true, state: "Serving" });
    expect(byName.get("rag-0")).toMatchObject({ reachable: false, state: undefined, tools: [] });
  });

  it("degrades to the empty fleet when the joins are absent or unavailable", () => {
    expect(fleetData({}).agents).toEqual([]);
    expect(fleetData({ labels: {} }).agents).toEqual([]);
    const data = fleetData({
      labels: { agent_machine_fanin: { available: false }, agent_state_fanin: { output: {} }, agent_tools_fanin: { output: { items: "not-an-array" } } },
    } as never);
    expect(data.agents).toEqual([]);
  });

  it("parses the recorded fleet fixture", () => {
    const data = fleetData(fixtures["/monitor/fleet"]);
    expect(data.agents.map((agent) => [agent.name, agent.reachable])).toEqual([
      ["chatbot-7d9c8b6f4-x2k9p", true],
      ["rag0-0", false],
    ]);
    expect(data.pods).toHaveLength(2);
    expect(data.deployments.map((d) => objectName(d, "?"))).toEqual(["demo-chatbot-mesh-chatbot"]);
    expect(metricsByPod(data.podMetrics)["chatbot-7d9c8b6f4-x2k9p"]).toEqual({ cpu: "2500u", memory: "8704Ki" });
  });
});

describe("metricsByPod", () => {
  it("aggregates Kubernetes quantities across every pod container", () => {
    const metrics = metricsByPod([
      { metadata: { name: "observer-0" }, containers: [{ usage: { cpu: "500000n", memory: "1Mi" } }, { usage: { cpu: "500u", memory: "512Ki" } }] },
      { metadata: { name: "chatbot-0" }, containers: [{ usage: { cpu: "1m", memory: "1M" } }, { usage: { cpu: "0.001", memory: "500K" } }] },
      { name: "flat-0", cpu: "3m", memory: "4Mi" },
    ]);
    expect(metrics["observer-0"]).toEqual({ cpu: "1m", memory: "1536Ki" });
    expect(metrics["chatbot-0"]).toEqual({ cpu: "2m", memory: "1500K" });
    expect(metrics["flat-0"]).toEqual({ cpu: "3m", memory: "4Mi" });
  });
});

describe("fleet reads", () => {
  it("reads the observer's own fleet and state same-origin", async () => {
    const client = createKitClient({ fetch: fixtureFetch() });
    expect((await fetchFleet(client)).agents).toHaveLength(2);
    expect(await fetchObserverState(client)).toBe("AwaitingRequest");
    const down = createKitClient({ fetch: (async () => new Response("x", { status: 503 })) as typeof fetch });
    await expect(fetchFleet(down)).rejects.toThrow("HTTP 503");
    expect(await fetchObserverState(down)).toBe("");
  });
});

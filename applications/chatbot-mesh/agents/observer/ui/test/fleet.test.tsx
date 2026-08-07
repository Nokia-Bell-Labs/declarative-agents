import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { FleetView } from "../src/App";
import { fleetData, metricsByPod, routes } from "../src/fleet";
import type { FleetSnapshot } from "../src/useFleet";

// fanInItem builds one join outcome: the dispatched pod plus that pod's result.
// signal defaults to the read signal; pass another to mark the agent unreachable.
function fanInItem(pod: string, body: unknown, signal = "AgentMonitorRead") {
  return {
    index: 0,
    input: { metadata: { name: pod } },
    command_name: "read_agent",
    result: { signal, structured_output: { body } },
  };
}

function fanInLabel(items: unknown[]) {
  return { output: { items, succeeded: items.length, failed: 0, policy: "collect_all" } };
}

test("preserves observer monitor routes and command-state contract", () => {
  assert.deepEqual(routes, {
    fleet: "/monitor/fleet",
    state: "/monitor/state",
  });
  const data = fleetData({
    labels: {
      discover_mesh_pods: { output: { mapped: { pods: [{ name: "observer-0" }] } } },
      list_mesh_deployments: { output: { deployments: [{ name: "observer" }] } },
      list_mesh_services: { output: { services: [{ name: "observer" }] } },
      poll_pod_metrics: {
        output: {
          pod_metrics: [{
            name: "observer-0",
            containers: [{ usage: { cpu: "2m", memory: "8Mi" } }],
          }],
        },
      },
    },
  });
  assert.deepEqual(data.pods, [{ name: "observer-0" }]);
  assert.deepEqual(metricsByPod(data.podMetrics)["observer-0"], { cpu: "2m", memory: "8Mi" });
});

test("zips the four monitor fan-in joins into one agent per pod", () => {
  const data = fleetData({
    labels: {
      [ "agent_machine_fanin" ]: fanInLabel([
        fanInItem("chatbot-0", { name: "chatbot", states: ["Idle", "Serving"], transitions: [{}, {}, {}] }),
      ]),
      [ "agent_state_fanin" ]: fanInLabel([
        fanInItem("chatbot-0", { run: { state: "Serving", status: "running" }, diagnostics: [], errors: [] }),
      ]),
      [ "agent_tools_fanin" ]: fanInLabel([
        fanInItem("chatbot-0", { tools: [{ name: "rag_query" }, { name: "invoke_llm" }] }),
      ]),
      [ "agent_events_fanin" ]: fanInLabel([
        fanInItem("chatbot-0", {
          recent_events: [{ signal: "Started", timestamp: "2026-08-06T12:00:00Z" }],
        }),
      ]),
    },
  });

  assert.equal(data.agents.length, 1);
  const agent = data.agents[0];
  // The pod name keys the card so its resource lookup matches per-pod metrics.
  assert.equal(agent?.name, "chatbot-0");
  assert.equal(agent?.reachable, true);
  assert.equal(agent?.state, "Serving");
  assert.equal(agent?.machine?.states?.length, 2);
  assert.equal(agent?.machine?.transitions?.length, 3);
  assert.deepEqual(agent?.tools, [{ name: "rag_query" }, { name: "invoke_llm" }]);
  assert.equal(agent?.events?.[0]?.signal, "Started");
});

test("marks only the pod whose state read failed as unreachable", () => {
  const data = fleetData({
    labels: {
      // The join reports every dispatched item as succeeded, because the fan-in
      // continues past CommandError. Reachability comes from each item's signal.
      [ "agent_state_fanin" ]: fanInLabel([
        fanInItem("chatbot-0", { run: { state: "Serving" } }),
        fanInItem("rag-0", undefined, "CommandError"),
      ]),
      [ "agent_tools_fanin" ]: fanInLabel([
        fanInItem("chatbot-0", { tools: [{ name: "rag_query" }] }),
        fanInItem("rag-0", undefined, "CommandError"),
      ]),
    },
  });

  const byName = new Map(data.agents.map((agent) => [agent.name, agent]));
  assert.equal(byName.get("chatbot-0")?.reachable, true);
  assert.equal(byName.get("chatbot-0")?.state, "Serving");
  assert.equal(byName.get("rag-0")?.reachable, false);
  assert.equal(byName.get("rag-0")?.state, undefined);
  assert.deepEqual(byName.get("rag-0")?.tools, []);
});

test("degrades to the empty fleet when the fan-in joins are absent or unavailable", () => {
  assert.deepEqual(fleetData({}).agents, []);
  assert.deepEqual(fleetData({ labels: {} }).agents, []);
  const data = fleetData({
    labels: {
      [ "agent_machine_fanin" ]: { available: false },
      [ "agent_state_fanin" ]: { output: {} },
      [ "agent_tools_fanin" ]: { output: { items: "not-an-array" } },
    },
  });
  assert.deepEqual(data.agents, []);
});

test("renders the fleet shell, topology, agents, and degraded metrics", () => {
  const snapshot: FleetSnapshot = {
    status: "connected",
    observerState: "observing",
    lastPoll: new Date("2026-08-03T12:00:00Z"),
    data: {
      agents: [{
        name: "chatbot",
        reachable: true,
        state: { current_state: "running" },
        machine: { purpose: "Answer questions", states: ["idle"], transitions: [] },
        tools: [{ name: "rag_query" }],
        events: [{ signal: "Started", timestamp: "2026-08-03T12:00:00Z" }],
      }],
      pods: [{
        metadata: {
          name: "chatbot-0",
          labels: { "app.kubernetes.io/component": "chatbot" },
        },
      }],
      deployments: [{
        metadata: {
          name: "chatbot",
          labels: { "app.kubernetes.io/component": "chatbot" },
        },
      }],
      services: [{
        metadata: { name: "chatbot" },
        spec: { selector: { "app.kubernetes.io/component": "chatbot" } },
      }],
      podMetrics: [],
    },
  };
  const html = renderToStaticMarkup(<FleetView snapshot={snapshot} />);
  for (const expected of [
    "Fleet Observer",
    "Connected · observer observing",
    "Topology",
    "service: chatbot",
    "chatbot-0 · n/a",
    "rag_query",
    "metrics unavailable",
  ]) {
    assert.match(html, new RegExp(expected.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

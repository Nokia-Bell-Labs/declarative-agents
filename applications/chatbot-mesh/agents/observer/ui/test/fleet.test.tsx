import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { FleetView } from "../src/App";
import { fleetData, metricsByPod, routes } from "../src/fleet";
import type { FleetSnapshot } from "../src/useFleet";

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
      poll_agent_monitors: { output: { agents: [{ name: "observer", reachable: true }] } },
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
  assert.equal(data.agents[0]?.name, "observer");
  assert.deepEqual(metricsByPod(data.podMetrics)["observer-0"], { cpu: "2m", memory: "8Mi" });
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

import { describe, expect, it } from "vitest";
import { fleetData, type FleetData } from "../../src/api/fleetApi";
import { toModel, type CollectorSpan, type TraceSummary } from "../../src/api/traceApi";
import { fixtures } from "../../src/fixtures";
import {
  buildTopologyGraph,
  buildTopologyState,
  commonNamePrefix,
  egressByAgent,
  requestWalks,
  roleOf,
  selectTopologyTraces,
  serviceGroups,
  shortUnitName,
  type TopologySpan,
} from "../../src/panels/Topology";

// Spans in the shape the kit TraceModel carries: attributes as a map.
const span = (id: string, service: string, attributes: Record<string, unknown> = {}, parentId?: string): TopologySpan => ({ id, parentId, service, attributes });

describe("short unit names (cohere-demo GH-369)", () => {
  it("strips the release prefix and generated pod segments", () => {
    const prefix = "demo-chatbot-mesh-";
    expect(shortUnitName("demo-chatbot-mesh-chatbot-57f469b678-n6rmf", prefix)).toBe("chatbot");
    expect(shortUnitName("demo-chatbot-mesh-exporter-5b5b7f5dd6-wvjfq", prefix)).toBe("exporter");
    expect(shortUnitName("demo-chatbot-mesh-osint-fixture-54cdddb6c-9dw9v", prefix)).toBe("osint-fixture");
    expect(shortUnitName("demo-chatbot-mesh-rag0-chroma-0", prefix)).toBe("rag0-chroma");
    expect(shortUnitName("demo-chatbot-mesh-exporter", prefix)).toBe("exporter");
    expect(shortUnitName("rag0-0", prefix)).toBe("rag0");
  });

  it("finds the release prefix the deployments share", () => {
    expect(commonNamePrefix(["demo-chatbot-mesh-chatbot", "demo-chatbot-mesh-rag0"])).toBe("demo-chatbot-mesh-");
    expect(commonNamePrefix([])).toBe("");
  });

  it("reads the role from the annotation the caller names, and none without one", () => {
    const deployment = fleetData(fixtures["/monitor/fleet"]).deployments[0];
    expect(roleOf(deployment, "mesh.example/role")).toBe("chat front door");
    expect(roleOf(deployment, "mesh.cohere-demo/role")).toBeUndefined();
    expect(roleOf(deployment)).toBeUndefined();
  });
});

describe("buildTopologyGraph (cohere-demo GH-359)", () => {
  it("derives mesh edges from cross-service parents and egress from server.address", () => {
    const graph = buildTopologyGraph(
      [
        span("a", "chatbot", { "server.address": "api.cohere.com" }),
        span("b", "rag0", {}, "a"),
        span("c", "rag0", { "server.address": "demo-chatbot-mesh-rag0-chroma" }, "b"),
      ],
      ["chatbot", "rag0"],
      new Map([["demo-chatbot-mesh-rag0-chroma", "rag0-chroma"]]),
    );
    const edge = (from: string, to: string) => graph.edges.find((e) => e.from === from && e.to === to);
    expect(edge("chatbot", "rag0")?.kind).toBe("mesh");
    expect(edge("chatbot", "api.cohere.com")?.kind).toBe("egress");
    expect(edge("rag0", "rag0-chroma")?.kind).toBe("egress");
    const kinds = new Map(graph.nodes.map((n) => [n.id, n.kind]));
    expect(kinds.get("chatbot")).toBe("agent");
    expect(kinds.get("api.cohere.com")).toBe("external");
    expect(kinds.get("rag0-chroma")).toBe("service");
  });

  it("marks the observer's edges as polling and counts repeats", () => {
    const graph = buildTopologyGraph([span("o", "observer"), span("p1", "chatbot", {}, "o"), span("p2", "chatbot", {}, "o")], ["observer", "chatbot"], new Map());
    const edge = graph.edges.find((e) => e.from === "observer");
    expect(edge?.kind).toBe("poll");
    expect(edge?.count).toBe(2);
  });

  it("yields an empty graph for no spans rather than an invented one", () => {
    const graph = buildTopologyGraph([], ["chatbot"], new Map());
    expect(graph.nodes).toEqual([]);
    expect(graph.edges).toEqual([]);
  });

  it("draws no edge for loopback egress", () => {
    const graph = buildTopologyGraph([span("o", "observer", { "server.address": "127.0.0.1:8001" })], ["observer"], new Map());
    expect(graph.edges).toEqual([]);
  });

  it("collapses a mesh service named for an agent onto the agent node", () => {
    const graph = buildTopologyGraph([span("a", "chatbot", { "server.address": "demo-chatbot-mesh-rag0" })], ["chatbot", "rag0"], new Map([["demo-chatbot-mesh-rag0", "rag0"]]));
    expect(new Map(graph.nodes.map((n) => [n.id, n.kind])).get("rag0")).toBe("agent");
    expect(graph.edges[0].to).toBe("rag0");
  });

  it("makes the Docker host's services host nodes and reads net.peer.name", () => {
    const graph = buildTopologyGraph(
      [span("a", "chatbot", { "server.address": "host.docker.internal:11434" }), span("b", "chatbot", { "net.peer.name": "dolt" })],
      ["chatbot"],
      new Map([["dolt", "dolt"]]),
    );
    const kinds = new Map(graph.nodes.map((n) => [n.id, n.kind]));
    expect(kinds.get("host.docker.internal")).toBe("host");
    expect(kinds.get("dolt")).toBe("service");
    expect(egressByAgent(graph).get("chatbot")).toEqual(["host.docker.internal", "dolt"]);
  });

  it("seeds the deployed inventory so a quiet unit is present, not missing", () => {
    const graph = buildTopologyGraph([], ["chatbot"], new Map(), [
      { id: "exporter", kind: "agent" },
      { id: "dolt", kind: "service" },
    ]);
    expect(graph.nodes.map((n) => n.id).sort()).toEqual(["dolt", "exporter"]);
    expect(graph.edges).toEqual([]);
  });

  it("derives the recorded trace's edges from the kit TraceModel", () => {
    const model = toModel(fixtures["/query/traces/{trace_id}"]);
    const graph = buildTopologyGraph(model.spans, ["chatbot", "knowledge0"], new Map());
    const edge = (from: string, to: string) => graph.edges.find((e) => e.from === from && e.to === to);
    expect(edge("chatbot", "knowledge0")?.kind).toBe("mesh");
    expect(edge("chatbot", "api.cohere.com")?.kind).toBe("egress");
    expect(edge("chatbot", "host.docker.internal")?.kind).toBe("egress");
  });
});

describe("trace selection (cohere-demo GH-425)", () => {
  const summary = (traceId: string, rootService: string, spanCount: number): TraceSummary => ({ traceId, rootService, rootSpanName: "", spanCount, startTime: "", durationMs: 0 });

  it("prefers root diversity after three picks, caps spans, and always adds the largest eligible", () => {
    const picked = selectTopologyTraces([
      summary("t1", "chatbot", 10),
      summary("t2", "chatbot", 12),
      summary("t3", "chatbot", 8),
      summary("t4", "chatbot", 9),
      summary("t5", "rag0", 5),
      summary("t6", "exporter", 400),
      summary("t7", "chatbot", 50),
      summary("", "collector", 3),
    ]);
    expect(picked).toEqual(["t1", "t2", "t3", "t5", "t7"]);
  });

  it("stops at five diverse picks and does not repeat the largest", () => {
    const picked = selectTopologyTraces(["a", "b", "c", "d", "e", "f"].map((root, index) => summary(`t${index}`, root, 100 - index)));
    expect(picked).toEqual(["t0", "t1", "t2", "t3", "t4"]);
  });

  it("selects nothing from an empty list", () => {
    expect(selectTopologyTraces([])).toEqual([]);
  });
});

describe("requestWalks (cohere-demo GH-417, GH-425)", () => {
  let clock = 0;
  const collectorSpan = (service: string, name: string, attributes: Record<string, unknown>): CollectorSpan => {
    clock += 1;
    return {
      span_id: `s${clock}`,
      name,
      service,
      start_time: new Date(clock * 1000).toISOString(),
      end_time: new Date(clock * 1000 + 10).toISOString(),
      attributes: Object.entries(attributes).map(([Key, Value]) => ({ Key, Value: { Type: "STRING", Value } })),
    };
  };
  const trace = (spans: CollectorSpan[]) => toModel({ trace_id: `t${clock}`, spans, span_count: spans.length });

  it("keeps the richest walk per service and orders steps by iteration", () => {
    const recent = trace([
      collectorSpan("chatbot", "execute_tool rag_query", { "command.name": "rag_query", "command.signal": "QueryResponded", iteration: 20 }),
      collectorSpan("chatbot", "execute_tool embed_query", { "command.name": "embed_query", "command.signal": "QueryEmbedded", iteration: 1 }),
      collectorSpan("rag0", "execute_tool read_documents", { "command.name": "read_documents", "command.signal": "DocumentsRead", iteration: 3 }),
      collectorSpan("chatbot", "invoke_agent machine_request", { "run.final_state": "Served" }),
    ]);
    const older = trace([
      collectorSpan("chatbot", "execute_tool old_command", { "command.name": "old_command", "command.signal": "Old", iteration: 1 }),
      collectorSpan("exporter", "execute_tool render_pdf", { "command.name": "render_pdf", "command.signal": "PdfRendered", iteration: 2 }),
    ]);
    const walks = requestWalks([recent, older]);
    expect([...walks.keys()].sort()).toEqual(["chatbot", "exporter", "rag0"]);
    expect(walks.get("chatbot")?.steps.map((step) => step.command)).toEqual(["embed_query", "rag_query"]);
    expect(walks.get("exporter")?.steps).toEqual([{ iteration: 2, command: "render_pdf", signal: "PdfRendered" }]);
  });

  it("yields nothing for traces without execute_tool spans", () => {
    expect(requestWalks([trace([collectorSpan("chatbot", "machine_request chat", {})])]).size).toBe(0);
    expect(requestWalks([]).size).toBe(0);
  });
});

describe("service list grouping (agentic-wiki-mesh, chatbot-mesh)", () => {
  const base: FleetData = { agents: [], pods: [], deployments: [], services: [], podMetrics: [] };

  it("matches the Service by its metadata component label (wiki-mesh GH-105)", () => {
    const groups = serviceGroups({
      ...base,
      deployments: [{ metadata: { name: "chatbot", labels: { "app.kubernetes.io/component": "chatbot" } } }],
      services: [{ metadata: { name: "chatbot-svc", labels: { "app.kubernetes.io/component": "chatbot" } } }],
      pods: [{ metadata: { name: "chatbot-0", labels: { "app.kubernetes.io/component": "chatbot" } } }],
    });
    expect(groups).toEqual([{ deployment: "chatbot", role: undefined, services: ["chatbot-svc"], pods: ["chatbot-0"] }]);
  });

  it("matches the Service by its selector component (chatbot-mesh)", () => {
    const groups = serviceGroups({
      ...base,
      deployments: [{ metadata: { name: "chatbot", labels: { "app.kubernetes.io/component": "chatbot" } } }],
      services: [{ metadata: { name: "chatbot" }, spec: { selector: { "app.kubernetes.io/component": "chatbot" } } }],
    });
    expect(groups[0].services).toEqual(["chatbot"]);
  });

  it("falls back to the deployment selector and reads the role annotation", () => {
    const groups = serviceGroups(fleetData(fixtures["/monitor/fleet"]), "mesh.example/role");
    expect(groups).toEqual([{ deployment: "demo-chatbot-mesh-chatbot", role: "chat front door", services: ["demo-chatbot-mesh-chatbot"], pods: ["chatbot-7d9c8b6f4-x2k9p"] }]);
  });
});

describe("buildTopologyState", () => {
  it("joins graph, hover info, egress, and walks from one read", () => {
    const data = fleetData(fixtures["/monitor/fleet"]);
    const state = buildTopologyState(data, [toModel(fixtures["/query/traces/{trace_id}"])], "mesh.example/role");
    const kinds = new Map(state.graph.nodes.map((node) => [node.id, node.kind]));
    // chatbot serves the monitor in the fleet, so it is an agent.
    expect(kinds.get("chatbot")).toBe("agent");
    expect(state.info.get("demo-chatbot-mesh-chatbot")?.role).toBe("chat front door");
    expect(state.info.get("chatbot")).toEqual({ pod: "chatbot-7d9c8b6f4-x2k9p", resources: "cpu 2500u · mem 8704Ki" });
    expect(state.egress.get("chatbot")).toContain("api.cohere.com");
    expect(state.walks.get("chatbot")?.steps.length).toBeGreaterThan(0);
  });
});

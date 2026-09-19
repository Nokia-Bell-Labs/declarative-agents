import { useEffect, useState } from "react";
import { useKitClient } from "../../client/context";
import { metricsByPod, objectName, type FleetData, type KubernetesObject, type ResourceUsage } from "../../api/fleetApi";
import type { ServiceWalk, TraceModel } from "../../api/traceApi";
import { commonNamePrefix, roleOf, shortUnitName } from "./names";
import { buildTopologyGraph, egressByAgent, type TopologyGraph, type TopologyNode } from "./topologyGraph";
import { fetchTopologyTraces, requestWalks, TOPOLOGY_TRACE_PAGE } from "./traceReads";

// What a node's hover says (GH-376): the unit's declared role, its pod, and
// live resources where known.
export interface TopologyNodeInfo {
  role?: string;
  pod?: string;
  resources?: string;
}

// TopologyState is everything the figure and the agent cards share from one
// trace read: the graph, each node's hover info, each unit's observed egress,
// and each service's richest request walk.
export interface TopologyState {
  graph: TopologyGraph;
  info: Map<string, TopologyNodeInfo>;
  egress: Map<string, string[]>;
  walks: Map<string, ServiceWalk>;
}

// The topology is deriving, derived, or degraded to the service list with a
// stated reason (no traces read, or the backend unreachable).
export type TopologyRead = { status: "deriving" } | { status: "ready"; state: TopologyState } | { status: "no-traces"; reason: string };

const COMPONENT_LABEL = "app.kubernetes.io/component";
export const NO_TRACES_REASON = "no recent traces to derive the graph from";

function shortOf(object: KubernetesObject, fallback: string, prefix: string): string {
  return shortUnitName(objectName(object, fallback), prefix);
}

// buildTopologyState derives the shared state from the fleet and the read
// traces. roleAnnotation names the application's role annotation key.
export function buildTopologyState(data: FleetData, traces: TraceModel[], roleAnnotation?: string, metrics: Record<string, ResourceUsage> = metricsByPod(data.podMetrics)): TopologyState {
  const names = data.deployments.map((deployment) => objectName(deployment, "deployment"));
  const prefix = commonNamePrefix(names);
  // Agents are the units that serve the monitor: the deployments' component
  // labels and short names, plus every pod the fleet read a monitor from.
  const agents = [
    ...data.deployments.map((deployment) => deployment.metadata?.labels?.[COMPONENT_LABEL]).filter((component): component is string => Boolean(component)),
    ...names.map((name) => name.slice(prefix.length)).filter(Boolean),
    ...data.agents.map((agent) => shortUnitName(agent.name ?? "", prefix)).filter(Boolean),
  ];
  const meshServices = new Map<string, string>();
  for (const service of data.services) {
    const name = objectName(service, "service");
    const short = name.startsWith(prefix) && name.length > prefix.length ? name.slice(prefix.length) : name;
    for (const spelling of [name, `${name}.default`, `${name}.default.svc`]) meshServices.set(spelling, short);
  }
  const agentShorts = new Set(names.map((name) => shortUnitName(name, prefix)));
  const inventory: TopologyNode[] = [
    ...[...agentShorts].map((id) => ({ id, kind: "agent" as const })),
    ...[...new Set(meshServices.values())].filter((short) => !agentShorts.has(short)).map((id) => ({ id, kind: "service" as const })),
  ];
  const spans = traces.flatMap((trace) => trace.spans);
  const graph = buildTopologyGraph(spans, agents, meshServices, inventory);

  const info = new Map<string, TopologyNodeInfo>();
  const entry = (short: string) => {
    const held = info.get(short) ?? {};
    info.set(short, held);
    return held;
  };
  for (const object of [...data.deployments, ...data.services]) {
    const role = roleOf(object, roleAnnotation);
    if (role) entry(shortOf(object, "object", prefix)).role = role;
  }
  for (const pod of data.pods) {
    const podName = objectName(pod, "pod");
    const node = entry(shortUnitName(podName, prefix));
    node.pod = podName;
    const usage = metrics[podName];
    if (usage) node.resources = `cpu ${usage.cpu ?? "?"} · mem ${usage.memory ?? "?"}`;
  }

  return { graph, info, egress: egressByAgent(graph), walks: requestWalks(traces) };
}

export interface UseTopologyOptions {
  // The agent ui.yaml names as the trace backend (srd004 R2.4).
  backend: string;
  roleAnnotation?: string;
  pageSize?: number;
}

// useTopology re-derives the topology whenever the fleet data changes. A
// re-derivation keeps the previous figure on screen until the new one lands.
export function useTopology(data: FleetData, { backend, roleAnnotation, pageSize = TOPOLOGY_TRACE_PAGE }: UseTopologyOptions): TopologyRead {
  const client = useKitClient();
  const [read, setRead] = useState<TopologyRead>({ status: "deriving" });
  useEffect(() => {
    let active = true;
    void fetchTopologyTraces(client, backend, pageSize).then((traces) => {
      if (!active) return;
      if (traces.status === "unavailable") setRead({ status: "no-traces", reason: `trace backend ${backend} unavailable: ${traces.reason}` });
      else if (traces.traces.every((trace) => trace.spans.length === 0)) setRead({ status: "no-traces", reason: NO_TRACES_REASON });
      else setRead({ status: "ready", state: buildTopologyState(data, traces.traces, roleAnnotation) });
    });
    return () => {
      active = false;
    };
  }, [client, backend, pageSize, data, roleAnnotation]);
  return read;
}

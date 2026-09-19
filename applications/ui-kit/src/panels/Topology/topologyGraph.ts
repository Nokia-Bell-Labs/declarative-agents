// Topology derived from observed telemetry (cohere-demo GH-359). The graph is
// built from two observations and nothing static: a span whose parent belongs
// to another service is an edge between those services, and a span's
// server.address (or net.peer.name) is an edge to whatever the address names,
// an in-mesh backing service or an external peer. The observer's own polling
// edges carry their own kind so the data plane reads apart from the watcher.
// Pure data in, nodes and weighted edges out.

// TopologySpan is the slice of a trace span the derivation reads. The kit's
// TraceSpan (traceApi) satisfies it, so models from fetchTraces feed it as is.
export interface TopologySpan {
  id?: string;
  parentId?: string;
  service?: string;
  attributes?: Record<string, unknown>;
}

export type TopologyNodeKind = "host" | "agent" | "service" | "external";

export interface TopologyNode {
  id: string;
  kind: TopologyNodeKind;
}

export interface TopologyEdge {
  from: string;
  to: string;
  count: number;
  kind: "mesh" | "egress" | "poll";
}

export interface TopologyGraph {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
}

// The service whose outbound edges are the watcher's polls, not data flow.
export const OBSERVER_SERVICE = "observer";

function attributeValue(span: TopologySpan, key: string): string | undefined {
  const value = span.attributes?.[key];
  return value === undefined || value === null || value === "" ? undefined : String(value);
}

// serviceForAddress maps a server.address host to a mesh service's short name
// when the address names one, so a unit's store call renders as an edge to
// that store rather than to an opaque DNS name. Anything unmatched is an
// external peer, labeled by host.
function serviceForAddress(address: string, meshServices: Map<string, string>): TopologyNode | undefined {
  const host = address.replace(/^https?:\/\//, "").split("/")[0].split(":")[0];
  // Loopback is intra-pod plumbing (an agent talking to its own sidecar).
  if (host === "127.0.0.1" || host === "localhost") return undefined;
  // The Docker host's own services (a local model server) are infrastructure
  // on the near side of the mesh, drawn in their own column (GH-367).
  if (host === "host.docker.internal" || host.endsWith(".docker.internal")) return { id: host, kind: "host" };
  const short = meshServices.get(host);
  if (short) return { id: short, kind: "service" };
  return { id: host, kind: "external" };
}

// buildTopologyGraph derives the graph. agents names the services that are
// agents; meshServices maps every DNS spelling of a mesh Service to its short
// name; inventory seeds the deployed units so a quiet one is present (dimmed
// by the view) rather than missing (GH-369).
export function buildTopologyGraph(
  spans: TopologySpan[],
  agents: string[],
  meshServices: Map<string, string>,
  inventory: TopologyNode[] = [],
): TopologyGraph {
  const serviceOf = new Map<string, string>();
  for (const span of spans) {
    if (span.id && span.service) serviceOf.set(span.id, span.service);
  }

  const nodes = new Map<string, TopologyNode>();
  const agentSet = new Set(agents);
  const edges = new Map<string, TopologyEdge>();
  const addNode = (id: string, kind: TopologyNodeKind) => {
    if (!nodes.has(id)) nodes.set(id, { id, kind });
  };
  const addEdge = (from: string, to: string, kind: TopologyEdge["kind"]) => {
    const key = `${from} ${to} ${kind}`;
    const edge = edges.get(key);
    if (edge) edge.count += 1;
    else edges.set(key, { from, to, count: 1, kind });
  };
  for (const node of inventory) addNode(node.id, node.kind);

  for (const span of spans) {
    const service = span.service;
    if (!service) continue;
    addNode(service, agentSet.has(service) ? "agent" : "service");

    const parentService = span.parentId ? serviceOf.get(span.parentId) : undefined;
    if (parentService && parentService !== service) {
      addNode(parentService, agentSet.has(parentService) ? "agent" : "service");
      addEdge(parentService, service, parentService === OBSERVER_SERVICE ? "poll" : "mesh");
    }

    const address = attributeValue(span, "server.address") ?? attributeValue(span, "net.peer.name");
    if (!address) continue;
    const target = serviceForAddress(address, meshServices);
    if (!target || target.id === service) continue;
    // A mesh Service named for an agent is that agent's boundary, so the edge
    // lands on the agent node rather than a duplicate.
    addNode(target.id, agentSet.has(target.id) ? "agent" : target.kind);
    addEdge(service, target.id, service === OBSERVER_SERVICE ? "poll" : "egress");
  }

  return { nodes: [...nodes.values()], edges: [...edges.values()] };
}

// egressByAgent lists each unit's observed egress peers, the same edges the
// figure draws, so an agent card can say which providers it calls (GH-376).
export function egressByAgent(graph: TopologyGraph): Map<string, string[]> {
  const egress = new Map<string, string[]>();
  for (const edge of graph.edges) {
    if (edge.kind !== "egress") continue;
    egress.set(edge.from, [...(egress.get(edge.from) ?? []), edge.to]);
  }
  return egress;
}

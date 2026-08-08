export const routes = {
  fleet: "/monitor/fleet",
  state: "/monitor/state",
} as const;

export type KubernetesObject = {
  name?: string;
  metadata?: {
    name?: string;
    labels?: Record<string, string>;
  };
  spec?: {
    selector?: Record<string, string>;
  };
};

export type PodMetric = KubernetesObject & {
  cpu?: string;
  memory?: string;
  containers?: Array<{ usage?: { cpu?: string; memory?: string } }>;
};

export type FleetAgent = {
  name?: string;
  reachable?: boolean;
  state?: string | { current_state?: string };
  machine?: {
    purpose?: string;
    states?: unknown[];
    transitions?: unknown[];
  };
  tools?: unknown[] | { tools?: unknown[] };
  events?: Array<{
    signal?: string;
    name?: string;
    timestamp?: string;
    time?: string;
  }>;
};

type CommandLabel = {
  available?: boolean;
  output?: {
    mapped?: Record<string, unknown>;
    [key: string]: unknown;
  };
};

// The four monitor fan-in joins, one per endpoint the observer reads from every
// discovered pod (GH-1319).
const fanInLabels = {
  machine: "agent_machine_fanin",
  state: "agent_state_fanin",
  tools: "agent_tools_fanin",
  events: "agent_events_fanin",
} as const;

// A fan-in item read its endpoint when it carries this signal. The join cannot
// answer this: the fan-in lists CommandError in continue_on so one unreachable
// agent never halts the pass, which makes the join count every dispatched item
// as succeeded. Reachability is therefore per item, not per join.
const monitorReadSignal = "AgentMonitorRead";

// FanInItem is one join outcome: the pod that was dispatched and that pod's
// result. structured_output is the REST word's parsed output, whose body is the
// monitor payload.
type FanInItem = {
  input?: KubernetesObject;
  result?: {
    signal?: string;
    structured_output?: { body?: Record<string, unknown> };
  };
};

export type FleetResponse = {
  labels?: Record<string, CommandLabel>;
};

export type FleetData = {
  agents: FleetAgent[];
  pods: KubernetesObject[];
  deployments: KubernetesObject[];
  services: KubernetesObject[];
  podMetrics: PodMetric[];
};

export type ResourceUsage = { cpu?: string; memory?: string };

function labelArray<T>(labels: Record<string, CommandLabel>, label: string, field: string): T[] {
  const entry = labels[label];
  if (!entry || entry.available === false || !entry.output) return [];
  const output = entry.output.mapped ?? entry.output;
  const value = output[field];
  return Array.isArray(value) ? (value as T[]) : [];
}

function fanInItems(labels: Record<string, CommandLabel>, label: string): FanInItem[] {
  const entry = labels[label];
  if (!entry || entry.available === false || !entry.output) return [];
  const items = entry.output.items;
  return Array.isArray(items) ? (items as FanInItem[]) : [];
}

function itemBody(item: FanInItem): Record<string, unknown> | undefined {
  return item.result?.structured_output?.body;
}

// podName keys an agent by the pod that was polled, so a card's resource lookup
// matches the per-pod metrics the fleet also reports (srd008 R3.5).
function podName(item: FanInItem): string {
  const input = item.input;
  return input?.metadata?.name ?? input?.name ?? "";
}

// runState reads the card's state string out of the monitor state view, which
// nests the run snapshot rather than exposing a flat current_state.
function runState(body?: Record<string, unknown>): string | undefined {
  return (body?.run as { state?: string } | undefined)?.state;
}

function bodyArray(body: Record<string, unknown> | undefined, field: string): unknown[] {
  const value = body?.[field];
  return Array.isArray(value) ? value : [];
}

// fanInReaders maps each join onto the agent field it fills. The endpoint shapes
// come from the agent-core monitor views: machine is flat, state nests run,
// tools carries tools, and events carries recent_events.
const fanInReaders: Array<[string, (agent: FleetAgent, item: FanInItem) => void]> = [
  [fanInLabels.machine, (agent, item) => {
    agent.machine = itemBody(item) as FleetAgent["machine"];
  }],
  [fanInLabels.state, (agent, item) => {
    agent.reachable = item.result?.signal === monitorReadSignal;
    agent.state = runState(itemBody(item));
  }],
  [fanInLabels.tools, (agent, item) => {
    agent.tools = bodyArray(itemBody(item), "tools");
  }],
  [fanInLabels.events, (agent, item) => {
    agent.events = bodyArray(itemBody(item), "recent_events") as FleetAgent["events"];
  }],
];

// fleetAgents zips the four per-endpoint joins into one card per discovered pod.
// Each join holds one item per pod, so the pod name joins them. A pod that
// appears in no join contributes nothing, and an absent or unavailable join
// leaves its fields empty rather than throwing.
function fleetAgents(labels: Record<string, CommandLabel>): FleetAgent[] {
  const agents = new Map<string, FleetAgent>();
  for (const [label, read] of fanInReaders) {
    for (const item of fanInItems(labels, label)) {
      const name = podName(item);
      if (!name) continue;
      let agent = agents.get(name);
      if (!agent) {
        agent = { name, reachable: false };
        agents.set(name, agent);
      }
      read(agent, item);
    }
  }
  return [...agents.values()];
}

export function fleetData(response: FleetResponse): FleetData {
  const labels = response.labels ?? {};
  return {
    agents: fleetAgents(labels),
    pods: labelArray(labels, "discover_mesh_pods", "pods"),
    deployments: labelArray(labels, "list_mesh_deployments", "deployments"),
    services: labelArray(labels, "list_mesh_services", "services"),
    podMetrics: labelArray(labels, "poll_pod_metrics", "pod_metrics"),
  };
}

const cpuQuantityFactors: Record<string, number> = {
  n: 1,
  u: 1_000,
  m: 1_000_000,
  "": 1_000_000_000,
};

const memoryQuantityFactors: Record<string, number> = {
  "": 1,
  k: 1_000,
  K: 1_000,
  M: 1_000_000,
  G: 1_000_000_000,
  T: 1_000_000_000_000,
  Ki: 1_024,
  Mi: 1_048_576,
  Gi: 1_073_741_824,
  Ti: 1_099_511_627_776,
};

function parseQuantity(value: string, factors: Record<string, number>): number | undefined {
  const match = /^([0-9]+(?:\.[0-9]+)?)([A-Za-z]*)$/.exec(value.trim());
  if (!match) return undefined;
  const factor = factors[match[2]];
  if (factor === undefined) return undefined;
  const parsed = Number(match[1]) * factor;
  return Number.isFinite(parsed) ? parsed : undefined;
}

function sumQuantities(
  values: Array<string | undefined>,
  factors: Record<string, number>,
): number | undefined {
  if (!values.length) return undefined;
  let total = 0;
  for (const value of values) {
    if (!value) return undefined;
    const parsed = parseQuantity(value, factors);
    if (parsed === undefined) return undefined;
    total += parsed;
  }
  return total;
}

function formatCPU(nanocores: number | undefined): string | undefined {
  if (nanocores === undefined) return undefined;
  for (const [factor, suffix] of [[1_000_000_000, ""], [1_000_000, "m"], [1_000, "u"]] as const) {
    if (nanocores >= factor && nanocores % factor === 0) {
      return `${nanocores / factor}${suffix}`;
    }
  }
  return `${Math.round(nanocores)}n`;
}

function formatMemory(bytes: number | undefined): string | undefined {
  if (bytes === undefined) return undefined;
  for (const [factor, suffix] of [
    [1_099_511_627_776, "Ti"], [1_073_741_824, "Gi"],
    [1_048_576, "Mi"], [1_024, "Ki"],
    [1_000_000_000_000, "T"], [1_000_000_000, "G"],
    [1_000_000, "M"], [1_000, "K"],
  ] as const) {
    if (bytes >= factor && bytes % factor === 0) return `${bytes / factor}${suffix}`;
  }
  return `${Math.round(bytes)}`;
}

export function metricsByPod(items: PodMetric[]): Record<string, ResourceUsage> {
  const result: Record<string, ResourceUsage> = {};
  for (const item of items) {
    const name = item.name ?? item.metadata?.name;
    if (!name) continue;
    if (item.cpu || item.memory) {
      result[name] = { cpu: item.cpu, memory: item.memory };
      continue;
    }
    const containers = item.containers ?? [];
    result[name] = {
      cpu: formatCPU(sumQuantities(
        containers.map((container) => container.usage?.cpu), cpuQuantityFactors,
      )),
      memory: formatMemory(sumQuantities(
        containers.map((container) => container.usage?.memory), memoryQuantityFactors,
      )),
    };
  }
  return result;
}

export function objectName(value: KubernetesObject, fallback: string): string {
  return value.name ?? value.metadata?.name ?? fallback;
}

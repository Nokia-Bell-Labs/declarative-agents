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

// Keep consuming the existing command_state envelope. In particular, this does
// not synthesize the per-agent monitor fan-in tracked separately by GH-1319.
export function fleetData(response: FleetResponse): FleetData {
  const labels = response.labels ?? {};
  return {
    agents: labelArray(labels, "poll_agent_monitors", "agents"),
    pods: labelArray(labels, "discover_mesh_pods", "pods"),
    deployments: labelArray(labels, "list_mesh_deployments", "deployments"),
    services: labelArray(labels, "list_mesh_services", "services"),
    podMetrics: labelArray(labels, "poll_pod_metrics", "pod_metrics"),
  };
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
    let cpu: string | undefined;
    let memory: string | undefined;
    for (const container of item.containers ?? []) {
      cpu ??= container.usage?.cpu;
      memory ??= container.usage?.memory;
    }
    result[name] = { cpu, memory };
  }
  return result;
}

export function objectName(value: KubernetesObject, fallback: string): string {
  return value.name ?? value.metadata?.name ?? fallback;
}

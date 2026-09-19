import type { ComponentType } from "react";
import { CONTRACT_ENDPOINTS, type ContractEndpoint } from "../contract";
import type { MonitoredAgent } from "../api/monitorApi";

// A panel is the reuse unit (srd004 R4): a component plus a manifest naming
// what it reads. monitored_agents is either a fixed list or "declared", meaning
// the agents the application's ui.yaml lists.
export interface PanelManifest {
  id: string;
  title: string;
  route: string;
  required_endpoints: ContractEndpoint[];
  monitored_agents: string[] | "declared";
}

// PanelProps is what a shell passes every mounted panel: the ui.yaml panel
// config, the declared monitored agents, and the declared trace backend.
export interface PanelProps<Config = Record<string, unknown>> {
  config?: Config;
  monitoredAgents: MonitoredAgent[];
  traceBackend?: string;
}

export interface KitPanel<Config = Record<string, unknown>> {
  manifest: PanelManifest;
  component: ComponentType<PanelProps<Config>>;
}

// definePanel pairs a component with its manifest and rejects a manifest that
// names an endpoint outside the presentation contract (srd004 R4.2).
export function definePanel<Config = Record<string, unknown>>(
  manifest: PanelManifest,
  component: ComponentType<PanelProps<Config>>,
): KitPanel<Config> {
  const outside = manifest.required_endpoints.filter((endpoint) => !(CONTRACT_ENDPOINTS as readonly string[]).includes(endpoint));
  if (outside.length > 0) {
    throw new Error(`panel ${manifest.id} requires endpoints outside the presentation contract: ${outside.join(", ")}`);
  }
  return { manifest, component };
}

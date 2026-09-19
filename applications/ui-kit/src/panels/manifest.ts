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
  // An agent name, or a same-origin path prefix starting with "/" (srd004
  // R2.4; see traceQueryPath).
  traceBackend?: string;
}

// KitPanel erases the author's config type: a shell passes the ui.yaml config
// as a plain map, and each mount narrows it.
export interface KitPanel {
  manifest: PanelManifest;
  component: ComponentType<PanelProps>;
}

// definePanel pairs a component with its manifest and rejects a manifest that
// names an endpoint outside the presentation contract (srd004 R4.2).
export function definePanel<Config = Record<string, unknown>>(
  manifest: PanelManifest,
  component: ComponentType<PanelProps<Config>>,
): KitPanel {
  const outside = manifest.required_endpoints.filter((endpoint) => !(CONTRACT_ENDPOINTS as readonly string[]).includes(endpoint));
  if (outside.length > 0) {
    throw new Error(`panel ${manifest.id} requires endpoints outside the presentation contract: ${outside.join(", ")}`);
  }
  return { manifest, component: component as ComponentType<PanelProps> };
}

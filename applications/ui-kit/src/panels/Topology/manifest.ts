import type { PanelManifest } from "../manifest";

export const topologyManifest: PanelManifest = {
  id: "topology",
  title: "Topology",
  route: "/topology",
  required_endpoints: ["/monitor/fleet", "/query/traces", "/query/traces/{trace_id}"],
  monitored_agents: [],
};

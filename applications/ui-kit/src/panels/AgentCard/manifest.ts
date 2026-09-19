import type { PanelManifest } from "../manifest";

export const agentCardManifest: PanelManifest = {
  id: "agent-card",
  title: "Agents",
  route: "/agents",
  required_endpoints: ["/monitor/fleet"],
  monitored_agents: [],
};

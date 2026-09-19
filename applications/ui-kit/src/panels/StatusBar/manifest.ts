import type { PanelManifest } from "../manifest";

export const statusBarManifest: PanelManifest = {
  id: "status-bar",
  title: "Status",
  route: "/status",
  required_endpoints: ["/monitor/fleet", "/monitor/state"],
  monitored_agents: [],
};

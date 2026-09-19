import type { PanelManifest } from "../manifest";

export const machineViewManifest: PanelManifest = {
  id: "machine-view",
  title: "Machines",
  route: "/machines",
  required_endpoints: ["/monitor/machines", "/monitor/state", "/monitor/tools/declared", "/monitor/events/stream"],
  monitored_agents: "declared",
};

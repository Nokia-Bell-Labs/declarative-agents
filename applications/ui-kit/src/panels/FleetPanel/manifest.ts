import type { PanelManifest } from "../manifest";

export const fleetPanelManifest: PanelManifest = {
  id: "fleet",
  title: "Fleet",
  route: "/fleet",
  required_endpoints: ["/monitor/state", "/monitor/events/stream"],
  monitored_agents: "declared",
};

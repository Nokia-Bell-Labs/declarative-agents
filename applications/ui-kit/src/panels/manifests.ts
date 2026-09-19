import type { PanelManifest } from "./manifest";
import { agentCardManifest } from "./AgentCard/manifest";
import { fleetPanelManifest } from "./FleetPanel/manifest";
import { machineViewManifest } from "./MachineView/manifest";
import { statusBarManifest } from "./StatusBar/manifest";
import { topologyManifest } from "./Topology/manifest";
import { tracePanelManifest } from "./TracePanel/manifest";

// The kit panel manifests without their components. The manifests are plain
// data, so the build-time ui.yaml plugin can check a kit panel's export in
// Node without loading React components or their stylesheets.
export const kitPanelManifests: PanelManifest[] = [
  statusBarManifest,
  fleetPanelManifest,
  tracePanelManifest,
  machineViewManifest,
  topologyManifest,
  agentCardManifest,
];

export const kitPanelManifestById: Record<string, PanelManifest> = Object.fromEntries(
  kitPanelManifests.map((manifest) => [manifest.id, manifest]),
);

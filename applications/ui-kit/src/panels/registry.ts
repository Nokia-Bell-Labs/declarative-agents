import type { KitPanel } from "./manifest";
import { agentCardPanel } from "./AgentCard";
import { fleetPanel } from "./FleetPanel";
import { machineViewPanel } from "./MachineView";
import { statusBarPanel } from "./StatusBar";
import { topologyPanel } from "./Topology";
import { tracePanel } from "./TracePanel";

// kitPanels is every panel the kit publishes (srd004 R4.4), keyed by id so an
// application registry reads {...kitPanelRegistry, ...domainPanels}.
export const kitPanels: KitPanel[] = [statusBarPanel, fleetPanel, tracePanel, machineViewPanel, topologyPanel, agentCardPanel];

export const kitPanelRegistry: Record<string, KitPanel> = Object.fromEntries(kitPanels.map((panel) => [panel.manifest.id, panel]));

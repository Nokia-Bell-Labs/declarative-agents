import type { KitPanel } from "./manifest";
import { fleetPanel } from "./FleetPanel";
import { statusBarPanel } from "./StatusBar";

// kitPanels is every panel the kit publishes (srd004 R4.4), keyed by id so an
// application registry reads {...kitPanelRegistry, ...domainPanels}.
export const kitPanels: KitPanel[] = [statusBarPanel, fleetPanel];

export const kitPanelRegistry: Record<string, KitPanel> = Object.fromEntries(kitPanels.map((panel) => [panel.manifest.id, panel]));

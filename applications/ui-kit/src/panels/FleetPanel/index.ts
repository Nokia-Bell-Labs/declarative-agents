import { definePanel } from "../manifest";
import { eventInWindow, FleetPanel, FleetPanelMount, type HighlightWindow } from "./FleetPanel";
import { fleetPanelManifest } from "./manifest";

export { eventInWindow, FleetPanel, FleetPanelMount, fleetPanelManifest, type HighlightWindow };
export const fleetPanel = definePanel(fleetPanelManifest, FleetPanelMount);

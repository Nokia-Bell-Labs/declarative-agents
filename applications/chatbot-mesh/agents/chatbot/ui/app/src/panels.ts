import type { PanelRegistry } from "@declarative-agents/ui-kit";
import ChatPanel from "./ChatPanel";
import ObservabilityPanel from "./ObservabilityPanel";
import ProvisioningPanel from "./ProvisioningPanel";

// The application's panels, keyed by the ui.yaml panel id. ui.yaml is the only
// route table: adding a panel is one ui.yaml entry plus one line here, and a Go
// test fails when a declared panel id has no entry (applications srd004 R7.4).
export const registry: PanelRegistry = {
  chat: ChatPanel,
  observability: ObservabilityPanel,
  provisioning: ProvisioningPanel,
};

// Public entry of @declarative-agents/ui-kit (srd004). KIT_VERSION tracks the
// package version; CONTRACT_VERSION is the presentation contract's major.
export const KIT_VERSION = "1.1.0";

export { CONTRACT_ENDPOINTS, CONTRACT_VERSION, MONITOR_PROXY_PREFIX, type ContractEndpoint } from "./contract";
export { createKitClient, HTTPError, proxyPath, type KitClient, type KitClientOptions, type ProxyResult } from "./client/client";
export { KitClientProvider, useKitClient } from "./client/context";
export * from "./api/monitorApi";
export * from "./api/traceApi";
export { MAX_MONITOR_EVENTS, useAgentMonitor, type AgentMonitor } from "./hooks/useAgentMonitor";
export { useTrace, useTraceList, useTraces } from "./hooks/useTrace";
export * from "./api/fleetApi";
export { EMPTY_FLEET, useFleet, type FleetSnapshot } from "./hooks/useFleet";
export * from "./api/turnActivity";
export { useTurnActivity } from "./hooks/useTurnActivity";
export { panelForPath, panelHref, splitPanelPath, staticHref, type PanelRoute, type PanelRouting } from "./shell/paths";
export { usePanelLocation, type PanelLocation } from "./shell/usePanelLocation";
export { Sidebar, type SidebarGroup, type SidebarProps } from "./shell/Sidebar";
export { PanelFrame } from "./shell/PanelFrame";
export {
  KIT_PACKAGE,
  routingFromConfig,
  shellTitle,
  TRACE_QUERY_SUFFIX,
  traceBackendFromConfig,
  validateUIConfig,
  type ShellRouting,
  type UIBranding,
  type UIConfig,
  type UIMonitoredAgent,
  type UIPanel,
  type UIRoute,
  type UISidebar,
  type UISidebarGroup,
  type UITraceBackend,
} from "./shell/uiConfig";
export { AppShell, type AppShellProps, type PanelRegistry } from "./shell/AppShell";
export { useAgentPresence, type AgentPresence } from "./shell/useAgentPresence";
export { definePanel, type KitPanel, type PanelManifest, type PanelProps } from "./panels/manifest";
export * from "./panels/StatusBar";
export * from "./panels/FleetPanel";
export * from "./panels/MachineView";
export * from "./panels/TracePanel";
export * from "./panels/Topology";
export * from "./panels/AgentCard";
export { kitPanelRegistry, kitPanels } from "./panels/registry";
export { kitPanelManifestById, kitPanelManifests } from "./panels/manifests";
export { canonicalPanelPath, navigateTo, PanelLink, ShellRoutingProvider, usePanelPath } from "./shell/subPath";

// Public entry of @declarative-agents/ui-kit (srd004). KIT_VERSION tracks the
// package version; CONTRACT_VERSION is the presentation contract's major.
export const KIT_VERSION = "0.1.0";

export { CONTRACT_ENDPOINTS, CONTRACT_VERSION, MONITOR_PROXY_PREFIX, type ContractEndpoint } from "./contract";
export { createKitClient, HTTPError, proxyPath, type KitClient, type KitClientOptions, type ProxyResult } from "./client/client";
export { KitClientProvider, useKitClient } from "./client/context";
export * from "./api/monitorApi";
export * from "./api/traceApi";
export { MAX_MONITOR_EVENTS, useAgentMonitor, type AgentMonitor } from "./hooks/useAgentMonitor";
export { useTrace, useTraceList, useTraces } from "./hooks/useTrace";

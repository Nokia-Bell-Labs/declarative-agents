// The presentation contract (srd004 R1.1): the closed set of endpoints a kit
// panel may read. The owning SRD of each endpoint defines its response; the
// kit restates no schema beyond the fields its clients read.
export const CONTRACT_VERSION = 1;

export const CONTRACT_ENDPOINTS = [
  "/monitor/state",
  "/monitor/machines",
  "/monitor/tools/declared",
  "/monitor/events/stream",
  "/monitor/fleet",
  "/query/traces",
  "/query/traces/{trace_id}",
] as const;

export type ContractEndpoint = (typeof CONTRACT_ENDPOINTS)[number];

// The proxy every cross-agent read goes through (srd004 R2.1).
export const MONITOR_PROXY_PREFIX = "/monitor-proxy";

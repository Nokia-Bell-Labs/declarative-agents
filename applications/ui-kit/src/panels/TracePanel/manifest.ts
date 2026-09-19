import type { PanelManifest } from "../manifest";

// TracePanel reads the trace backend's list and one trace by id, both through
// the proxy of the backend ui.yaml declares (srd004 R2.4); it monitors no agent.
export const tracePanelManifest: PanelManifest = {
  id: "trace",
  title: "Traces",
  route: "/traces",
  required_endpoints: ["/query/traces", "/query/traces/{trace_id}"],
  monitored_agents: [],
};

import type { ContractEndpoint } from "../contract";
import type { DeclaredTool, StateSnapshot } from "../api/monitorApi";
import type { FleetResponse } from "../api/fleetApi";
import type { CollectorTrace, CollectorTraceList } from "../api/traceApi";
import monitorEventsStream from "./monitor-events-stream.json";
import monitorFleet from "./monitor-fleet.json";
import monitorMachines from "./monitor-machines.json";
import monitorState from "./monitor-state.json";
import monitorToolsDeclared from "./monitor-tools-declared.json";
import queryTrace from "./query-trace.json";
import queryTraces from "./query-traces.json";

// Recorded responses, one per presentation-contract endpoint (srd004 R3.2).
// They are the consumer contract tests: every client and panel runs against
// them. The machines and trace fixtures are live recordings from the
// agentic-wiki-mesh demo (collector and chatbot at 2026-09-19).

export interface SSEFrame {
  event: string;
  data: string;
}

export const fixtures = {
  "/monitor/state": monitorState as StateSnapshot,
  "/monitor/machines": monitorMachines as unknown[],
  "/monitor/tools/declared": monitorToolsDeclared as DeclaredTool[],
  "/monitor/events/stream": monitorEventsStream as SSEFrame[],
  "/monitor/fleet": monitorFleet as FleetResponse,
  "/query/traces": queryTraces as CollectorTraceList,
  "/query/traces/{trace_id}": queryTrace as CollectorTrace,
} satisfies Record<ContractEndpoint, unknown>;

// fixtureFetch answers kit requests from the fixtures, so tests and panel
// previews run a KitClient without a backend. Proxied paths resolve to the
// contract path behind the proxy; agents listed in absentAgents answer 404.
export function fixtureFetch(options: { absentAgents?: string[]; overrides?: Record<string, unknown> } = {}): typeof fetch {
  const absent = new Set(options.absentAgents ?? []);
  return async (input) => {
    const url = new URL(String(input), "http://fixture.invalid");
    let path = url.pathname;
    const proxied = path.match(/^\/monitor-proxy\/([^/]+)(\/.*)$/);
    if (proxied) {
      if (absent.has(decodeURIComponent(proxied[1]))) return new Response("not deployed", { status: 404 });
      path = proxied[2];
    }
    if (options.overrides && path in options.overrides) return Response.json(options.overrides[path]);
    if (path.startsWith("/query/traces/")) return Response.json(fixtures["/query/traces/{trace_id}"]);
    const body = (fixtures as Record<string, unknown>)[path];
    return body === undefined ? new Response("no fixture", { status: 404 }) : Response.json(body);
  };
}

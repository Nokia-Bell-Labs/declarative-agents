import type { KitClient } from "../../client/client";
import { fetchTraceList, fetchTraces, type ServiceWalk, type TraceModel, type TraceSummary } from "../../api/traceApi";

// The trace reads the topology is derived from (cohere-demo GH-359, GH-417,
// GH-425), rebound from the retired /trace-proxy onto the kit trace client and
// the declared trace backend (srd004 R2.3, R2.4).

// How many recent traces the selection looks at, and the span cap above which
// a trace is skipped: one giant lifecycle trace would blow the proxy's
// response cap without adding an edge the small ones lack.
export const TOPOLOGY_TRACE_PAGE = 15;
export const TOPOLOGY_MAX_SPANS = 300;
const DIVERSE_PICKS = 3;
const MAX_PICKS = 5;

// selectTopologyTraces picks which recent traces to read. Small recent traces
// with distinct roots are preferred, since the graph wants coverage across the
// mesh's callers: after three picks a root already seen is skipped, and at
// most five are taken. The largest eligible trace is always read as well, so a
// mesh whose every trace roots at one service still yields the fat request the
// walks want (GH-425).
export function selectTopologyTraces(summaries: TraceSummary[]): string[] {
  const eligible = summaries.filter((trace) => trace.traceId && trace.spanCount <= TOPOLOGY_MAX_SPANS);
  const seenRoots = new Set<string>();
  const picked: string[] = [];
  for (const trace of eligible) {
    if (seenRoots.has(trace.rootService) && picked.length >= DIVERSE_PICKS) continue;
    seenRoots.add(trace.rootService);
    picked.push(trace.traceId);
    if (picked.length >= MAX_PICKS) break;
  }
  const largest = [...eligible].sort((a, b) => b.spanCount - a.spanCount)[0];
  if (largest && !picked.includes(largest.traceId)) picked.push(largest.traceId);
  return picked;
}

export type TopologyTraces = { status: "ok"; traces: TraceModel[] } | { status: "unavailable"; reason: string };

// fetchTopologyTraces reads the selected traces from the backend. A failed
// list is a stated reason; a trace that fails to read is dropped, so the
// topology degrades rather than breaks.
export async function fetchTopologyTraces(client: KitClient, backend: string, pageSize: number = TOPOLOGY_TRACE_PAGE): Promise<TopologyTraces> {
  const list = await fetchTraceList(client, backend, pageSize, 0);
  if (list.status !== "ok") return { status: "unavailable", reason: list.status === "unavailable" ? list.reason : "trace list not read" };
  const ids = selectTopologyTraces(list.page.traces);
  const models = await fetchTraces(client, backend, ids);
  return { status: "ok", traces: ids.flatMap((id) => models.get(id) ?? []) };
}

// requestWalks keeps, per service, the richest request-machine walk among the
// read traces. Recency alone picked wrong: a short document read is newer than
// the turn it followed, and the walk a card shows is the machine that did the
// most work recently (GH-425). Walks never mix iterations across traces
// (GH-417), because each comes whole from one TraceModel.
export function requestWalks(traces: TraceModel[]): Map<string, ServiceWalk> {
  const walks = new Map<string, ServiceWalk>();
  for (const trace of traces) {
    for (const walk of trace.walks) {
      const held = walks.get(walk.service);
      if (held && held.steps.length >= walk.steps.length) continue;
      walks.set(walk.service, walk);
    }
  }
  return walks;
}

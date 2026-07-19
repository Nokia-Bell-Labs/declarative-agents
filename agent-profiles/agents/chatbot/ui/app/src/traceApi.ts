import { useEffect, useState } from "react";

// The observability v2 waterfall fetches a turn's connected cross-agent trace by
// id from the trace backend (Jaeger query API), reached same-origin through the
// chatbot's monitor_proxy (agent-core GH-358): /monitor-proxy/jaeger/api/traces/<id>.
// Absent locally, so the panel degrades to a backend-missing notice.
export const TRACE_BACKEND = "jaeger";

export function traceQueryPath(traceId: string): string {
  return `/monitor-proxy/${TRACE_BACKEND}/api/traces/${encodeURIComponent(traceId)}`;
}

export interface TraceSpan {
  id: string;
  parentId?: string;
  name: string;
  service: string;
  startUs: number;
  durationUs: number;
}

export interface TraceModel {
  spans: TraceSpan[];
  startUs: number;
  endUs: number;
  services: string[];
}

export type TraceState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "ok"; trace: TraceModel }
  | { status: "empty" }
  | { status: "unavailable"; reason: string };

interface JaegerRef {
  refType: string;
  spanID: string;
}
interface JaegerSpan {
  spanID: string;
  operationName: string;
  startTime: number;
  duration: number;
  processID: string;
  references?: JaegerRef[];
}
interface JaegerTrace {
  spans: JaegerSpan[];
  processes: Record<string, { serviceName: string }>;
}

function toModel(trace: JaegerTrace): TraceModel {
  const spans: TraceSpan[] = trace.spans.map((s) => ({
    id: s.spanID,
    parentId: (s.references ?? []).find((r) => r.refType === "CHILD_OF")?.spanID,
    name: s.operationName,
    service: trace.processes[s.processID]?.serviceName ?? s.processID,
    startUs: s.startTime,
    durationUs: s.duration,
  }));
  spans.sort((a, b) => a.startUs - b.startUs);
  const startUs = Math.min(...spans.map((s) => s.startUs));
  const endUs = Math.max(...spans.map((s) => s.startUs + s.durationUs));
  const services = Array.from(new Set(spans.map((s) => s.service)));
  return { spans, startUs, endUs, services };
}

// useTrace fetches the trace when traceId changes, degrading to "unavailable" when
// the backend cannot be reached so the panel stays functional without it.
export function useTrace(traceId: string | undefined): TraceState {
  const [state, setState] = useState<TraceState>({ status: "idle" });

  useEffect(() => {
    if (!traceId) {
      setState({ status: "idle" });
      return;
    }
    let active = true;
    setState({ status: "loading" });
    (async () => {
      let res: Response;
      try {
        res = await fetch(traceQueryPath(traceId));
      } catch (err) {
        if (active) setState({ status: "unavailable", reason: err instanceof Error ? err.message : String(err) });
        return;
      }
      if (!active) return;
      if (!res.ok) {
        setState({ status: "unavailable", reason: `trace backend returned HTTP ${res.status}` });
        return;
      }
      let body: { data?: JaegerTrace[] } = {};
      try {
        body = (await res.json()) as { data?: JaegerTrace[] };
      } catch {
        setState({ status: "unavailable", reason: "trace backend returned a non-JSON body" });
        return;
      }
      const trace = body.data?.[0];
      if (!trace || !trace.spans || trace.spans.length === 0) {
        setState({ status: "empty" });
        return;
      }
      setState({ status: "ok", trace: toModel(trace) });
    })();
    return () => {
      active = false;
    };
  }, [traceId]);

  return state;
}

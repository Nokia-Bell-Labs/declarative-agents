import { useEffect, useState } from "react";

// The observability v2 waterfall fetches a turn's connected cross-agent trace by
// id from the collector query surface, reached same-origin through the chatbot's
// monitor_proxy (agent-core GH-358): /monitor-proxy/collector/query/traces/<id>.
// Absent locally, so the panel degrades to a backend-missing notice.
export const TRACE_BACKEND = "collector";

export function traceQueryPath(traceId: string): string {
  return `/monitor-proxy/${TRACE_BACKEND}/query/traces/${encodeURIComponent(traceId)}`;
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

interface CollectorSpan {
  span_id: string;
  parent_span_id?: string;
  name: string;
  service: string;
  start_time: string;
  end_time: string;
  status?: { code?: number; description?: string };
  attributes?: Record<string, string>;
}
interface CollectorTrace {
  trace_id: string;
  spans: CollectorSpan[];
  span_count: number;
}

function isoToUs(iso: string): number {
  return new Date(iso).getTime() * 1000;
}

function toModel(trace: CollectorTrace): TraceModel {
  const spans: TraceSpan[] = trace.spans.map((s) => {
    const startUs = isoToUs(s.start_time);
    const endUs = isoToUs(s.end_time);
    return {
      id: s.span_id,
      parentId: s.parent_span_id || undefined,
      name: s.name,
      service: s.service,
      startUs,
      durationUs: endUs - startUs,
    };
  });
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
      let body: CollectorTrace | undefined;
      try {
        body = (await res.json()) as CollectorTrace;
      } catch {
        setState({ status: "unavailable", reason: "trace backend returned a non-JSON body" });
        return;
      }
      if (!body || !body.spans || body.spans.length === 0) {
        setState({ status: "empty" });
        return;
      }
      setState({ status: "ok", trace: toModel(body) });
    })();
    return () => {
      active = false;
    };
  }, [traceId]);

  return state;
}

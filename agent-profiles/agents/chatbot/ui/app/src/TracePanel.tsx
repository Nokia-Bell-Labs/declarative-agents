import { useTrace, type TraceModel, type TraceSpan } from "./traceApi";

// A stable color per service so the waterfall reads as tiers (chatbot, rag0, ...).
const SERVICE_COLORS = ["#005aff", "#37cc73", "#f7b737", "#7d33f2", "#e23b3b", "#00b3b3"];

function serviceColor(services: string[], service: string): string {
  const idx = services.indexOf(service);
  return SERVICE_COLORS[idx % SERVICE_COLORS.length];
}

function Waterfall({ trace }: { trace: TraceModel }) {
  const total = Math.max(1, trace.endUs - trace.startUs);
  const depth = spanDepths(trace.spans);
  return (
    <div className="trace-waterfall">
      <div className="trace-legend">
        {trace.services.map((s) => (
          <span className="trace-legend-item" key={s}>
            <span className="trace-swatch" style={{ background: serviceColor(trace.services, s) }} />
            {s}
          </span>
        ))}
        <span className="trace-total">{(total / 1000).toFixed(1)} ms · {trace.spans.length} spans</span>
      </div>
      {trace.spans.map((span) => {
        const left = ((span.startUs - trace.startUs) / total) * 100;
        const width = Math.max(0.5, (span.durationUs / total) * 100);
        return (
          <div className="trace-row" key={span.id}>
            <div className="trace-label" style={{ paddingLeft: `${(depth.get(span.id) ?? 0) * 12}px` }}>
              <span className="trace-svc" style={{ color: serviceColor(trace.services, span.service) }}>
                {span.service}
              </span>{" "}
              {span.name}
            </div>
            <div className="trace-track">
              <div
                className="trace-bar"
                style={{ left: `${left}%`, width: `${width}%`, background: serviceColor(trace.services, span.service) }}
                title={`${span.name} — ${(span.durationUs / 1000).toFixed(2)} ms`}
              />
            </div>
          </div>
        );
      })}
    </div>
  );
}

// spanDepths derives an indentation depth per span from its CHILD_OF parent chain.
function spanDepths(spans: TraceSpan[]): Map<string, number> {
  const byId = new Map(spans.map((s) => [s.id, s]));
  const depth = new Map<string, number>();
  const compute = (id: string, guard: Set<string>): number => {
    if (depth.has(id)) return depth.get(id)!;
    const span = byId.get(id);
    if (!span || !span.parentId || !byId.has(span.parentId) || guard.has(id)) {
      depth.set(id, 0);
      return 0;
    }
    guard.add(id);
    const d = compute(span.parentId, guard) + 1;
    depth.set(id, d);
    return d;
  };
  spans.forEach((s) => compute(s.id, new Set()));
  return depth;
}

export default function TracePanel({ traceId }: { traceId: string | undefined }) {
  const state = useTrace(traceId);

  if (!traceId) {
    return <div className="trace-notice">The selected turn has no trace id.</div>;
  }
  if (state.status === "loading") {
    return <div className="trace-notice">Loading trace {traceId.slice(0, 12)}…</div>;
  }
  if (state.status === "unavailable") {
    return (
      <div className="trace-notice trace-notice-warn">
        Trace backend not reachable ({state.reason}). The per-agent monitor panels above stay live; deploy the
        collector and Jaeger (Helm values) to see the cross-agent waterfall.
      </div>
    );
  }
  if (state.status === "empty") {
    return <div className="trace-notice">No spans found for trace {traceId.slice(0, 12)} yet.</div>;
  }
  if (state.status === "ok") {
    return <Waterfall trace={state.trace} />;
  }
  return null;
}

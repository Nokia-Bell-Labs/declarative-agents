import { useState } from "react";
import type { TraceModel, TraceSpan } from "../../api/traceApi";
import { useTraceReader } from "./options";
import { SpanDetail } from "./SpanDetail";
import { StepGroupList, type SpanSelection } from "./SpanRows";
import { continuations, stepGroups } from "./traceGroups";
import { FAILURE_SIGNAL, serviceColor, timelineLanes, timelineSequence } from "./traceLayout";
import { FULL_WINDOW, WindowBrush, type BrushWindow } from "./WindowBrush";

// How the timeline lists the window's calls: "sequence" is one row per call in
// start order (cohere-demo); "steps" groups them under the root machine's
// dispatches with state labels and answers (agentic-wiki-mesh).
export type TimelineBody = "sequence" | "steps";

// The external-peer lane mirrors calls it received; its bars stay neutral.
const EXTERNAL_BAR = "var(--text-tertiary, #999999)";

// The agent timeline (GH-511): one row per active message generator -- mesh
// services and the external peers their boundary spans called -- a brush to
// zoom a time window, and the window's calls top to bottom. The point is
// causality: an input makes the chatbot embed, the embedding queries the
// store, the results feed the rerank, the rerank feeds the model.
export function Timeline({ trace, body = "sequence" }: { trace: TraceModel; body?: TimelineBody }) {
  const total = Math.max(1, trace.endUs - trace.startUs);
  const reader = useTraceReader(trace);
  const [range, setRange] = useState<BrushWindow>(FULL_WINDOW);
  const [selected, setSelected] = useState<string | undefined>(undefined);
  const startUs = trace.startUs + range.start * total;
  const endUs = trace.startUs + range.end * total;
  const width = Math.max(1, endUs - startUs);
  const lanes = timelineLanes(trace, startUs, endUs);
  const selectedSpan = trace.spans.find((s) => s.id === selected);
  const toggle = (id: string) => setSelected(selected === id ? undefined : id);
  const follow = (span: TraceSpan) => {
    setSelected(span.id);
    document.getElementById(`span-row-${span.id}`)?.scrollIntoView?.({ block: "center" });
  };
  const selection: SpanSelection = { selected, onSelect: toggle, onFollow: follow };

  return (
    <div className="timeline" data-testid="trace-timeline">
      <div className="trace-head">Agent timeline — drag the handles to zoom; rows are the agents active in the window</div>
      <WindowBrush
        ticks={trace.spans.map((s) => ({
          key: s.id,
          left: ((s.startUs - trace.startUs) / total) * 100,
          width: (s.durationUs / total) * 100,
          color: serviceColor(trace.services, s.service),
        }))}
        range={range}
        onChange={setRange}
        windowUs={endUs - startUs}
        activeLanes={lanes.length}
      />
      {lanes.map((lane) => (
        <div className="timeline-lane" key={lane.name} data-testid="trace-lane">
          <div className={`timeline-lane-name${lane.external ? " timeline-lane-external" : ""}`}>
            {lane.name}
            {lane.external && (
              <span className="timeline-external-mark" title="external peer — bars mirror the calls it received">
                ↗
              </span>
            )}
          </div>
          <div className="timeline-lane-track">
            {lane.spans.map((s) => (
              <button
                type="button"
                key={`${lane.name}:${s.id}`}
                className={`timeline-bar${selected === s.id ? " timeline-bar-selected" : ""}`}
                style={{
                  left: `${((Math.max(s.startUs, startUs) - startUs) / width) * 100}%`,
                  width: `${Math.max(0.4, ((Math.min(s.startUs + s.durationUs, endUs) - Math.max(s.startUs, startUs)) / width) * 100)}%`,
                  background: lane.external ? EXTERNAL_BAR : serviceColor(trace.services, s.service),
                }}
                title={`${s.command ?? s.name} — ${(s.durationUs / 1000).toFixed(1)} ms${s.target ? ` → ${s.target}` : ""}`}
                onClick={() => toggle(s.id)}
              />
            ))}
          </div>
        </div>
      ))}
      {body === "steps" ? (
        <>
          <div className="trace-head timeline-seq-head">The steps of this turn — each dispatch the root agent made, and what it caused</div>
          <StepGroupList groups={stepGroups(trace, startUs, endUs)} trace={trace} reader={reader} selection={selection} />
        </>
      ) : (
        <>
          <div className="trace-head timeline-seq-head">The window, top to bottom — each call and what it caused</div>
          <div className="timeline-sequence" data-testid="trace-sequence">
            {timelineSequence(trace, startUs, endUs).map(({ span: s, offsetUs }) => (
              <button type="button" key={s.id} className={`timeline-seq-row${selected === s.id ? " timeline-seq-selected" : ""}`} onClick={() => toggle(s.id)}>
                <span className="timeline-seq-offset">+{(offsetUs / 1000).toFixed(0)} ms</span>
                <span className="trace-svc" style={{ color: serviceColor(trace.services, s.service) }}>
                  {s.service}
                </span>
                <span className="timeline-seq-call">{s.command ?? s.name}</span>
                {s.target && <span className="timeline-seq-target">→ {s.target}</span>}
                {s.signal && <span className={`walk-signal${FAILURE_SIGNAL.test(s.signal) ? " walk-signal-failed" : ""}`}>{s.signal}</span>}
                <span className="trace-ms">{(s.durationUs / 1000).toFixed(1)} ms</span>
              </button>
            ))}
          </div>
          {lanes.length === 0 && <div className="trace-notice">No calls in this window.</div>}
        </>
      )}
      {selectedSpan && <SpanDetail span={selectedSpan} model={trace} links={continuations(selectedSpan, trace.spans)} onFollow={follow} onClose={() => setSelected(undefined)} />}
    </div>
  );
}

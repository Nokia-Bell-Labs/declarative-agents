import { useState, type ReactNode } from "react";
import type { ServiceWalk, TraceModel } from "../../api/traceApi";
import { useTrace } from "../../hooks/useTrace";
import type { PanelProps } from "../manifest";
import { TraceOptionsProvider, type TraceViewOptions } from "./options";
import { Timeline, type TimelineBody } from "./Timeline";
import { TraceList } from "./TraceList";
import { FAILURE_SIGNAL } from "./traceLayout";
import { SpanTreeToggle } from "./Waterfall";

// TracePanel shows one cross-agent trace read from the declared trace backend
// (srd004 R2.4): the agent timeline, the span tree behind a toggle, and the
// request machines' walks. Base is cohere-demo's TracePanel; the step view,
// state labels, and trace list come from agentic-wiki-mesh.

// The backend a mount reads when the shell declares none (srd004 R2.4).
const DEFAULT_BACKEND = "collector";

export interface TracePanelProps extends TraceViewOptions {
  traceId: string | undefined;
  backend?: string;
  // "sequence" lists the window's calls in order; "steps" groups them.
  timelineBody?: TimelineBody;
  // Appended to the notice when the backend cannot be read.
  unavailableHint?: string;
  // Extra views over the loaded trace, rendered under the timeline (an
  // application's machine view, for one).
  renderExtra?: (trace: TraceModel) => ReactNode;
}

// MachineWalks renders the request-scoped machines the trace recorded: one
// ladder per service, each step the command an iteration ran and the signal
// it emitted (GH-413).
export function MachineWalks({ walks }: { walks: ServiceWalk[] }) {
  if (walks.length === 0) return null;
  return (
    <div className="walks" data-testid="trace-walks">
      <div className="trace-head walks-head">Request machines — the walk each service took</div>
      <div className="walks-grid">
        {walks.map((walk) => (
          <div className="walk" key={walk.service}>
            <div className="walk-title">
              {walk.service} · {walk.steps.length} steps
            </div>
            <ol className="walk-steps">
              {walk.steps.map((step) => (
                <li className="walk-step" key={step.iteration}>
                  <span className="walk-iteration">{step.iteration}</span>
                  <span className="walk-command">{step.command}</span>
                  <span className={`walk-signal${FAILURE_SIGNAL.test(step.signal) ? " walk-signal-failed" : ""}`}>{step.signal}</span>
                </li>
              ))}
            </ol>
          </div>
        ))}
      </div>
    </div>
  );
}

function TraceBody({
  traceId,
  backend = DEFAULT_BACKEND,
  timelineBody,
  unavailableHint = "The cross-agent trace appears once the trace backend agent is deployed and reachable.",
  renderExtra,
}: TracePanelProps) {
  const state = useTrace(backend, traceId);
  if (!traceId) return <div className="trace-notice">No trace selected.</div>;
  if (state.status === "idle" || state.status === "loading") {
    return <div className="trace-notice">Loading trace {traceId.slice(0, 12)}…</div>;
  }
  if (state.status === "unavailable") {
    return (
      <div className="trace-notice trace-notice-warn" data-testid="trace-unavailable">
        Trace backend {backend} not reachable ({state.reason}). {unavailableHint}
      </div>
    );
  }
  if (state.status === "empty") {
    return (
      <div className="trace-notice" data-testid="trace-empty">
        The trace backend holds no spans for trace {traceId.slice(0, 12)} yet; reopen it in a moment.
      </div>
    );
  }
  return (
    <>
      <Timeline trace={state.trace} body={timelineBody} />
      {renderExtra?.(state.trace)}
      <SpanTreeToggle trace={state.trace} />
      <MachineWalks walks={state.trace.walks} />
    </>
  );
}

export function TracePanel(props: TracePanelProps) {
  return (
    <TraceOptionsProvider options={props}>
      <div className="dak-trace" data-testid="trace-panel">
        <TraceBody {...props} />
      </div>
    </TraceOptionsProvider>
  );
}

export interface TraceViewProps extends Omit<TracePanelProps, "traceId"> {
  pageSize?: number;
  listTitle?: string;
  emptyHint?: string;
}

// TraceView is the list-to-detail browser: the backend's trace list, and the
// trace a row opens with a way back to the list.
export function TraceView({ pageSize, listTitle, emptyHint, ...panel }: TraceViewProps) {
  const backend = panel.backend ?? DEFAULT_BACKEND;
  const [openId, setOpenId] = useState<string | undefined>(undefined);
  if (!openId) {
    return <TraceList backend={backend} onOpen={setOpenId} pageSize={pageSize} title={listTitle} emptyHint={emptyHint} unavailableHint={panel.unavailableHint} />;
  }
  return (
    <div className="dak-trace">
      <div className="trace-section" data-testid="trace-section">
        <div className="trace-head trace-view-head">
          <button type="button" className="detail-toggle" data-testid="trace-back" onClick={() => setOpenId(undefined)}>
            ‹ all traces
          </button>
          <span>Cross-agent trace</span>
          <span className="trace-id">{openId.slice(0, 16)}</span>
        </div>
        <TracePanel {...panel} backend={backend} traceId={openId} />
      </div>
    </div>
  );
}

// TracePanelMount is the mountable form: the shell supplies the declared trace
// backend.
export function TracePanelMount({ traceBackend }: PanelProps) {
  return <TraceView backend={traceBackend ?? DEFAULT_BACKEND} />;
}

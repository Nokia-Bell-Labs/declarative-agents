import { useState } from "react";
import { isSingleSpanTrace, readableTraces, type TraceSummary } from "../../api/traceApi";
import { useTraceList } from "../../hooks/useTrace";

// The trace list (agentic-wiki-mesh rel09.0-uc001, srd005 R3.4): the trace
// backend's own list of what it holds, one page at a time. Single-span traces
// (health probes, heartbeats) outnumber request traces and are hidden by
// default, with the count of what the filter hides.

export const DEFAULT_TRACE_PAGE_SIZE = 50;

export interface TraceListProps {
  // The agent ui.yaml names as the trace backend (srd004 R2.4).
  backend: string;
  onOpen: (traceId: string) => void;
  openTraceId?: string;
  pageSize?: number;
  title?: string;
  // Shown when a page holds no readable trace.
  emptyHint?: string;
  // Appended to the notice when the backend cannot be read.
  unavailableHint?: string;
}

// relativeTime renders a start time as seconds, minutes, or hours ago.
export function relativeTime(startTime: string, now: number = Date.now()): string {
  const at = Date.parse(startTime);
  if (Number.isNaN(at)) return startTime;
  const seconds = Math.max(0, Math.round((now - at) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m ago`;
  return `${Math.round(seconds / 3600)}h ago`;
}

function TraceRow({ summary, open, onOpen }: { summary: TraceSummary; open: boolean; onOpen: () => void }) {
  return (
    <button type="button" className={`trace-list-row${open ? " trace-list-row-on" : ""}`} data-testid="trace-list-row" data-trace={summary.traceId} onClick={onOpen}>
      <span className="trace-svc">{summary.rootService}</span>
      <span className="trace-list-name">{summary.rootSpanName}</span>
      <span className="trace-list-spans">{summary.spanCount} spans</span>
      <span className="trace-ms">{summary.durationMs.toFixed(0)} ms</span>
      <span className="trace-list-when">{relativeTime(summary.startTime)}</span>
    </button>
  );
}

export function TraceList({
  backend,
  onOpen,
  openTraceId,
  pageSize = DEFAULT_TRACE_PAGE_SIZE,
  title = "Collected traces",
  emptyHint = "The trace backend holds no multi-span traces on this page yet.",
  unavailableHint = "Traces appear here once the trace backend agent is deployed and reachable.",
}: TraceListProps) {
  const [showAll, setShowAll] = useState(false);
  const [offset, setOffset] = useState(0);
  const state = useTraceList(backend, pageSize, offset);

  let body;
  if (state.status === "loading") {
    body = <div className="trace-notice">Reading the trace backend…</div>;
  } else if (state.status === "unavailable") {
    body = (
      <div className="trace-notice trace-notice-warn" data-testid="trace-list-unavailable">
        Trace backend {backend} not reachable ({state.reason}). {unavailableHint}
      </div>
    );
  } else {
    const { page } = state;
    const rows = showAll ? page.traces : readableTraces(page);
    const hidden = page.traces.filter(isSingleSpanTrace).length;
    const last = page.offset + page.traces.length;
    body = (
      <>
        <div className="trace-head trace-list-head">
          <span>
            {title} — {rows.length} shown
            {!showAll && hidden > 0 && (
              <span className="trace-list-hidden" data-testid="trace-list-hidden">
                {" "}
                · {hidden} single-span hidden
              </span>
            )}
          </span>
          <label className="trace-list-filter">
            <input type="checkbox" checked={showAll} data-testid="trace-list-show-all" onChange={(event) => setShowAll(event.target.checked)} />
            include single-span traces
          </label>
        </div>
        {rows.length === 0 ? (
          <div className="trace-notice" data-testid="trace-list-empty">
            {emptyHint}
          </div>
        ) : (
          rows.map((summary) => <TraceRow key={summary.traceId} summary={summary} open={summary.traceId === openTraceId} onOpen={() => onOpen(summary.traceId)} />)
        )}
        <div className="trace-list-pager" data-testid="trace-list-pager">
          <button type="button" disabled={page.offset === 0} onClick={() => setOffset(Math.max(0, page.offset - pageSize))}>
            ‹ newer
          </button>
          <span data-testid="trace-list-range">{page.traces.length === 0 ? `0 of ${page.total}` : `${page.offset + 1}–${last} of ${page.total}`}</span>
          <button type="button" disabled={page.offset + pageSize >= page.total} onClick={() => setOffset(page.offset + pageSize)}>
            older ›
          </button>
        </div>
      </>
    );
  }

  return (
    <div className="dak-trace">
      <div className="trace-list" data-testid="trace-list">
        {body}
      </div>
    </div>
  );
}

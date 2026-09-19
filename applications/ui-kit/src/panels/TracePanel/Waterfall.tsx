import { useState } from "react";
import type { TraceModel } from "../../api/traceApi";
import { useTraceReader } from "./options";
import { errorClass, SpanErrorMark } from "./SpanRows";
import { filterTree, groupRootsByService, serviceColor, spanTree, type SpanNode } from "./traceLayout";

// The trace as a collapsible span tree on one shared timeline (GH-423): rows in
// depth-first order so a parent sits above everything it caused, whichever
// agent ran it, and a toggle folds a subtree away with its count.
export function Waterfall({ trace }: { trace: TraceModel }) {
  const total = Math.max(1, trace.endUs - trace.startUs);
  const reader = useTraceReader(trace);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  // The filter narrows to matching calls -- by command, name, service, or
  // declared description -- keeping ancestors so the tree stays rooted.
  // Clicking a row's command chip pins the filter to that exact command.
  const [query, setQuery] = useState("");
  const fullRoots = groupRootsByService(spanTree(trace.spans));
  const needle = query.trim().toLowerCase();
  const exact = needle.startsWith("=") ? needle.slice(1) : undefined;
  const { roots, matched } =
    needle === ""
      ? { roots: fullRoots, matched: trace.spans.length }
      : filterTree(fullRoots, (span) =>
          exact !== undefined
            ? span.command === exact
            : [span.command ?? "", span.name, span.service, reader.description(span) ?? ""].join(" ").toLowerCase().includes(needle),
        );

  const toggle = (id: string) => {
    setCollapsed((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };
  const collapseToRoots = () => setCollapsed(new Set(roots.flatMap((root) => (root.children.length ? [root.span.id] : []))));

  const rows: Array<{ node: SpanNode; depth: number }> = [];
  const walk = (node: SpanNode, depth: number) => {
    rows.push({ node, depth });
    if (collapsed.has(node.span.id)) return;
    for (const child of node.children) walk(child, depth + 1);
  };
  for (const root of roots) walk(root, 0);

  return (
    <div className="trace-waterfall" data-testid="trace-waterfall">
      <div className="trace-legend">
        {trace.services.map((s) => (
          <span className="trace-legend-item" key={s}>
            <span className="trace-swatch" style={{ background: serviceColor(trace.services, s) }} />
            {s}
          </span>
        ))}
        <span className="trace-total">
          {(total / 1000).toFixed(1)} ms · {trace.spans.length} spans
        </span>
        <input className="trace-filter" type="search" placeholder="filter calls…" aria-label="filter calls" value={query} onChange={(event) => setQuery(event.target.value)} />
        {needle !== "" && <span className="trace-matched">{matched} match(es)</span>}
        <span className="trace-fold">
          <button type="button" onClick={() => setCollapsed(new Set())}>
            expand all
          </button>
          <button type="button" onClick={collapseToRoots}>
            collapse to roots
          </button>
        </span>
      </div>
      {rows.map(({ node, depth }) => {
        const span = node.span;
        const left = ((span.startUs - trace.startUs) / total) * 100;
        const width = Math.max(0.5, (span.durationUs / total) * 100);
        const folded = collapsed.has(span.id);
        const description = reader.description(span);
        return (
          <div className={`trace-row${errorClass(span)}`} key={span.id} data-testid="trace-row">
            <div className="trace-label" style={{ paddingLeft: `${depth * 14}px` }}>
              {node.children.length > 0 ? (
                <button type="button" className="trace-toggle" aria-expanded={!folded} onClick={() => toggle(span.id)}>
                  {folded ? "▸" : "▾"}
                </button>
              ) : (
                <span className="trace-toggle trace-toggle-leaf" />
              )}
              <span className="trace-svc" style={{ color: serviceColor(trace.services, span.service) }}>
                {span.service}
              </span>{" "}
              {span.command ? (
                <button
                  type="button"
                  className="trace-command"
                  title={`filter to every ${span.command} call`}
                  onClick={() => setQuery(query === `=${span.command}` ? "" : `=${span.command}`)}
                >
                  {span.command}
                </button>
              ) : (
                span.name
              )}
              <SpanErrorMark span={span} />
              {folded && node.descendants > 0 && <span className="trace-folded-count">+{node.descendants}</span>}
              {description && (
                <span className="trace-desc" title={description}>
                  {description}
                </span>
              )}
              <span className="trace-ms">{(span.durationUs / 1000).toFixed(1)} ms</span>
            </div>
            <div className="trace-track">
              <div
                className={`trace-bar${errorClass(span)}`}
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

// The span tree stays available behind a toggle: the timeline narrates the
// sequence, the tree shows parentage.
export function SpanTreeToggle({ trace }: { trace: TraceModel }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="spantree-toggle-wrap">
      <button type="button" className="detail-toggle" data-testid="trace-tree-toggle" onClick={() => setOpen((was) => !was)}>
        {open ? "hide the span tree" : "show the span tree"}
      </button>
      {open && <Waterfall trace={trace} />}
    </div>
  );
}

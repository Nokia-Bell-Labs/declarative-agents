import type { KeyboardEvent } from "react";
import { isErrorSpan, type TraceModel, type TraceSpan } from "../../api/traceApi";
import type { TraceReader } from "./options";
import { continuations, type Continuation, type ServiceGroup, type StepGroup } from "./traceGroups";
import { CATEGORY_COLORS, FAILURE_SIGNAL, serviceColor, spanAnswer } from "./traceLayout";

// The clickable span rows shared by the timeline's step view and the story
// overlay (GH-652): one row per span with its category badge, the machine
// state it ran in when the declarations say, its signal, and any
// cross-service continuation; a forced tool call and a model's answer ride
// their own follow-on rows.

function activate(onSelect: () => void) {
  return (event: KeyboardEvent) => {
    if (event.key === "Enter" || event.key === " ") onSelect();
  };
}

function Signal({ signal }: { signal: string }) {
  return <span className={`walk-signal${FAILURE_SIGNAL.test(signal) ? " walk-signal-failed" : ""}`}>{signal}</span>;
}

// SpanErrorMark flags a span whose status is an error (OTel status code 2);
// its title carries the status description.
export function SpanErrorMark({ span }: { span: TraceSpan }) {
  if (!isErrorSpan(span)) return null;
  return (
    <span className="span-error-mark" data-testid="span-error" title={span.status?.description || "the span ended with an error status"}>
      error
    </span>
  );
}

// errorClass appends span-error to a row or bar class for a failed span.
export function errorClass(span: TraceSpan): string {
  return isErrorSpan(span) ? " span-error" : "";
}

export function SpanRow({
  span,
  trace,
  reader,
  selected,
  onSelect,
  links,
  onFollow,
}: {
  span: TraceSpan;
  trace: TraceModel;
  reader: TraceReader;
  selected: boolean;
  onSelect: () => void;
  links: Continuation[];
  onFollow: (span: TraceSpan) => void;
}) {
  const category = reader.category(span);
  const state = reader.state(span);
  const description = reader.description(span);
  return (
    <div
      id={`span-row-${span.id}`}
      className={`span-row timeline-seq-row${selected ? " timeline-seq-selected" : ""}${errorClass(span)}`}
      data-testid="trace-call"
      data-category={category}
      data-command={span.command ?? ""}
      data-state={state ?? ""}
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={activate(onSelect)}
    >
      <span className="timeline-seq-offset">+{((span.startUs - trace.startUs) / 1000).toFixed(0)} ms</span>
      <span className="seq-category" style={{ background: CATEGORY_COLORS[category] }}>
        {category}
      </span>
      <span className="trace-svc" style={{ color: serviceColor(trace.services, span.service) }}>
        {span.service}
      </span>
      <span className="timeline-seq-call">{span.command ?? span.name}</span>
      <SpanErrorMark span={span} />
      {state && (
        <span className="span-state" data-testid="span-state" title="the machine state this word ran in">
          {state}
        </span>
      )}
      {description && (
        <span className="trace-desc" title={description}>
          {description}
        </span>
      )}
      {span.target && <span className="timeline-seq-target">→ {span.target}</span>}
      {span.signal && <Signal signal={span.signal} />}
      <span className="trace-ms">{(span.durationUs / 1000).toFixed(1)} ms</span>
      {links.map((link) => (
        <button
          key={`${link.direction}:${link.span.id}`}
          type="button"
          className="span-follow"
          title={link.direction === "sent" ? "Follow the handoff to its continuation" : "Jump to the span this one continues"}
          onClick={(event) => {
            event.stopPropagation();
            onFollow(link.span);
          }}
        >
          {link.direction === "sent" ? `→ continues at ${link.span.service}` : `← received from ${link.span.service}`}
        </button>
      ))}
    </div>
  );
}

// The forced call's reply as its own row (GH-710): one HTTP call, two beats --
// the model call above, the tool call it returned below.
function ToolCallRow({ tool, onSelect }: { tool: string; onSelect: () => void }) {
  return (
    <div className="span-row timeline-seq-row span-row-toolcall" role="button" tabIndex={0} onClick={onSelect} onKeyDown={activate(onSelect)}>
      <span className="timeline-seq-offset">↳</span>
      <span className="seq-category" style={{ background: CATEGORY_COLORS["tool call"] }}>
        tool call
      </span>
      <span className="timeline-seq-call">{tool}</span>
      <span className="timeline-seq-target">returned by the model</span>
    </div>
  );
}

// The model's answer under its call (GH-728): a one-line snippet from the
// captured output messages or response body; the drawer holds the full text.
function AnswerRow({ span, onSelect }: { span: TraceSpan; onSelect: () => void }) {
  const answer = spanAnswer(span);
  if (answer === undefined) return null;
  const snippet = answer.replace(/\s+/g, " ").slice(0, 110) + (answer.length > 110 ? "…" : "");
  return (
    <div
      className="span-row timeline-seq-row span-row-answer"
      data-testid="trace-answer"
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={activate(onSelect)}
      title="The model's answer — click for the full span content"
    >
      <span className="timeline-seq-offset">↳</span>
      <span className="seq-category span-answer-badge">response</span>
      <span className="span-answer-text">“{snippet}”</span>
    </div>
  );
}

export interface SpanSelection {
  selected: string | undefined;
  onSelect: (id: string) => void;
  onFollow: (span: TraceSpan) => void;
}

// SpanEntry is one span with its follow-on rows.
export function SpanEntry({ span, trace, reader, selection, links }: { span: TraceSpan; trace: TraceModel; reader: TraceReader; selection: SpanSelection; links: Continuation[] }) {
  const select = () => selection.onSelect(span.id);
  return (
    <div className="span-entry">
      <SpanRow span={span} trace={trace} reader={reader} selected={selection.selected === span.id} onSelect={select} links={links} onFollow={selection.onFollow} />
      {span.command?.endsWith("_via_tool") && <ToolCallRow tool={reader.toolName(span.command)} onSelect={select} />}
      {reader.category(span) === "model call" && <AnswerRow span={span} onSelect={select} />}
    </div>
  );
}

// StepGroupList renders the window grouped on the root machine's steps, so a
// fan-out to other agents reads inside the step that caused it.
export function StepGroupList({ groups, trace, reader, selection }: { groups: StepGroup[]; trace: TraceModel; reader: TraceReader; selection: SpanSelection }) {
  if (groups.length === 0) return <div className="trace-notice">No calls in this window.</div>;
  return (
    <div className="step-groups" data-testid="trace-steps">
      {groups.map((group) => (
        <div className="step-group" data-testid="step-group" key={group.anchor.id}>
          <div className="step-group-head">
            <span className="trace-svc" style={{ color: serviceColor(trace.services, group.anchor.service) }}>
              {group.anchor.service}
            </span>
            <span className="step-group-command">{group.anchor.command ?? group.anchor.name}</span>
            {group.anchor.signal && <Signal signal={group.anchor.signal} />}
            {group.spans.length > 1 && <span className="step-group-count">{group.spans.length} spans</span>}
            <span className="trace-ms">+{((group.anchor.startUs - trace.startUs) / 1000).toFixed(0)} ms</span>
          </div>
          {group.spans.map((span) => (
            <SpanEntry key={span.id} span={span} trace={trace} reader={reader} selection={selection} links={continuations(span, trace.spans)} />
          ))}
        </div>
      ))}
    </div>
  );
}

// AgentGroupList renders the same window by emitting service, each folded
// until opened; the rows carry their handoff links.
export function AgentGroupList({
  groups,
  trace,
  reader,
  selection,
  expanded,
  onToggle,
}: {
  groups: ServiceGroup[];
  trace: TraceModel;
  reader: TraceReader;
  selection: SpanSelection;
  expanded: Set<string>;
  onToggle: (service: string) => void;
}) {
  return (
    <div className="agent-groups">
      {groups.map((group) => {
        const open = expanded.has(group.service);
        return (
          <div className="agent-group" key={group.service}>
            <button type="button" className="agent-group-head" aria-expanded={open} onClick={() => onToggle(group.service)}>
              <span className="agent-group-fold">{open ? "▾" : "▸"}</span>
              <span className="agent-group-name">{group.service}</span>
              <span className="agent-group-count">{group.spans.length} span(s) in window</span>
            </button>
            {open &&
              group.spans.map((span) => (
                <SpanEntry key={span.id} span={span} trace={trace} reader={reader} selection={selection} links={continuations(span, trace.spans)} />
              ))}
          </div>
        );
      })}
    </div>
  );
}

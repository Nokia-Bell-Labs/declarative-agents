import type { TraceModel, TraceSpan } from "../../api/traceApi";
import { traceRootService } from "./traceLayout";

// Grouping for the step and agent views (GH-652): the same window of spans read
// two ways. Steps groups every agent's spans under the root machine's own
// commands, in the order the machine ran them; Agents groups the window by the
// emitting service, with the cross-service links that let a reader follow a
// handoff. Identical in cohere-demo and agentic-wiki-mesh.

export interface StepGroup {
  // The root-service span that anchors the step: the command the machine ran.
  anchor: TraceSpan;
  // Every span (any agent) whose start falls inside the step, anchor included,
  // in time order.
  spans: TraceSpan[];
}

export interface ServiceGroup {
  service: string;
  spans: TraceSpan[];
}

// windowSpans filters to the brush window: a span belongs when its start lies
// inside [startUs, endUs).
export function windowSpans(model: TraceModel, startUs: number, endUs: number): TraceSpan[] {
  return model.spans.filter((span) => span.startUs >= startUs && span.startUs < endUs);
}

// stepGroups anchors on the root service's command spans and assigns every
// other span in the window to the step whose interval holds its start. Spans
// before the first anchor fold into the first step; a window with no anchors
// yields one anonymous group.
export function stepGroups(model: TraceModel, startUs: number, endUs: number): StepGroup[] {
  const root = traceRootService(model);
  const inWindow = windowSpans(model, startUs, endUs);
  const anchors = inWindow.filter((span) => span.service === root && span.command !== undefined).sort((a, b) => a.startUs - b.startUs);
  if (anchors.length === 0) {
    return inWindow.length > 0 ? [{ anchor: inWindow[0], spans: inWindow }] : [];
  }
  const groups: StepGroup[] = anchors.map((anchor) => ({ anchor, spans: [] }));
  for (const span of inWindow) {
    if (anchors.includes(span)) continue;
    let slot = 0;
    for (let i = 0; i < anchors.length; i++) {
      if (anchors[i].startUs <= span.startUs) slot = i;
      else break;
    }
    groups[slot].spans.push(span);
  }
  for (const group of groups) {
    group.spans = [group.anchor, ...group.spans].sort((a, b) => a.startUs - b.startUs);
  }
  return groups;
}

// agentGroups: the window's spans by emitting service, services in
// first-appearance order, spans in time order.
export function agentGroups(model: TraceModel, startUs: number, endUs: number): ServiceGroup[] {
  const groups = new Map<string, TraceSpan[]>();
  for (const span of windowSpans(model, startUs, endUs).sort((a, b) => a.startUs - b.startUs)) {
    const list = groups.get(span.service) ?? [];
    list.push(span);
    groups.set(span.service, list);
  }
  return [...groups.entries()].map(([service, spans]) => ({ service, spans }));
}

export interface Continuation {
  direction: "sent" | "received";
  span: TraceSpan;
}

// continuations names the cross-service hops a span takes part in: children in
// another service ("sent"), and a parent in another service ("received").
// Same-service links are navigation noise and omitted.
export function continuations(span: TraceSpan, spans: TraceSpan[]): Continuation[] {
  const out: Continuation[] = [];
  if (span.parentId) {
    const parent = spans.find((candidate) => candidate.id === span.parentId);
    if (parent && parent.service !== span.service) out.push({ direction: "received", span: parent });
  }
  for (const candidate of spans) {
    if (candidate.parentId === span.id && candidate.service !== span.service) out.push({ direction: "sent", span: candidate });
  }
  return out;
}

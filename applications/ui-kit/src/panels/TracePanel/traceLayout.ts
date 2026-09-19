import type { TraceModel, TraceSpan } from "../../api/traceApi";

// Presentation helpers over the kit TraceModel: the span tree, the agent
// timeline, the story categories, and the declared-word lookups. Merged from
// cohere-demo (tree, timeline, host-based categories, answers) and
// agentic-wiki-mesh (the declared-word surface). The data layer that builds
// TraceModel lives in src/api/traceApi.ts.

// ---- The span tree (GH-423) ----

// One node of the span tree: the span, its children in start order, and how
// many descendants a collapse hides.
export interface SpanNode {
  span: TraceSpan;
  children: SpanNode[];
  descendants: number;
}

// spanTree arranges a trace's spans as a forest: children under their parent
// sorted by start time, roots in start order, and a span whose parent the
// trace does not carry rooted rather than dropped -- the collector's page cap
// can trim an ancestor, and an orphaned subtree is still evidence.
export function spanTree(spans: TraceSpan[]): SpanNode[] {
  const nodes = new Map<string, SpanNode>();
  for (const span of spans) {
    nodes.set(span.id, { span, children: [], descendants: 0 });
  }
  const roots: SpanNode[] = [];
  for (const node of nodes.values()) {
    const parent = node.span.parentId ? nodes.get(node.span.parentId) : undefined;
    if (parent && parent !== node) parent.children.push(node);
    else roots.push(node);
  }
  const order = (a: SpanNode, b: SpanNode) => a.span.startUs - b.span.startUs;
  const count = (node: SpanNode): number => {
    node.children.sort(order);
    node.descendants = node.children.reduce((sum, child) => sum + 1 + count(child), 0);
    return node.descendants;
  };
  roots.sort(order);
  for (const root of roots) count(root);
  return roots;
}

// groupRootsByService folds a service's parentless spans under one synthetic
// group node. The exporters parent most execute_tool spans to per-iteration
// spans that never reach the collector, so a raw tree is nearly flat and
// folding does almost nothing. The group node is presentation, and says so in
// its name; a service with one root keeps it ungrouped.
export function groupRootsByService(roots: SpanNode[]): SpanNode[] {
  const byService = new Map<string, SpanNode[]>();
  for (const root of roots) {
    byService.set(root.span.service, [...(byService.get(root.span.service) ?? []), root]);
  }
  const grouped: SpanNode[] = [];
  for (const [service, members] of byService) {
    if (members.length === 1) {
      grouped.push(members[0]);
      continue;
    }
    const startUs = Math.min(...members.map((m) => m.span.startUs));
    const endUs = Math.max(...members.map((m) => m.span.startUs + m.span.durationUs));
    grouped.push({
      span: { id: `service-group:${service}`, name: `${service} — ${members.length} spans`, service, startUs, durationUs: endUs - startUs },
      children: members,
      descendants: members.reduce((sum, m) => sum + 1 + m.descendants, 0),
    });
  }
  grouped.sort((a, b) => a.span.startUs - b.span.startUs);
  return grouped;
}

// filterTree narrows a forest to the nodes matching a predicate, keeping every
// ancestor of a match so the tree stays rooted, and recounts descendants.
export function filterTree(roots: SpanNode[], matches: (span: TraceSpan) => boolean): { roots: SpanNode[]; matched: number } {
  let matched = 0;
  const prune = (node: SpanNode): SpanNode | undefined => {
    const children = node.children.map(prune).filter((child): child is SpanNode => child !== undefined);
    const hit = matches(node.span);
    if (hit) matched++;
    if (!hit && children.length === 0) return undefined;
    return { span: node.span, children, descendants: children.reduce((sum, child) => sum + 1 + child.descendants, 0) };
  };
  const pruned = roots.map(prune).filter((root): root is SpanNode => root !== undefined);
  return { roots: pruned, matched };
}

// ---- The agent timeline (GH-511) ----

export interface TimelineLane {
  name: string;
  external: boolean;
  spans: TraceSpan[];
}

// bareHost strips a port so a lane reads as the peer, not the socket.
function bareHost(target: string): string {
  return target.split(":")[0];
}

// A host belongs to a service when it is the service or a deployment name
// ending in it (demo-chatbot-mesh-rag0); a backing store like ...-rag0-chroma
// is its own peer.
function hostIsService(host: string, services: Iterable<string>): boolean {
  for (const service of services) {
    if (host === service || host.endsWith(`-${service}`)) return true;
  }
  return false;
}

// timelineLanes arranges a trace as agent rows: one lane per service in
// first-activity order, then one per external peer the boundary spans called
// (mirrored bars, since the peer exports no spans of its own). Only spans
// overlapping the window survive, and a lane with nothing in it is dropped.
export function timelineLanes(model: TraceModel, windowStartUs: number, windowEndUs: number): TimelineLane[] {
  const inWindow = (span: TraceSpan) => span.startUs < windowEndUs && span.startUs + span.durationUs > windowStartUs;
  const services = new Set(model.services);
  const byService = new Map<string, TraceSpan[]>();
  const byExternal = new Map<string, TraceSpan[]>();
  for (const span of model.spans) {
    if (!inWindow(span)) continue;
    byService.set(span.service, [...(byService.get(span.service) ?? []), span]);
    if (span.target) {
      const host = bareHost(span.target);
      if (!hostIsService(host, services)) byExternal.set(host, [...(byExternal.get(host) ?? []), span]);
    }
  }
  const lanes: TimelineLane[] = [];
  for (const [name, spans] of byService) lanes.push({ name, external: false, spans });
  for (const [name, spans] of byExternal) lanes.push({ name, external: true, spans });
  const firstStart = (lane: TimelineLane) => Math.min(...lane.spans.map((s) => s.startUs));
  lanes.sort((a, b) => firstStart(a) - firstStart(b));
  return lanes;
}

export interface SequenceRow {
  span: TraceSpan;
  offsetUs: number;
}

// timelineSequence is the window's calls top to bottom: every executed word
// and request boundary in start order, offset from the trace start.
export function timelineSequence(model: TraceModel, windowStartUs: number, windowEndUs: number): SequenceRow[] {
  return model.spans
    .filter(
      (span) =>
        span.startUs < windowEndUs &&
        span.startUs + span.durationUs > windowStartUs &&
        (span.command !== undefined || span.name.startsWith("execute_tool") || span.name.startsWith("machine_request") || span.parentId === undefined),
    )
    .sort((a, b) => a.startUs - b.startUs)
    .map((span) => ({ span, offsetUs: span.startUs - model.startUs }));
}

// ---- The declared-word surface (agentic-wiki-mesh) ----

// An application may ship a surface generated from the declarations its agents
// load. A span records the word it ran; what that word is -- a model call, a
// call to another agent, a store read -- is a property of the program, so a
// surface, when supplied, classifies ahead of any wire heuristic.
export interface DeclaredWord {
  description?: string;
  kind: string;
  states?: string[];
}

export interface DeclaredSurface {
  // Request-machine server name to the agent declaring it.
  servers: Record<string, string>;
  // Word name to its "<agent>/<word>" key, for the words only one agent declares.
  aliases: Record<string, string>;
  words: Record<string, DeclaredWord>;
}

// serviceAgents resolves which agent's program each service runs: a service's
// machine_request span names the server that admitted the request, and the
// surface says which agent declares that server.
export function serviceAgents(spans: TraceSpan[], surface: DeclaredSurface): Map<string, string> {
  const agents = new Map<string, string>();
  for (const span of spans) {
    if (!span.name.startsWith("machine_request ") || agents.has(span.service)) continue;
    const server = span.name.slice("machine_request ".length).split("/")[0];
    const agent = surface.servers[server];
    if (agent) agents.set(span.service, agent);
  }
  return agents;
}

// declaredWord looks a span's word up by the agent running its service, else
// by the name when only one agent declares it. A word the surface does not
// carry resolves to nothing, and the caller falls back.
export function declaredWord(span: TraceSpan, surface: DeclaredSurface, agents: Map<string, string> = new Map()): DeclaredWord | undefined {
  if (!span.command) return undefined;
  const agent = agents.get(span.service);
  if (agent) {
    const scoped = surface.words[`${agent}/${span.command}`];
    if (scoped) return scoped;
  }
  const alias = surface.aliases[span.command];
  return alias ? surface.words[alias] : undefined;
}

// spanStates names the machine states a declared transition runs the word in.
export function spanStates(span: TraceSpan, surface: DeclaredSurface, agents: Map<string, string> = new Map()): string[] {
  return declaredWord(span, surface, agents)?.states ?? [];
}

// spanState is the one state the word runs in when the declaration leaves no
// choice, and nothing when it does: a guess would be worse than a blank.
export function spanState(span: TraceSpan, surface: DeclaredSurface, agents: Map<string, string> = new Map()): string | undefined {
  const states = spanStates(span, surface, agents);
  return states.length === 1 ? states[0] : undefined;
}

export function spanDescription(span: TraceSpan, surface: DeclaredSurface, agents: Map<string, string> = new Map()): string | undefined {
  return declaredWord(span, surface, agents)?.description;
}

// ---- The story categories (GH-517) ----

export type SpanCategory =
  | "input"
  | "step"
  | "tool call"
  | "tool run"
  | "model call"
  | "store call"
  | "agent call"
  | "external call"
  | "handoff"
  | "response";

// Category colors are token references with the kit palette as fallback, so
// they land in inline styles and still follow the application's theme.
export const CATEGORY_COLORS: Record<SpanCategory, string> = {
  input: "var(--chart-blue, #005aff)",
  step: "var(--text-tertiary, #999999)",
  "tool call": "var(--chart-pink, #e03dcd)",
  "tool run": "color-mix(in srgb, var(--chart-teal, #23abb6) 65%, var(--text-primary, #001135))",
  "model call": "var(--chart-purple, #7d33f2)",
  "store call": "var(--chart-yellow, #f7b737)",
  "agent call": "var(--chart-green, #37cc73)",
  "external call": "var(--chart-orange, #747f31)",
  handoff: "var(--chart-teal, #23abb6)",
  response: "var(--chart-red, #e23b3b)",
};

// The wire heuristic used when no declared surface is supplied: which hosts
// are model providers or stores, and which command prefixes run what a model
// selected. Substrings match against the host with its port stripped.
export interface CategoryRules {
  modelHosts: string[];
  storeHosts: string[];
  toolRunPrefixes: string[];
}

export const DEFAULT_CATEGORY_RULES: CategoryRules = {
  modelHosts: ["api.cohere.com", "api.openai.com", "api.anthropic.com", "ollama"],
  storeHosts: ["chroma"],
  toolRunPrefixes: [],
};

// The kind each declared word carries, mapped onto the story vocabulary.
const KIND_CATEGORIES: Record<string, SpanCategory> = {
  "llm tool": "model call",
  handoff: "agent call",
  "store call": "store call",
  "external call": "external call",
  tool: "tool run",
};

export interface CategoryOptions {
  rules?: Partial<CategoryRules>;
  surface?: DeclaredSurface;
  // Service to agent, from serviceAgents; computed by the caller once per trace.
  agents?: Map<string, string>;
}

function isModelCall(span: TraceSpan): boolean {
  return span.attributes?.["gen_ai.operation.name"] === "chat" || span.attributes?.["gen_ai.request.model"] !== undefined;
}

// spanCategory classifies one span for the story. rootService is the agent the
// request entered: its machine_request is the input, any other service's a
// handoff. With a declared surface the declaration decides (and a word it does
// not carry is a plain tool run); without one the host heuristic of the rules
// decides.
export function spanCategory(span: TraceSpan, services: Set<string>, rootService: string, options: CategoryOptions = {}): SpanCategory {
  const structural = span.name.startsWith("machine_request") || (!span.parentId && !span.command);
  if (options.surface) {
    if (isModelCall(span)) return "model call";
    const declared = declaredWord(span, options.surface, options.agents);
    if (declared && KIND_CATEGORIES[declared.kind]) return KIND_CATEGORIES[declared.kind];
    if (structural) return span.service === rootService ? "input" : "handoff";
    return span.command ? "tool run" : "step";
  }
  if (structural) return span.service === rootService ? "input" : "handoff";
  if (span.command?.startsWith("compose_response") || (span.command?.startsWith("compose_") && (span.signal ?? "").includes("ResponseComposed"))) {
    return "response";
  }
  // A forced tool call (GH-710): the span is the model call; the tool call it
  // returns renders as its own follow-on row.
  if (isModelCall(span) || span.command?.endsWith("_via_tool")) return "model call";
  const rules = { ...DEFAULT_CATEGORY_RULES, ...options.rules };
  if (rules.toolRunPrefixes.some((prefix) => span.command?.startsWith(prefix))) return "tool run";
  if (span.target) {
    const host = bareHost(span.target);
    if (rules.modelHosts.some((model) => host === model || host.includes(model))) return "model call";
    if (rules.storeHosts.some((store) => host.includes(store))) return "store call";
    // A peer that is none of the trace's services is outside the mesh.
    return hostIsService(host, services) ? "agent call" : "external call";
  }
  return "step";
}

// spanAnswer extracts the model's answer text from a span for the inline
// answer rows. An invoke_llm span carries it in gen_ai.output.messages; a
// REST-dispatched model call carries the provider response in
// http.response.body, under message.content (chat) or response (Ollama
// generate). Truncation can break the body's JSON, so a lenient scan for the
// first text field backs the parse. Undefined when the span carries no answer.
export function spanAnswer(span: TraceSpan): string | undefined {
  const joined = (content: unknown): string => {
    if (typeof content === "string") return content;
    if (Array.isArray(content)) {
      return content.map((part) => (part && typeof part === "object" && "text" in part ? String((part as { text: unknown }).text) : "")).join("");
    }
    return "";
  };
  const output = span.attributes?.["gen_ai.output.messages"];
  if (typeof output === "string") {
    try {
      const messages = JSON.parse(output);
      if (Array.isArray(messages)) {
        const assistant = messages.find((m: { role?: unknown }) => m && typeof m === "object" && m.role === "assistant");
        const text = joined((assistant as { content?: unknown } | undefined)?.content).trim();
        if (text !== "") return text;
      }
    } catch {
      // fall through to the response body
    }
  }
  const body = span.attributes?.["http.response.body"];
  if (typeof body === "string" && body !== "") {
    try {
      const parsed = JSON.parse(body);
      const message = parsed?.message;
      if (message && typeof message === "object") {
        const text = joined((message as { content?: unknown }).content).trim();
        if (text !== "") return text;
      }
      if (typeof parsed?.response === "string" && parsed.response.trim() !== "") return parsed.response.trim();
    } catch {
      const match = body.match(/"(?:text|content|response)"\s*:\s*"((?:[^"\\]|\\.)*)/);
      if (match) {
        try {
          return (JSON.parse(`"${match[1]}"`) as string).trim() || undefined;
        } catch {
          return match[1].trim() || undefined;
        }
      }
    }
  }
  return undefined;
}

// traceRootService is the agent the request entered: the earliest
// machine_request, not the earliest span, because pod clocks skew.
export function traceRootService(model: TraceModel): string {
  const entry = model.spans.find((span) => span.name.startsWith("machine_request"));
  return entry?.service ?? model.spans[0]?.service ?? "";
}

// ---- Palette and signals ----

// A stable color per service so the waterfall reads as tiers.
export const SERVICE_COLORS = [
  "var(--chart-blue, #005aff)",
  "var(--chart-green, #37cc73)",
  "var(--chart-yellow, #f7b737)",
  "var(--chart-purple, #7d33f2)",
  "var(--chart-red, #e23b3b)",
  "var(--chart-teal, #23abb6)",
];

export function serviceColor(services: string[], service: string): string {
  const idx = Math.max(0, services.indexOf(service));
  return SERVICE_COLORS[idx % SERVICE_COLORS.length];
}

// The signals that mean an iteration went wrong rather than forward.
export const FAILURE_SIGNAL = /Failed|Error|Refused|Denied/;

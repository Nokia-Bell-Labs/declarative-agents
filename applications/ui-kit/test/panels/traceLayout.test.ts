import { describe, expect, it } from "vitest";
import { toModel, type TraceModel, type TraceSpan } from "../../src/api/traceApi";
import { fixtures } from "../../src/fixtures";
import { agentGroups, continuations, stepGroups, windowSpans } from "../../src/panels/TracePanel/traceGroups";
import {
  filterTree,
  groupRootsByService,
  serviceAgents,
  spanAnswer,
  spanCategory,
  spanDescription,
  spanState,
  spanStates,
  spanTree,
  timelineLanes,
  timelineSequence,
  traceRootService,
  type DeclaredSurface,
} from "../../src/panels/TracePanel/traceLayout";

// Ported from cohere-demo traceApi.test.ts and traceGroups.test.ts, plus the
// agentic-wiki-mesh grouping and surface tests over the recorded turn.

const node = (id: string, parentId: string | undefined, startUs: number, command?: string): TraceSpan => ({
  id,
  parentId,
  name: id,
  service: "chatbot",
  startUs,
  durationUs: 10,
  command,
});

describe("spanTree", () => {
  it("nests children under parents in start order and counts descendants", () => {
    const roots = spanTree([node("root", undefined, 0), node("late-child", "root", 50), node("early-child", "root", 10), node("grandchild", "early-child", 20)]);
    expect(roots).toHaveLength(1);
    expect(roots[0].descendants).toBe(3);
    expect(roots[0].children.map((c) => c.span.id)).toEqual(["early-child", "late-child"]);
    expect(roots[0].children[0].children[0].span.id).toBe("grandchild");
  });

  it("roots a span whose parent the trace does not carry", () => {
    const roots = spanTree([node("orphan", "trimmed-ancestor", 5), node("root", undefined, 0)]);
    expect(roots.map((r) => r.span.id)).toEqual(["root", "orphan"]);
  });
});

describe("groupRootsByService", () => {
  it("folds a service's many roots under one group and leaves single roots alone", () => {
    const roots = spanTree([
      { id: "a", name: "a", service: "chatbot", startUs: 10, durationUs: 5 },
      { id: "b", name: "b", service: "chatbot", startUs: 0, durationUs: 5 },
      { id: "c", name: "c", service: "rag0", startUs: 3, durationUs: 5 },
    ]);
    const grouped = groupRootsByService(roots);
    expect(grouped.map((g) => g.span.id)).toEqual(["service-group:chatbot", "c"]);
    expect(grouped[0].descendants).toBe(2);
    expect(grouped[0].span.durationUs).toBe(15);
  });
});

describe("filterTree", () => {
  it("keeps matches and their ancestors, recounting descendants", () => {
    const roots = spanTree([node("root", undefined, 0), node("mid", "root", 0), node("leaf-hit", "mid", 0, "rag_query"), node("leaf-miss", "mid", 0, "embed"), node("other-root", undefined, 0)]);
    const { roots: filtered, matched } = filterTree(roots, (s) => s.command === "rag_query");
    expect(matched).toBe(1);
    expect(filtered).toHaveLength(1);
    expect(filtered[0].span.id).toBe("root");
    expect(filtered[0].descendants).toBe(2);
    expect(filtered[0].children[0].children.map((c) => c.span.id)).toEqual(["leaf-hit"]);
  });

  it("an all-matching filter returns the forest whole", () => {
    const { roots: filtered, matched } = filterTree(spanTree([node("a", undefined, 0), node("b", "a", 0)]), () => true);
    expect(matched).toBe(2);
    expect(filtered[0].descendants).toBe(1);
  });
});

describe("timelineLanes (GH-511)", () => {
  const model: TraceModel = {
    spans: [
      { id: "a", name: "execute_tool embed_query_cohere", service: "chatbot", startUs: 0, durationUs: 100, command: "embed_query_cohere", target: "api.cohere.com", signal: "QueryEmbedded" },
      { id: "b", name: "execute_tool rag_query", service: "rag0", startUs: 120, durationUs: 50, command: "rag_query", target: "demo-chatbot-mesh-rag0-chroma:8000", signal: "DocumentsRead" },
      { id: "c", name: "execute_tool cohere_rerank_rag", service: "chatbot", startUs: 200, durationUs: 80, command: "cohere_rerank_rag", target: "api.cohere.com", signal: "CandidatesReranked" },
    ],
    startUs: 0,
    endUs: 280,
    services: ["chatbot", "rag0"],
    walks: [],
  };

  it("rows are services plus external peers, in first-activity order", () => {
    const lanes = timelineLanes(model, 0, 280);
    expect(lanes.map((l) => l.name)).toEqual(["chatbot", "api.cohere.com", "rag0", "demo-chatbot-mesh-rag0-chroma"]);
    expect(lanes.find((l) => l.name === "api.cohere.com")?.external).toBe(true);
    expect(lanes.find((l) => l.name === "chatbot")?.external).toBe(false);
  });

  it("zooming the window drops inactive agents", () => {
    expect(timelineLanes(model, 110, 180).map((l) => l.name)).toEqual(["rag0", "demo-chatbot-mesh-rag0-chroma"]);
  });

  it("the sequence reads top to bottom with offsets, targets, and signals", () => {
    const sequence = timelineSequence(model, 0, 280);
    expect(sequence.map((r) => r.span.command)).toEqual(["embed_query_cohere", "rag_query", "cohere_rerank_rag"]);
    expect(sequence[0].offsetUs).toBe(0);
    expect(sequence[1].span.target).toContain("chroma");
    expect(sequence[2].span.signal).toBe("CandidatesReranked");
  });
});

describe("spanCategory without a surface (GH-517)", () => {
  const services = new Set(["chatbot", "rag0"]);
  const span = (over: Partial<TraceSpan>): TraceSpan => ({ id: "x", name: "execute_tool w", service: "chatbot", startUs: 0, durationUs: 1, ...over });

  it("reads as the story: input, handoff, calls, steps, response", () => {
    expect(spanCategory(span({ name: "machine_request chatbot_chat/chat" }), services, "chatbot")).toBe("input");
    expect(spanCategory(span({ name: "machine_request rag/query", service: "rag0" }), services, "chatbot")).toBe("handoff");
    expect(spanCategory(span({ command: "embed_query_cohere", target: "api.cohere.com" }), services, "chatbot")).toBe("model call");
    expect(spanCategory(span({ command: "rag_resolve", target: "demo-chatbot-mesh-rag0:18085" }), services, "chatbot")).toBe("agent call");
    expect(spanCategory(span({ command: "rag_get", target: "demo-chatbot-mesh-rag0-chroma:8000", service: "rag0" }), services, "chatbot")).toBe("store call");
    expect(spanCategory(span({ command: "declare_question" }), services, "chatbot")).toBe("step");
    expect(spanCategory(span({ command: "compose_response", signal: "ChatResponseComposed" }), services, "chatbot")).toBe("response");
  });

  it("marks forced tool calls, and the retrieval fan through the application's rules (GH-690)", () => {
    const rules = { toolRunPrefixes: ["rag_query", "osint_search"] };
    expect(spanCategory(span({ command: "select_sources_via_tool", target: "api.cohere.com", parentId: "p" }), services, "chatbot")).toBe("model call");
    expect(spanCategory(span({ command: "rag_query", target: "demo-chatbot-mesh-rag0:18085", parentId: "p" }), services, "chatbot", { rules })).toBe("tool run");
    expect(spanCategory(span({ command: "osint_search", target: "demo-chatbot-mesh-osint-fixture:8090", parentId: "p" }), services, "chatbot", { rules })).toBe("tool run");
    // Without the rule the same call reads by its host: a peer outside the mesh.
    expect(spanCategory(span({ command: "osint_search", target: "demo-chatbot-mesh-osint-fixture:8090", parentId: "p" }), services, "chatbot")).toBe("external call");
  });

  it("reads a span the runtime stamped with GenAI attributes as a model call", () => {
    expect(spanCategory(span({ command: "select_sources", parentId: "p", attributes: { "gen_ai.operation.name": "chat" } }), services, "chatbot")).toBe("model call");
  });
});

describe("spanAnswer (GH-728)", () => {
  const answerSpan = (attributes: Record<string, unknown>): TraceSpan => ({ id: "a", name: "execute_tool w", service: "chatbot", startUs: 0, durationUs: 1, attributes });

  it("reads the assistant text from captured output messages", () => {
    expect(spanAnswer(answerSpan({ "gen_ai.output.messages": '[{"role":"assistant","content":"10\\nSupported."}]' }))).toBe("10\nSupported.");
  });

  it("reads a response body's message envelope, joining content parts, and Ollama bodies", () => {
    expect(
      spanAnswer(answerSpan({ "http.response.body": '{"id":"r1","message":{"role":"assistant","content":[{"type":"text","text":"The brief "},{"type":"text","text":"text."}]},"usage":{}}' })),
    ).toBe("The brief text.");
    expect(spanAnswer(answerSpan({ "http.response.body": '{"model":"qwen2.5:3b","message":{"role":"assistant","content":"The answer."},"done":true}' }))).toBe("The answer.");
    expect(spanAnswer(answerSpan({ "http.response.body": '{"model":"qwen2.5:3b","response":"Generated text.","done":true}' }))).toBe("Generated text.");
  });

  it("extracts the first text field from a truncation-broken body", () => {
    const truncated = '{"id":"r1","message":{"role":"assistant","content":[{"type":"text","text":"Salvaged \\"answer\\" text"}]},"citations":[{"start":1';
    expect(spanAnswer(answerSpan({ "http.response.body": truncated }))).toBe('Salvaged "answer" text');
    expect(spanAnswer(answerSpan({ "http.response.body": '{"message":{"role":"assistant","content":"Salvaged \\"answer\\" text' }))).toBe('Salvaged "answer" text');
  });

  it("stays silent for tool-call outputs and non-text bodies", () => {
    expect(spanAnswer(answerSpan({ "gen_ai.output.messages": '[{"role":"assistant","content":"","tool_calls":[{}]}]' }))).toBeUndefined();
    expect(spanAnswer(answerSpan({ "http.response.body": '{"embeddings":{"float":[[0.1,0.2]]}}' }))).toBeUndefined();
    expect(spanAnswer(answerSpan({}))).toBeUndefined();
  });
});

describe("step and agent groups", () => {
  const model = (spans: TraceSpan[]): TraceModel => ({
    spans,
    startUs: Math.min(...spans.map((s) => s.startUs)),
    endUs: Math.max(...spans.map((s) => s.startUs + s.durationUs)),
    services: [...new Set(spans.map((s) => s.service))],
    walks: [],
  });
  const span = (over: Partial<TraceSpan> & { id: string; service: string; startUs: number }): TraceSpan => ({ name: `execute_tool ${over.command ?? over.id}`, durationUs: 10, attributes: {}, ...over });
  const root = span({ id: "root", service: "chatbot", startUs: 0, durationUs: 1000, name: "machine_request chatbot_chat/chat" });
  const stepA = span({ id: "a", service: "chatbot", startUs: 10, command: "embed_query_cohere", parentId: "root" });
  const ragChild = span({ id: "r1", service: "rag0", startUs: 15, command: "rag_query", parentId: "a" });
  const stepB = span({ id: "b", service: "chatbot", startUs: 40, command: "compose_answer", parentId: "root" });
  const all = [root, stepA, ragChild, stepB];

  it("anchors on the root machine's commands and files other agents' spans under them", () => {
    const groups = stepGroups(model(all), 0, 2000);
    expect(groups.map((g) => g.anchor.command)).toEqual(["embed_query_cohere", "compose_answer"]);
    expect(groups[0].spans.map((s) => s.id)).toEqual(["root", "a", "r1"]);
    expect(groups[1].spans.map((s) => s.id)).toEqual(["b"]);
  });

  it("respects the brush window, and reads a window with no anchor as one group", () => {
    expect(stepGroups(model(all), 0, 30).map((g) => g.anchor.command)).toEqual(["embed_query_cohere"]);
    expect(windowSpans(model([stepA, stepB]), 0, 30).map((s) => s.id)).toEqual(["a"]);
    expect(stepGroups(model(all), 15, 20).map((g) => g.spans.map((s) => s.id))).toEqual([["r1"]]);
    expect(stepGroups(model(all), 900, 950)).toEqual([]);
  });

  it("groups the window's spans by emitting service in first-appearance order", () => {
    const groups = agentGroups(model(all), 0, 2000);
    expect(groups.map((g) => g.service)).toEqual(["chatbot", "rag0"]);
    expect(groups[1].spans.map((s) => s.id)).toEqual(["r1"]);
  });

  it("links a span to its cross-service parent and children only", () => {
    expect(continuations(stepA, all)).toEqual([{ direction: "sent", span: ragChild }]);
    expect(continuations(ragChild, all)).toEqual([{ direction: "received", span: stepA }]);
    expect(continuations(stepB, all)).toEqual([]);
  });
});

describe("the recorded turn", () => {
  const trace = toModel(fixtures["/query/traces/{trace_id}"]);
  const byCommand = (command: string, service = "chatbot") => trace.spans.find((span) => span.command === command && span.service === service)!;

  it("makes one step per dispatch of the chatbot's machine, in order", () => {
    const groups = stepGroups(trace, trace.startUs, trace.endUs + 1);
    const commands = groups.map((g) => g.anchor.command);
    expect(commands[0]).toBe("capture_request");
    expect(commands[commands.length - 1]).toBe("compose_response");
    expect(groups.every((g) => g.anchor.service === "chatbot")).toBe(true);
  });

  it("reads the knowledge manager's work inside the step that called it", () => {
    const groups = stepGroups(trace, trace.startUs, trace.endUs + 1);
    const step = groups.find((g) => g.anchor.command === "knowledge_query")!;
    expect(step.spans.filter((s) => s.service === "knowledge0").map((s) => s.command)).toEqual([undefined, undefined, "knowledge_resolve", "knowledge_query"]);
    expect(groups.filter((g) => g !== step).flatMap((g) => g.spans).filter((s) => s.service === "knowledge0")).toEqual([]);
  });

  it("links the chatbot's call to the knowledge manager's machine and back", () => {
    const call = byCommand("knowledge_query");
    const sent = continuations(call, trace.spans);
    expect(sent).toHaveLength(1);
    expect(sent[0]).toMatchObject({ direction: "sent", span: { service: "knowledge0" } });
    expect(continuations(sent[0].span, trace.spans)[0]).toEqual({ direction: "received", span: call });
  });

  // An excerpt of the surface agentic-wiki-mesh generates for this turn.
  const surface: DeclaredSurface = {
    servers: { chatbot_chat: "chatbot", chroma_knowledge_requests: "knowledge-manager" },
    aliases: { select_sources: "chatbot/select_sources", embed_query: "chatbot/embed_query", knowledge_resolve: "knowledge-manager/knowledge_resolve", compose_response: "chatbot/compose_response", capture_request: "chatbot/capture_request" },
    words: {
      "chatbot/select_sources": { description: "Select declared knowledge source names for the original user question.", kind: "llm tool", states: ["ComposingSourceSelection"] },
      "chatbot/embed_query": { kind: "llm tool", states: ["DefaultingCitations"] },
      "chatbot/knowledge_query": { kind: "handoff", states: ["CheckingView"] },
      "chatbot/capture_request": { kind: "tool", states: ["AwaitingRequest"] },
      "chatbot/compose_response": { kind: "tool", states: ["DeclaringPageNone", "DeclaringPageSkipped", "DeclaringPageWritten"] },
      "knowledge-manager/knowledge_query": { kind: "store call", states: ["ResolvingCollection"] },
      "knowledge-manager/knowledge_resolve": { kind: "store call", states: ["AwaitingRequest"] },
    },
  };
  const agents = serviceAgents(trace.spans, surface);
  const root = traceRootService(trace);
  const services = new Set(trace.services);
  const categoryOf = (command: string, service = "chatbot") => spanCategory(byCommand(command, service), services, root, { surface, agents });

  it("resolves each service to the agent whose program it runs", () => {
    expect(root).toBe("chatbot");
    expect(agents.get("knowledge0")).toBe("knowledge-manager");
  });

  it("classifies by the declaration when a surface is supplied, not by host", () => {
    expect(categoryOf("select_sources")).toBe("model call");
    expect(categoryOf("embed_query")).toBe("model call");
    expect(categoryOf("knowledge_query")).toBe("agent call");
    expect(categoryOf("knowledge_query", "knowledge0")).toBe("store call");
    expect(categoryOf("compose_response")).toBe("tool run");
    const undeclared: TraceSpan = { id: "u", name: "execute_tool invent", service: "chatbot", startUs: 0, durationUs: 1, command: "invent", target: "api.cohere.com", parentId: "p" };
    expect(spanCategory(undeclared, services, root, { surface, agents })).toBe("tool run");
  });

  it("names the state a word ran in, and none for a word several branches dispatch", () => {
    expect(spanState(byCommand("select_sources"), surface, agents)).toBe("ComposingSourceSelection");
    expect(spanState(byCommand("knowledge_query", "knowledge0"), surface, agents)).toBe("ResolvingCollection");
    expect(spanStates(byCommand("compose_response"), surface, agents)).toHaveLength(3);
    expect(spanState(byCommand("compose_response"), surface, agents)).toBeUndefined();
    expect(spanDescription(byCommand("select_sources"), surface, agents)).toContain("Select declared knowledge source names");
  });
});

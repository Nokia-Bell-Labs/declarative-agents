import { describe, expect, it } from "vitest";
import { createKitClient } from "../src/client/client";
import { fetchTrace, fetchTraceList, fetchTraces, isErrorSpan, isSingleSpanTrace, readableTraces, toListPage, toModel, traceBackendLabel, traceListPath, traceQueryPath } from "../src/api/traceApi";
import { fixtureFetch, fixtures } from "../src/fixtures";

// Spans as the collector serves them: attributes are an array of typed pairs.
function span(service: string, name: string, attributes: Record<string, unknown>, index: number) {
  return {
    span_id: `s${index}`,
    name,
    service,
    start_time: new Date(1000 + index).toISOString(),
    end_time: new Date(1100 + index).toISOString(),
    attributes: Object.entries(attributes).map(([Key, Value]) => ({ Key, Value: { Type: typeof Value === "number" ? "INT64" : "STRING", Value } })),
  };
}

describe("trace paths", () => {
  it("reads the declared backend through the monitor proxy (srd004 R2.4)", () => {
    expect(traceQueryPath("collector", "abc/def")).toBe("/monitor-proxy/collector/query/traces/abc%2Fdef");
    expect(traceListPath("spool", 50, 0)).toBe("/monitor-proxy/spool/query/traces?page_size=50&offset=0");
  });

  it("reads a backend that starts with / as a same-origin path prefix (srd004 R2.4)", () => {
    expect(traceQueryPath("/", "abc")).toBe("/query/traces/abc");
    expect(traceListPath("/", 20, 40)).toBe("/query/traces?page_size=20&offset=40");
    expect(traceQueryPath("/monitor-proxy/collector", "abc")).toBe(traceQueryPath("collector", "abc"));
    expect(traceListPath("/monitor-proxy/collector/", 50, 0)).toBe(traceListPath("collector", 50, 0));
    expect(traceBackendLabel("collector")).toBe("collector");
    expect(traceBackendLabel("/")).toBe("at /query/traces");
  });
});

describe("toModel", () => {
  it("folds the provider adapter's twin into the dispatch span it wraps", () => {
    const dispatch = span("chatbot", "chat qwen2.5:3b", { "command.name": "invoke_llm_fast", "command.signal": "LLMResponded", "server.address": "host.docker.internal" }, 0);
    const adapter = { ...span("chatbot", "chat qwen2.5:3b", { "gen_ai.usage.input_tokens": 601, "server.address": "host.docker.internal" }, 1), parent_span_id: "s0" };
    const child = { ...span("chatbot", "execute_tool after", { "command.name": "after" }, 2), parent_span_id: "s1" };
    const model = toModel({ trace_id: "t1", span_count: 3, spans: [dispatch, adapter, child] });
    const chats = model.spans.filter((s) => s.name.startsWith("chat "));
    expect(chats).toHaveLength(1);
    expect(chats[0].attributes?.["gen_ai.usage.input_tokens"]).toBe(601);
    expect(model.spans.find((s) => s.command === "after")?.parentId).toBe("s0");
  });

  it("builds one ordered ladder per service from the execute_tool spans", () => {
    const spans = [
      span("chatbot", "execute_tool b", { "command.name": "b", iteration: 2, "command.signal": "B" }, 1),
      span("chatbot", "execute_tool a", { "command.name": "a", iteration: 1, "command.signal": "A" }, 0),
      span("rag0", "execute_tool q", { "command.name": "q", iteration: 1 }, 2),
    ];
    const model = toModel({ trace_id: "t", span_count: 3, spans });
    expect(model.walks).toEqual([
      { service: "chatbot", steps: [{ iteration: 1, command: "a", signal: "A" }, { iteration: 2, command: "b", signal: "B" }] },
      { service: "rag0", steps: [{ iteration: 1, command: "q", signal: "" }] },
    ]);
  });

  it("carries the span status in either spelling and marks code 2 as an error", () => {
    const spans = [
      { ...span("chatbot", "execute_tool a", { "command.name": "a" }, 0), status: { Code: 2, Description: "selector resolved to <nil>" } },
      { ...span("chatbot", "execute_tool b", { "command.name": "b" }, 1), status: { code: 1 } },
      { ...span("chatbot", "execute_tool c", { "command.name": "c" }, 2), status: { Code: 0, Description: "" } },
      span("chatbot", "execute_tool d", { "command.name": "d" }, 3),
    ];
    const model = toModel({ trace_id: "t", span_count: 4, spans });
    expect(model.spans.map((s) => s.status)).toEqual([{ code: 2, description: "selector resolved to <nil>" }, { code: 1 }, { code: 0 }, undefined]);
    expect(model.spans.map(isErrorSpan)).toEqual([true, false, false, false]);
  });

  it("keeps the recorded error spans", () => {
    const failed = toModel(fixtures["/query/traces/{trace_id}"]).spans.filter(isErrorSpan);
    expect(failed.length).toBeGreaterThan(0);
    expect(failed.find((s) => s.command === "flatten_citations")?.status?.description).toContain("flatten_citations");
  });

  it("models the recorded turn", () => {
    const model = toModel(fixtures["/query/traces/{trace_id}"]);
    expect(model.spans.length).toBeGreaterThan(0);
    expect(model.spans.length).toBeLessThanOrEqual(fixtures["/query/traces/{trace_id}"].span_count);
    expect(model.services).toContain("chatbot");
    expect(model.walks.length).toBeGreaterThan(0);
    expect(model.endUs).toBeGreaterThan(model.startUs);
  });
});

describe("the trace list", () => {
  it("maps the recorded page and hides single-span traces", () => {
    const page = toListPage(fixtures["/query/traces"], 0, 50);
    expect(page.total).toBe(2);
    expect(page.traces[0]).toMatchObject({ rootService: "chatbot", spanCount: fixtures["/query/traces/{trace_id}"].span_count });
    expect(readableTraces(page).map((t) => t.traceId)).toEqual([fixtures["/query/traces"].traces[0].trace_id]);
  });

  it("falls back to the requested page when the envelope omits its counts", () => {
    const page = toListPage({ traces: [{ trace_id: "t1", span_count: 3 }] } as never, 100, 25);
    expect([page.total, page.offset, page.pageSize]).toEqual([1, 100, 25]);
    expect(toListPage(undefined, 0, 25).traces).toEqual([]);
    expect(isSingleSpanTrace({ traceId: "x", rootService: "", rootSpanName: "", spanCount: 0, startTime: "", durationMs: 0 })).toBe(true);
  });
});

describe("trace fetches", () => {
  const client = createKitClient({ fetch: fixtureFetch() });

  it("reads one trace, a page, and several traces", async () => {
    const one = await fetchTrace(client, "collector", "any");
    expect(one.status).toBe("ok");
    const list = await fetchTraceList(client, "collector", 50, 0);
    expect(list.status === "ok" && list.page.traces).toHaveLength(2);
    expect((await fetchTraces(client, "collector", ["a", "b"])).size).toBe(2);
  });

  it("degrades: 404 is empty, a failure or absent backend is unavailable", async () => {
    const status = (code: number) => createKitClient({ fetch: (async () => new Response("x", { status: code })) as typeof fetch });
    expect(await fetchTrace(status(404), "collector", "t")).toEqual({ status: "empty" });
    expect(await fetchTrace(status(502), "collector", "t")).toMatchObject({ status: "unavailable" });
    const down = createKitClient({ fetch: (async () => { throw new Error("refused"); }) as typeof fetch });
    expect(await fetchTrace(down, "collector", "t")).toEqual({ status: "unavailable", reason: "refused" });
    expect(await fetchTraceList(down, "collector", 50, 0)).toEqual({ status: "unavailable", reason: "refused" });
    expect((await fetchTraces(down, "collector", ["a"])).size).toBe(0);
  });
});

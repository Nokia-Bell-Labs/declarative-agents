// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { toModel } from "../../src/api/traceApi";
import { createKitClient } from "../../src/client/client";
import { KitClientProvider } from "../../src/client/context";
import { fixtures } from "../../src/fixtures";
import { SpanDetail, TraceList, TracePanel, tracePanel, tracePanelManifest, TraceStoryOverlay, TraceView, Waterfall, type DeclaredSurface } from "../../src/panels/TracePanel";
import { FixtureProvider } from "./support";

const TRACE_ID = fixtures["/query/traces/{trace_id}"].trace_id;
const trace = toModel(fixtures["/query/traces/{trace_id}"]);

afterEach(cleanup);

// A client whose every request answers from one function, for the 404,
// failure, and paging cases the shared fixtures do not carry.
function ClientWith({ respond, children }: { respond: (url: URL) => Response | Promise<Response>; children: ReactNode }) {
  const client = createKitClient({ fetch: async (input) => respond(new URL(String(input), "http://fixture.invalid")) });
  return <KitClientProvider client={client}>{children}</KitClientProvider>;
}

describe("TracePanel", () => {
  it("renders the timeline, the walks, and the span tree for the recorded turn", async () => {
    render(
      <FixtureProvider>
        <TracePanel traceId={TRACE_ID} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("trace-timeline")).toBeTruthy());
    expect(screen.getAllByTestId("trace-lane").length).toBeGreaterThan(1);
    expect(screen.getByTestId("trace-walks")).toBeTruthy();
    expect(screen.getByTestId("trace-panel").classList.contains("dak-trace")).toBe(true);
    fireEvent.click(screen.getByTestId("trace-tree-toggle"));
    expect(screen.getByTestId("trace-waterfall")).toBeTruthy();
    expect(screen.getAllByTestId("trace-row").length).toBeGreaterThan(10);
    // A sequence row opens the span's content.
    fireEvent.click(within(screen.getByTestId("trace-sequence")).getByText("invoke_cohere_command"));
    expect(within(screen.getByTestId("span-detail")).getByText("http.request.body")).toBeTruthy();
  });

  it("groups by step with state labels and answers when the application passes its surface", async () => {
    const surface: DeclaredSurface = {
      servers: { chatbot_chat: "chatbot" },
      aliases: { select_sources: "chatbot/select_sources" },
      words: { "chatbot/select_sources": { kind: "llm tool", states: ["ComposingSourceSelection"], description: "Select sources." } },
    };
    render(
      <FixtureProvider>
        <TracePanel traceId={TRACE_ID} timelineBody="steps" surface={surface} describe={(command) => (command === "cohere_rerank" ? "Rerank the chunks." : undefined)} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("trace-steps")).toBeTruthy());
    expect(screen.getAllByTestId("step-group").length).toBeGreaterThan(20);
    const select = screen.getAllByTestId("trace-call").find((row) => row.dataset.command === "select_sources")!;
    expect(select.dataset.category).toBe("model call");
    expect(select.dataset.state).toBe("ComposingSourceSelection");
    expect(screen.getAllByText("Rerank the chunks.").length).toBeGreaterThan(0);
    expect(screen.getAllByTestId("trace-answer").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/continues at knowledge0/).length).toBeGreaterThan(0);
  });

  it("shows the empty notice for a trace the backend does not hold", async () => {
    render(
      <ClientWith respond={() => new Response("not found", { status: 404 })}>
        <TracePanel traceId="missing" />
      </ClientWith>,
    );
    await waitFor(() => expect(screen.getByTestId("trace-empty")).toBeTruthy());
  });

  it("warns without naming a deployment tool when the backend is unreachable", async () => {
    render(
      <ClientWith respond={() => Promise.reject(new Error("connection refused"))}>
        <TracePanel traceId={TRACE_ID} backend="tracer" />
      </ClientWith>,
    );
    const notice = await screen.findByTestId("trace-unavailable");
    expect(notice.textContent).toContain("tracer");
    expect(notice.textContent).toContain("connection refused");
    expect(notice.textContent).not.toMatch(/helm|chart values/i);
  });

  it("reads through the proxy of the declared backend", async () => {
    const seen: string[] = [];
    render(
      <ClientWith
        respond={(url) => {
          seen.push(url.pathname);
          return Response.json(fixtures["/query/traces/{trace_id}"]);
        }}
      >
        <TracePanel traceId={TRACE_ID} backend="tracer" />
      </ClientWith>,
    );
    await screen.findByTestId("trace-timeline");
    expect(seen).toEqual([`/monitor-proxy/tracer/query/traces/${TRACE_ID}`]);
  });

  it("reads same-origin when the backend is a path prefix", async () => {
    const seen: string[] = [];
    render(
      <ClientWith
        respond={(url) => {
          seen.push(url.pathname);
          return Response.json(fixtures["/query/traces/{trace_id}"]);
        }}
      >
        <TracePanel traceId={TRACE_ID} backend="/" />
      </ClientWith>,
    );
    await screen.findByTestId("trace-timeline");
    expect(seen).toEqual([`/query/traces/${TRACE_ID}`]);
  });

  it("marks the recorded error spans in the timeline, the span tree, and the span content", async () => {
    render(
      <FixtureProvider>
        <TracePanel traceId={TRACE_ID} />
      </FixtureProvider>,
    );
    const sequence = await screen.findByTestId("trace-sequence");
    const failed = within(sequence).getByText("flatten_citations").closest("button")!;
    expect(failed.classList.contains("span-error")).toBe(true);
    expect(within(failed).getByTestId("span-error").getAttribute("title")).toContain("resolved to <nil>");
    expect(screen.getAllByTestId("trace-lane").some((lane) => lane.querySelector(".timeline-bar.span-error"))).toBe(true);
    fireEvent.click(failed);
    expect(within(screen.getByTestId("span-detail")).getByTestId("span-detail-status").textContent).toContain("resolved to <nil>");
  });
});

describe("Waterfall", () => {
  it("marks a failed span's row and bar", () => {
    render(<Waterfall trace={trace} />);
    const rows = screen.getAllByTestId("trace-row").filter((row) => row.classList.contains("span-error"));
    expect(rows.length).toBe(trace.spans.filter((span) => span.status?.code === 2).length);
    expect(rows[0].querySelector(".trace-bar.span-error")).toBeTruthy();
    expect(within(rows[0]).getByTestId("span-error")).toBeTruthy();
  });
});

describe("SpanDetail", () => {
  const drawer = (command: string) => {
    const span = trace.spans.find((s) => s.command === command)!;
    return render(<SpanDetail span={span} model={trace} onClose={() => {}} />).container;
  };

  it("reads a model call's messages by role, the prompt open and the preamble folded", () => {
    const html = drawer("select_sources").innerHTML;
    expect(html).toContain("gen_ai.input.messages");
    expect(html).toMatch(/<details class="span-message"><summary>system<\/summary>/);
    expect(html).toMatch(/<details class="span-message" open=""><summary>user<\/summary>/);
  });

  it("reads an embed or a rerank as the request and response they are", () => {
    const html = drawer("cohere_rerank").innerHTML;
    expect(html).toContain("<summary>request</summary>");
    expect(html).toContain("<summary>response</summary>");
    expect(html).toContain("relevance_score");
  });

  it("puts the content-bearing attributes first", () => {
    const html = drawer("invoke_cohere_command").innerHTML;
    expect(html.indexOf("http.request.body")).toBeLessThan(html.indexOf("command.duration_ms"));
  });
});

describe("TraceList", () => {
  it("lists the readable trace and counts the hidden single-span one", async () => {
    const opened: string[] = [];
    render(
      <FixtureProvider>
        <TraceList backend="collector" onOpen={(id) => opened.push(id)} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getAllByTestId("trace-list-row")).toHaveLength(1));
    expect(screen.getByTestId("trace-list-hidden").textContent).toContain("1 single-span hidden");
    fireEvent.click(screen.getByTestId("trace-list-show-all"));
    expect(screen.getAllByTestId("trace-list-row")).toHaveLength(2);
    fireEvent.click(screen.getAllByTestId("trace-list-row")[0]);
    expect(opened).toEqual([TRACE_ID]);
  });

  it("says so when the backend holds no traces", async () => {
    render(
      <FixtureProvider overrides={{ "/query/traces": { traces: [], total: 0, offset: 0, page_size: 50 } }}>
        <TraceList backend="collector" onOpen={() => {}} />
      </FixtureProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("trace-list-empty")).toBeTruthy());
    expect(screen.getByTestId("trace-list-range").textContent).toBe("0 of 0");
  });

  it("pages through the backend's list", async () => {
    const row = fixtures["/query/traces"].traces[0];
    const offsets: string[] = [];
    render(
      <ClientWith
        respond={(url) => {
          const offset = Number(url.searchParams.get("offset"));
          offsets.push(String(offset));
          const traces = Array.from({ length: offset === 0 ? 2 : 1 }, (_, i) => ({ ...row, trace_id: `t${offset + i}` }));
          return Response.json({ traces, total: 3, offset, page_size: 2 });
        }}
      >
        <TraceList backend="collector" pageSize={2} onOpen={() => {}} />
      </ClientWith>,
    );
    await waitFor(() => expect(screen.getByTestId("trace-list-range").textContent).toBe("1–2 of 3"));
    fireEvent.click(screen.getByText("older ›"));
    await waitFor(() => expect(screen.getByTestId("trace-list-range").textContent).toBe("3–3 of 3"));
    expect(screen.getAllByTestId("trace-list-row").map((node) => node.dataset.trace)).toEqual(["t2"]);
    expect((screen.getByText("older ›") as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByText("‹ newer"));
    await waitFor(() => expect(screen.getByTestId("trace-list-range").textContent).toBe("1–2 of 3"));
    expect(offsets).toEqual(["0", "2", "0"]);
  });

  it("warns when the backend is not deployed", async () => {
    render(
      <FixtureProvider absentAgents={["collector"]}>
        <TraceList backend="collector" onOpen={() => {}} />
      </FixtureProvider>,
    );
    const notice = await screen.findByTestId("trace-list-unavailable");
    expect(notice.textContent).toContain("HTTP 404");
    expect(notice.textContent).not.toMatch(/helm|chart values/i);
  });
});

describe("the mounted trace panel", () => {
  it("declares the trace endpoints and no monitored agent", () => {
    expect(tracePanel.manifest).toBe(tracePanelManifest);
    expect(tracePanelManifest).toEqual({ id: "trace", title: "Traces", route: "/traces", required_endpoints: ["/query/traces", "/query/traces/{trace_id}"], monitored_agents: [] });
  });

  it("opens a listed trace and returns to the list", async () => {
    const Mount = tracePanel.component;
    render(
      <FixtureProvider>
        <Mount monitoredAgents={[]} />
      </FixtureProvider>,
    );
    fireEvent.click(await screen.findByTestId("trace-list-row"));
    await screen.findByTestId("trace-timeline");
    expect(screen.queryByTestId("trace-list")).toBeNull();
    fireEvent.click(screen.getByTestId("trace-back"));
    await screen.findByTestId("trace-list-row");
  });
});

describe("TraceView", () => {
  it("follows openTraceId and reports opens and the way back through onOpen", async () => {
    const opened: Array<string | undefined> = [];
    const view = (openTraceId?: string) => (
      <FixtureProvider>
        <TraceView openTraceId={openTraceId} onOpen={(id) => opened.push(id)} />
      </FixtureProvider>
    );
    const { rerender } = render(view());
    fireEvent.click(await screen.findByTestId("trace-list-row"));
    // Controlled: the click is reported, the list stays until the prop moves.
    expect(opened).toEqual([TRACE_ID]);
    expect(screen.getByTestId("trace-list")).toBeTruthy();
    rerender(view(TRACE_ID));
    await screen.findByTestId("trace-timeline");
    fireEvent.click(screen.getByTestId("trace-back"));
    expect(opened).toEqual([TRACE_ID, undefined]);
    expect(screen.getByTestId("trace-timeline")).toBeTruthy();
  });
});

describe("TraceStoryOverlay", () => {
  const chats = [
    { chatId: "c1", title: "First chat", turns: [{ traceId: TRACE_ID, label: "how are embeddings stored?" }] },
    { chatId: "c2", title: "Second chat", turns: [{ traceId: "other", label: "second question" }] },
  ];

  it("shows the application's chats and the focused turn by step and by agent", async () => {
    let closed = false;
    render(
      <FixtureProvider>
        <TraceStoryOverlay chats={chats} currentChatId="c1" focusTraceId={TRACE_ID} toolNames={{}} onClose={() => (closed = true)} />
      </FixtureProvider>,
    );
    expect(screen.getByTestId("trace-overlay").parentElement?.classList.contains("dak-trace")).toBe(true);
    expect(screen.getByText("how are embeddings stored?")).toBeTruthy();
    await waitFor(() => expect(screen.getByTestId("trace-steps")).toBeTruthy());
    fireEvent.click(screen.getByRole("tab", { name: "Agents" }));
    fireEvent.click(screen.getByText("knowledge0"));
    expect(screen.getAllByTestId("trace-call").some((row) => row.dataset.command === "knowledge_resolve")).toBe(true);
    fireEvent.click(screen.getByText("✕ close"));
    expect(closed).toBe(true);
  });

  it("says so when no chat carries a trace", () => {
    render(
      <FixtureProvider>
        <TraceStoryOverlay chats={[]} onClose={() => {}} emptyHint="Nothing traced yet." />
      </FixtureProvider>,
    );
    expect(screen.getByText("Nothing traced yet.")).toBeTruthy();
  });
});

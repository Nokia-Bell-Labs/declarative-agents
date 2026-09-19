import { proxyPath, type KitClient } from "../client/client";

// The trace data layer: the collector's query surface (catalog srd020 R5) read
// through the proxy of the agent ui.yaml names as the trace backend (srd004
// R2.4). Merged from cohere-demo (get-by-id, several-at-once) and
// agentic-wiki-mesh (the paged list). Presentation helpers built on TraceModel
// ship with TracePanel.

export interface TraceSpan {
  id: string;
  parentId?: string;
  name: string;
  service: string;
  startUs: number;
  durationUs: number;
  // The declared word this span executed, from the command.name attribute.
  command?: string;
  // The remote peer a boundary span called (server.address).
  target?: string;
  // The signal the command emitted.
  signal?: string;
  // Every collected attribute, so a detail view needs no second fetch.
  attributes?: Record<string, unknown>;
}

// One step of a request-scoped machine's walk: the command an iteration ran and
// the signal it emitted.
export interface WalkStep {
  iteration: number;
  command: string;
  signal: string;
}

export interface ServiceWalk {
  service: string;
  steps: WalkStep[];
}

export interface TraceModel {
  spans: TraceSpan[];
  startUs: number;
  endUs: number;
  services: string[];
  walks: ServiceWalk[];
}

export type TraceState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "ok"; trace: TraceModel }
  | { status: "empty" }
  | { status: "unavailable"; reason: string };

// The collector serves attributes as an array of typed pairs, not a map.
export interface CollectorAttribute {
  Key: string;
  Value: { Type: string; Value: unknown };
}

export interface CollectorSpan {
  span_id: string;
  parent_span_id?: string;
  name: string;
  service: string;
  start_time: string;
  end_time: string;
  status?: { code?: number; description?: string };
  attributes?: CollectorAttribute[];
}

// CollectorTrace is GET /query/traces/{trace_id} (srd020 R5.2).
export interface CollectorTrace {
  trace_id: string;
  spans: CollectorSpan[];
  span_count: number;
}

export interface CollectorTraceSummary {
  trace_id: string;
  root_service: string;
  root_span_name: string;
  span_count: number;
  start_time: string;
  duration_ms: number;
}

// CollectorTraceList is GET /query/traces?page_size=&offset= (srd020 R5.1).
export interface CollectorTraceList {
  traces: CollectorTraceSummary[];
  total: number;
  offset: number;
  page_size: number;
}

export interface TraceSummary {
  traceId: string;
  rootService: string;
  rootSpanName: string;
  spanCount: number;
  startTime: string;
  durationMs: number;
}

export interface TraceListPage {
  traces: TraceSummary[];
  total: number;
  offset: number;
  pageSize: number;
}

export type TraceListState =
  | { status: "loading" }
  | { status: "ok"; page: TraceListPage }
  | { status: "unavailable"; reason: string };

export function traceQueryPath(backend: string, traceId: string): string {
  return proxyPath(backend, `query/traces/${encodeURIComponent(traceId)}`);
}

export function traceListPath(backend: string, pageSize: number, offset: number): string {
  return proxyPath(backend, `query/traces?page_size=${pageSize}&offset=${offset}`);
}

function attributeMap(span: CollectorSpan): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const attribute of span.attributes ?? []) {
    out[attribute.Key] = attribute.Value?.Value;
  }
  return out;
}

function isoToUs(iso: string): number {
  return new Date(iso).getTime() * 1000;
}

function nonEmpty(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

// One model call, one span. An invoke_llm word emits two spans for a single
// HTTP request: the machine's dispatch span, carrying the command, signal, and
// captured prompt, and the provider adapter's child with the same name and
// window, carrying wire metadata only. The child folds into its parent here,
// once, before any view sees the trace; the parent's own values win.
function mergeAdapterDuplicates(spans: TraceSpan[]): void {
  const byId = new Map(spans.map((span) => [span.id, span]));
  const absorbed = new Set<string>();
  for (const span of spans) {
    if (span.command !== undefined || span.parentId === undefined) continue;
    const parent = byId.get(span.parentId);
    if (!parent || parent.name !== span.name) continue;
    parent.attributes = { ...(span.attributes ?? {}), ...(parent.attributes ?? {}) };
    if (parent.target === undefined) parent.target = span.target;
    absorbed.add(span.id);
  }
  if (absorbed.size === 0) return;
  for (const span of spans) {
    if (span.parentId !== undefined && absorbed.has(span.parentId)) {
      span.parentId = byId.get(span.parentId)?.parentId;
    }
  }
  for (let i = spans.length - 1; i >= 0; i--) {
    if (absorbed.has(spans[i].id)) spans.splice(i, 1);
  }
}

export function toModel(trace: CollectorTrace): TraceModel {
  const spans: TraceSpan[] = trace.spans.map((s) => {
    const startUs = isoToUs(s.start_time);
    const endUs = isoToUs(s.end_time);
    const attributes = attributeMap(s);
    return {
      id: s.span_id,
      parentId: s.parent_span_id || undefined,
      name: s.name,
      service: s.service,
      startUs,
      durationUs: endUs - startUs,
      command: nonEmpty(attributes["command.name"]),
      target: nonEmpty(attributes["server.address"]),
      signal: nonEmpty(attributes["command.signal"]),
      attributes,
    };
  });
  spans.sort((a, b) => a.startUs - b.startUs);
  mergeAdapterDuplicates(spans);
  const startUs = Math.min(...spans.map((s) => s.startUs));
  const endUs = Math.max(...spans.map((s) => s.startUs + s.durationUs));
  const services = Array.from(new Set(spans.map((s) => s.service)));

  // The walk: every execute_tool span carries the iteration, the command it
  // ran, and the signal the command emitted, one ladder per service.
  const bySvc = new Map<string, WalkStep[]>();
  for (const span of trace.spans) {
    if (!span.name.startsWith("execute_tool")) continue;
    const attributes = attributeMap(span);
    const command = attributes["command.name"];
    const iteration = Number(attributes["iteration"]);
    if (typeof command !== "string" || command === "" || !Number.isFinite(iteration)) continue;
    const signal = attributes["command.signal"];
    const steps = bySvc.get(span.service) ?? [];
    steps.push({ iteration, command, signal: typeof signal === "string" ? signal : "" });
    bySvc.set(span.service, steps);
  }
  const walks: ServiceWalk[] = services
    .filter((service) => bySvc.has(service))
    .map((service) => ({
      service,
      steps: (bySvc.get(service) ?? []).sort((a, b) => a.iteration - b.iteration),
    }));
  return { spans, startUs, endUs, services, walks };
}

// toListPage maps one wire page to the model the list renders. The collector
// omits a field rather than sending a zero, and a row with no span count must
// not read as a readable trace, so the defaults fall to 0.
export function toListPage(body: CollectorTraceList | undefined, offset: number, pageSize: number): TraceListPage {
  const rows = body?.traces ?? [];
  return {
    traces: rows.map((row) => ({
      traceId: row.trace_id,
      rootService: row.root_service ?? "",
      rootSpanName: row.root_span_name ?? "",
      spanCount: row.span_count ?? 0,
      startTime: row.start_time ?? "",
      durationMs: row.duration_ms ?? 0,
    })),
    total: body?.total ?? rows.length,
    offset: body?.offset ?? offset,
    pageSize: body?.page_size ?? pageSize,
  };
}

// A single-span trace recorded no work worth reading as steps (heartbeats), so
// lists hide them by default and show how many are hidden.
export function isSingleSpanTrace(summary: TraceSummary): boolean {
  return summary.spanCount <= 1;
}

export function readableTraces(page: TraceListPage): TraceSummary[] {
  return page.traces.filter((summary) => !isSingleSpanTrace(summary));
}

function reason(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

// fetchTrace reads one trace. It degrades to "unavailable" when the backend
// cannot be reached and to "empty" when it holds no spans for the id yet.
export async function fetchTrace(client: KitClient, backend: string, traceId: string): Promise<TraceState> {
  let res: Response;
  try {
    res = await client.request(traceQueryPath(backend, traceId));
  } catch (err) {
    return { status: "unavailable", reason: reason(err) };
  }
  if (res.status === 404) return { status: "empty" };
  if (!res.ok) return { status: "unavailable", reason: `trace backend returned HTTP ${res.status}` };
  let body: CollectorTrace | undefined;
  try {
    body = (await res.json()) as CollectorTrace;
  } catch {
    return { status: "unavailable", reason: "trace backend returned a non-JSON body" };
  }
  if (!body || !body.spans || body.spans.length === 0) return { status: "empty" };
  return { status: "ok", trace: toModel(body) };
}

export async function fetchTraceList(client: KitClient, backend: string, pageSize: number, offset: number): Promise<TraceListState> {
  let res: Response;
  try {
    res = await client.request(traceListPath(backend, pageSize, offset));
  } catch (err) {
    return { status: "unavailable", reason: reason(err) };
  }
  if (!res.ok) return { status: "unavailable", reason: `trace backend returned HTTP ${res.status}` };
  try {
    return { status: "ok", page: toListPage((await res.json()) as CollectorTraceList, offset, pageSize) };
  } catch {
    return { status: "unavailable", reason: "trace backend returned a non-JSON body" };
  }
}

// fetchTraces reads several traces at once; a missing or failing one is
// dropped rather than failing the set.
export async function fetchTraces(client: KitClient, backend: string, traceIds: string[]): Promise<Map<string, TraceModel>> {
  const out = new Map<string, TraceModel>();
  await Promise.all(
    traceIds.map(async (id) => {
      const state = await fetchTrace(client, backend, id);
      if (state.status === "ok") out.set(id, state.trace);
    }),
  );
  return out;
}

import {
  HTTPError,
  toListPage,
  toModel,
  type CollectorTrace,
  type CollectorTraceList,
  type KitClient,
  type TraceListPage,
  type TraceModel,
} from '@declarative-agents/ui-kit'

// Every read goes through the kit client (srd004 R6.1). The collector serves
// this UI same-origin with /query/* (srd020 R7), so the paths are the query
// surface itself rather than /monitor-proxy/{agent}/, and the kit's wire types
// and parsers (toListPage, toModel) decode the trace responses. The
// conformance guard TestCollectorQueryResponseContract
// (applications/catalog/conformance/collector_test.go) pins the emitted keys.

const BASE = '/query'

export async function listTraces(client: KitClient, pageSize: number, offset: number): Promise<TraceListPage> {
  const params = new URLSearchParams({ page_size: String(pageSize), offset: String(offset) })
  return toListPage(await client.getJSON<CollectorTraceList>(`${BASE}/traces?${params}`), offset, pageSize)
}

// getTrace answers undefined when the collector holds no spans for the id.
export async function getTrace(client: KitClient, traceId: string): Promise<TraceModel | undefined> {
  try {
    const body = await client.getJSON<CollectorTrace>(`${BASE}/traces/${encodeURIComponent(traceId)}`)
    return body.spans?.length ? toModel(body) : undefined
  } catch (err) {
    if (err instanceof HTTPError && err.status === 404) return undefined
    throw err
  }
}

// The Explore contracts mirror the /query/spans/* routes; the conformance
// guards TestCollectorSpanStatsContract and TestCollectorSpanBreakdownContract
// pin these keys, so a change on either side fails there first.
export interface HeatmapPayload {
  time_bucket_boundaries: number[]
  duration_bucket_boundaries: number[]
  cells: number[][]
}

export interface GroupCount {
  value: string
  count: number
}

export interface SpanStatsResponse {
  heatmap: HeatmapPayload
  matched: number
  exemplar_trace_ids: string[] | null
  skipped_lines: number
  group_by: string
  groups: GroupCount[] | null
  dropped_groups: number
  dropped_span_total: number
}

export interface DivergenceEntry {
  key: string
  value: string
  inside_count: number
  outside_count: number
  inside_proportion: number
  outside_proportion: number
  score: number
}

export interface SpanBreakdownResponse {
  inside_total: number
  outside_total: number
  exemplar_trace_ids: string[] | null
  ranked: DivergenceEntry[] | null
  dropped: number
  skipped_lines: number
}

export interface SpanStatsQuery {
  service?: string
  span_name?: string
  time_buckets?: number
  group_by?: string
  top_n?: number
}

export interface SpanBreakdownQuery {
  selection_start_ms?: number
  selection_end_ms?: number
  selection_min_duration_ms?: number
  selection_max_duration_ms?: number
  top_n?: number
}

function queryString(q: object): string {
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(q)) {
    if (v !== undefined && v !== '') params.set(k, String(v))
  }
  return params.toString()
}

export const getSpanStats = (client: KitClient, q: SpanStatsQuery) =>
  client.getJSON<SpanStatsResponse>(`${BASE}/spans/stats?${queryString(q)}`)

export const getSpanBreakdown = (client: KitClient, q: SpanBreakdownQuery) =>
  client.getJSON<SpanBreakdownResponse>(`${BASE}/spans/breakdown?${queryString(q)}`)

const BASE = '/query'

// These interfaces mirror the query surface exactly; the conformance guard
// TestCollectorQueryResponseContract (applications/catalog/conformance/
// collector_test.go) pins the emitted keys, so a change on either side fails
// there first.
export interface TraceSummary {
  trace_id: string
  root_span_name: string
  root_service: string
  span_count: number
  start_time: string
  duration_ms: number
}

export interface TraceListResponse {
  traces: TraceSummary[]
  total: number
  offset: number
  page_size: number
}

export interface SpanDetail {
  span_id: string
  parent_span_id: string
  service: string
  name: string
  start_time: string
  end_time: string
  status: { Code: number; Description: string }
  attributes: unknown[]
}

export interface TraceDetailResponse {
  trace_id: string
  spans: SpanDetail[]
  span_count: number
}

export async function listTraces(pageSize: number, offset: number): Promise<TraceListResponse> {
  const params = new URLSearchParams({ page_size: String(pageSize), offset: String(offset) })
  const res = await fetch(`${BASE}/traces?${params}`)
  if (!res.ok) throw new Error(`API error: ${res.status}`)
  return res.json()
}

export async function getTrace(traceId: string): Promise<TraceDetailResponse> {
  const res = await fetch(`${BASE}/traces/${encodeURIComponent(traceId)}`)
  if (!res.ok) throw new Error(`API error: ${res.status}`)
  return res.json()
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

export async function getSpanStats(q: SpanStatsQuery): Promise<SpanStatsResponse> {
  const res = await fetch(`${BASE}/spans/stats?${queryString(q)}`)
  if (!res.ok) throw new Error(`API error: ${res.status}`)
  return res.json()
}

export async function getSpanBreakdown(q: SpanBreakdownQuery): Promise<SpanBreakdownResponse> {
  const res = await fetch(`${BASE}/spans/breakdown?${queryString(q)}`)
  if (!res.ok) throw new Error(`API error: ${res.status}`)
  return res.json()
}

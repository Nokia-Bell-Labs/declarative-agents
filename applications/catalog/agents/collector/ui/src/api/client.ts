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

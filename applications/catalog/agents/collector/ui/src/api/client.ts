const BASE = '/query'

export interface TraceSummary {
  trace_id: string
  root_span: string
  service: string
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
  Name: string
  SpanContext: { TraceID: string; SpanID: string }
  Parent: { TraceID: string; SpanID: string }
  StartTime: string
  EndTime: string
  Status: { Code: number; Description: string }
  Attributes: { Key: string; Value: { Type: string; Value: string } }[]
  Resource: { Key: string; Value: { Type: string; Value: string } }[]
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

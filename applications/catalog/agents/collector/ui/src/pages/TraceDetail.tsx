import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router'
import { getTrace, type SpanDetail } from '../api/client'

interface SpanNode {
  span: SpanDetail
  depth: number
  startMs: number
  durationMs: number
}

export default function TraceDetail() {
  const { traceId } = useParams()
  const [spans, setSpans] = useState<SpanDetail[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!traceId) return
    setLoading(true)
    getTrace(traceId)
      .then(data => {
        setSpans(data.spans ?? [])
        setError('')
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [traceId])

  if (loading) return <div className="loading">Loading trace...</div>
  if (error) return <div className="error">{error}</div>
  if (spans.length === 0) return <div className="empty">No spans found.</div>

  const nodes = buildWaterfall(spans)
  const traceStartMs = Math.min(...nodes.map(n => n.startMs))
  const traceEndMs = Math.max(...nodes.map(n => n.startMs + n.durationMs))
  const traceDurationMs = Math.max(traceEndMs - traceStartMs, 1)

  return (
    <div>
      <Link to="/traces" className="back-link">&larr; All traces</Link>
      <h1>Trace {traceId}</h1>
      <div className="waterfall-header">
        <span className="waterfall-stat"><strong>{spans.length}</strong> spans</span>
        <span className="waterfall-stat"><strong>{formatDuration(traceDurationMs)}</strong> total</span>
        <span className="waterfall-stat">
          services: <strong>{uniqueServices(spans).join(', ') || 'unknown'}</strong>
        </span>
      </div>

      <div className="waterfall-scale">
        <div />
        <div className="waterfall-ticks">
          <span>0ms</span>
          <span>{formatDuration(traceDurationMs / 4)}</span>
          <span>{formatDuration(traceDurationMs / 2)}</span>
          <span>{formatDuration((traceDurationMs * 3) / 4)}</span>
          <span>{formatDuration(traceDurationMs)}</span>
        </div>
        <div />
      </div>

      <div className="waterfall">
        {nodes.map((n, i) => {
          const leftPct = ((n.startMs - traceStartMs) / traceDurationMs) * 100
          const widthPct = (n.durationMs / traceDurationMs) * 100
          const isError = n.span.Status.Code === 2
          return (
            <div className="waterfall-row" key={i}>
              <div className="waterfall-label">
                <span className="waterfall-indent" style={{ width: n.depth * 16 }} />
                <span className="waterfall-name" title={n.span.Name}>{n.span.Name}</span>
              </div>
              <div className="waterfall-bar-track">
                <div
                  className={`waterfall-bar ${isError ? 'bar-error' : 'bar-ok'}`}
                  style={{ left: `${leftPct}%`, width: `${Math.max(widthPct, 0.3)}%` }}
                />
              </div>
              <div className="waterfall-duration">{formatDuration(n.durationMs)}</div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function buildWaterfall(spans: SpanDetail[]): SpanNode[] {
  const byId = new Map<string, SpanDetail>()
  const children = new Map<string, SpanDetail[]>()
  for (const s of spans) {
    byId.set(s.SpanContext.SpanID, s)
    const parentId = s.Parent?.SpanID || ''
    if (!children.has(parentId)) children.set(parentId, [])
    children.get(parentId)!.push(s)
  }

  const roots = spans.filter(s => !s.Parent?.SpanID || !byId.has(s.Parent.SpanID))
  roots.sort((a, b) => new Date(a.StartTime).getTime() - new Date(b.StartTime).getTime())

  const nodes: SpanNode[] = []
  function walk(span: SpanDetail, depth: number) {
    const startMs = new Date(span.StartTime).getTime()
    const endMs = new Date(span.EndTime).getTime()
    nodes.push({ span, depth, startMs, durationMs: Math.max(endMs - startMs, 0) })
    const kids = children.get(span.SpanContext.SpanID) ?? []
    kids.sort((a, b) => new Date(a.StartTime).getTime() - new Date(b.StartTime).getTime())
    for (const child of kids) walk(child, depth + 1)
  }
  for (const root of roots) walk(root, 0)
  return nodes
}

function uniqueServices(spans: SpanDetail[]): string[] {
  const set = new Set<string>()
  for (const s of spans) {
    const svc = s.Resource?.find(r => r.Key === 'service.name')
    if (svc?.Value?.Value) set.add(svc.Value.Value)
  }
  return [...set].sort()
}

function formatDuration(ms: number): string {
  if (ms == null) return ''
  if (ms < 1) return '<1ms'
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

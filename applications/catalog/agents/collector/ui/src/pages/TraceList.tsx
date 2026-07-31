import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { listTraces, type TraceSummary } from '../api/client'

const PAGE_SIZE = 20

export default function TraceList() {
  const navigate = useNavigate()
  const [traces, setTraces] = useState<TraceSummary[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [filter, setFilter] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    setLoading(true)
    listTraces(PAGE_SIZE, offset)
      .then(data => {
        setTraces(data.traces ?? [])
        setTotal(data.total)
        setError('')
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [offset])

  const filtered = filter
    ? traces.filter(t =>
        t.root_service?.toLowerCase().includes(filter.toLowerCase()) ||
        t.trace_id?.toLowerCase().includes(filter.toLowerCase()) ||
        t.root_span_name?.toLowerCase().includes(filter.toLowerCase()))
    : traces

  if (loading) return <div className="loading">Loading traces...</div>
  if (error) return <div className="error">{error}</div>

  return (
    <div>
      <h1>Traces</h1>
      <div className="filter-row">
        <input
          className="filter-input"
          placeholder="Filter by service, trace ID, or root span..."
          value={filter}
          onChange={e => setFilter(e.target.value)}
        />
        <span className="mono" style={{ color: 'var(--text-secondary)' }}>
          {total} total
        </span>
      </div>
      {filtered.length === 0 ? (
        <div className="empty">No traces found.</div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Trace ID</th>
                <th>Root Span</th>
                <th>Service</th>
                <th>Spans</th>
                <th>Start</th>
                <th>Duration</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map(t => (
                <tr key={t.trace_id} onClick={() => navigate(`/traces/${t.trace_id}`)}>
                  <td className="mono">{t.trace_id}</td>
                  <td>{t.root_span_name}</td>
                  <td>{t.root_service}</td>
                  <td>{t.span_count}</td>
                  <td className="mono">{formatTime(t.start_time)}</td>
                  <td className="mono">{formatDuration(t.duration_ms)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <div className="pagination">
        <button disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}>
          Prev
        </button>
        <span>
          {offset + 1}&ndash;{Math.min(offset + PAGE_SIZE, total)} of {total}
        </span>
        <button disabled={offset + PAGE_SIZE >= total} onClick={() => setOffset(offset + PAGE_SIZE)}>
          Next
        </button>
      </div>
    </div>
  )
}

function formatTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  const hms = d.toLocaleTimeString(undefined, { hour12: false })
  const ms = String(d.getMilliseconds()).padStart(3, '0')
  return `${hms}.${ms}`
}

function formatDuration(ms: number): string {
  if (ms == null) return ''
  if (ms < 1) return '<1ms'
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

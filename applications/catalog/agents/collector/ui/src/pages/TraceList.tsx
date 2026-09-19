import { useEffect, useState } from 'react'
import { relativeTime, useKitClient, type TraceListPage } from '@declarative-agents/ui-kit'
import { listTraces } from '../api/client'
import { navigateTo as navigate } from '@declarative-agents/ui-kit'

const PAGE_SIZE = 20

// The collector's own trace list (srd020 R6.1), read same-origin from
// /query/traces through the kit client.
export default function TraceList() {
  const client = useKitClient()
  const [page, setPage] = useState<TraceListPage | null>(null)
  const [offset, setOffset] = useState(0)
  const [filter, setFilter] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    setPage(null)
    listTraces(client, PAGE_SIZE, offset)
      .then(next => {
        setPage(next)
        setError('')
      })
      .catch(e => setError(e.message))
  }, [client, offset])

  if (error) return <div className="error">{error}</div>
  if (!page) return <div className="loading">Loading traces...</div>

  const needle = filter.toLowerCase()
  const rows = page.traces.filter(t =>
    [t.rootService, t.traceId, t.rootSpanName].some(field => field.toLowerCase().includes(needle)))

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
        <span className="mono muted">{page.total} total</span>
      </div>
      {rows.length === 0 ? (
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
              {rows.map(t => (
                <tr key={t.traceId} onClick={() => navigate(`/traces/${t.traceId}`)}>
                  <td className="mono">{t.traceId}</td>
                  <td>{t.rootSpanName}</td>
                  <td>{t.rootService}</td>
                  <td>{t.spanCount}</td>
                  <td className="mono">{relativeTime(t.startTime)}</td>
                  <td className="mono">{formatDuration(t.durationMs)}</td>
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
          {page.total === 0 ? 0 : offset + 1}&ndash;{Math.min(offset + PAGE_SIZE, page.total)} of {page.total}
        </span>
        <button disabled={offset + PAGE_SIZE >= page.total} onClick={() => setOffset(offset + PAGE_SIZE)}>
          Next
        </button>
      </div>
    </div>
  )
}

export function formatDuration(ms: number): string {
  if (ms < 1) return '<1ms'
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

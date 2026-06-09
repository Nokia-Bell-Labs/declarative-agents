import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { getTrace, TraceData } from '../api/client'

export default function TraceViewer() {
  const { suite, ts, pointId } = useParams()

  const [trace, setTrace] = useState<TraceData | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!suite || !ts || !pointId) return
    getTrace(suite, ts, pointId)
      .then(setTrace)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [suite, ts, pointId])

  if (loading) return <div className="loading">Loading trace...</div>
  if (error) return <div className="error">{error}</div>
  if (!trace) return <div className="error">Trace not found</div>

  const maxDur = Math.max(...trace.spans.map(s => s.durationMs), 1)

  return (
    <div>
      <Link to={`/sessions/${suite}/${ts}`} className="back-link">-- Session</Link>
      <h1>{pointId}</h1>

      <h2>Tool Call Timeline</h2>
      <div className="timeline">
        {trace.spans.map((span, i) => {
          const barWidth = Math.max((span.durationMs / maxDur) * 100, 2)
          const isLLM = span.name.startsWith('chat ')
          const isTool = !isLLM && span.toolName !== ''

          return (
            <div key={i} className="timeline-row">
              <div className="timeline-name">
                <span className={`timeline-badge ${isLLM ? 'badge-llm' : isTool ? 'badge-tool' : 'badge-other'}`}>
                  {isLLM ? 'LLM' : isTool ? 'TOOL' : 'SYS'}
                </span>
                <span className="mono">{span.toolName || span.name}</span>
              </div>
              <div className="timeline-bar-container">
                <div
                  className={`timeline-bar ${isLLM ? 'bar-llm' : isTool ? 'bar-tool' : 'bar-other'}`}
                  style={{ width: `${barWidth}%` }}
                />
              </div>
              <div className="timeline-meta">
                <span className="timeline-dur">{span.durationMs >= 1000 ? `${(span.durationMs/1000).toFixed(1)}s` : `${span.durationMs}ms`}</span>
                {span.tokensIn > 0 && (
                  <span className="timeline-tokens">{span.tokensIn} in / {span.tokensOut} out</span>
                )}
                {span.signal && <span className="timeline-signal">{span.signal}</span>}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

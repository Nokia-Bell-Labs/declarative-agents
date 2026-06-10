import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { listSessions, Session } from '../api/client'

export default function Dashboard() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    listSessions()
      .then(setSessions)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="loading">Loading sessions...</div>
  if (error) return <div className="error">{error}</div>
  if (sessions.length === 0) return <div className="empty">No sessions found. Start a benchmark to see results here.</div>

  return (
    <div>
      <h1>Evaluation Sessions</h1>
      <div className="session-grid">
        {sessions.map(s => {
          const passRate = s.pointCount > 0 ? (s.passCount / s.pointCount * 100) : 0
          return (
            <Link key={s.id} to={`/sessions/${s.id}`} className="session-card">
              <div className="session-card-header">
                <span className="session-name">{s.name}</span>
                <span className="session-ts">{s.timestamp}</span>
              </div>
              <div className="session-stats">
                <div className="stat">
                  <span className="stat-value">{s.pointCount}</span>
                  <span className="stat-label">points</span>
                </div>
                <div className="stat">
                  <span className="stat-value pass">{s.passCount}</span>
                  <span className="stat-label">passed</span>
                </div>
                <div className="stat">
                  <span className="stat-value fail">{s.failCount}</span>
                  <span className="stat-label">failed</span>
                </div>
                {s.timeoutCount > 0 && (
                  <div className="stat">
                    <span className="stat-value timeout">{s.timeoutCount}</span>
                    <span className="stat-label">timeout</span>
                  </div>
                )}
              </div>
              <div className="pass-bar">
                <div className="pass-bar-fill" style={{ width: `${passRate}%` }} />
              </div>
              <div className="pass-rate">{passRate.toFixed(0)}% pass rate</div>
            </Link>
          )
        })}
      </div>
    </div>
  )
}

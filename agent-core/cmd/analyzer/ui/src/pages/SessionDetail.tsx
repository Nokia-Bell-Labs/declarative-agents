import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts'
import { getSession, listPoints, SessionDetail as SessionDetailType, Point } from '../api/client'

export default function SessionDetail() {
  const { suite, ts } = useParams()

  const [detail, setDetail] = useState<SessionDetailType | null>(null)
  const [points, setPoints] = useState<Point[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!suite || !ts) return
    Promise.all([getSession(suite, ts), listPoints(suite, ts)])
      .then(([d, p]) => { setDetail(d); setPoints(p) })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [suite, ts])

  if (loading) return <div className="loading">Loading session...</div>
  if (error) return <div className="error">{error}</div>
  if (!detail) return <div className="error">Session not found</div>

  const models = [...new Set(points.map(p => p.model))].sort()
  const samples = [...new Set(points.map(p => p.sample))].sort()

  const chartData = detail.modelStats
    .sort((a, b) => b.successRate - a.successRate)
    .map(m => ({ name: m.model.split(':')[0], passed: m.successes, total: m.runs }))

  return (
    <div>
      <Link to="/" className="back-link">-- Sessions</Link>
      <h1>{suite} / {ts}</h1>

      <div className="summary-row">
        <div className="summary-stat">
          <span className="summary-value">{detail.totalPoints}</span>
          <span className="summary-label">total</span>
        </div>
        <div className="summary-stat">
          <span className="summary-value pass">{detail.totalPassed}</span>
          <span className="summary-label">passed</span>
        </div>
        <div className="summary-stat">
          <span className="summary-value fail">{detail.totalFailed}</span>
          <span className="summary-label">failed</span>
        </div>
        <div className="summary-stat">
          <span className="summary-value timeout">{detail.totalTimedOut}</span>
          <span className="summary-label">timeout</span>
        </div>
      </div>

      <h2>Model Leaderboard</h2>
      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>Model</th>
              <th>Score</th>
              <th>Success</th>
              <th>Clean</th>
              <th>Stuck</th>
              <th>Mean Iter</th>
              <th>Mean Tokens</th>
              <th>Mean Duration</th>
            </tr>
          </thead>
          <tbody>
            {detail.modelStats
              .sort((a, b) => b.successRate - a.successRate)
              .map(m => (
                <tr key={m.model}>
                  <td className="mono">{m.model}</td>
                  <td>{m.successes}/{m.runs}</td>
                  <td className={m.successRate === 1 ? 'pass' : m.successRate < 0.7 ? 'fail' : ''}>
                    {(m.successRate * 100).toFixed(0)}%
                  </td>
                  <td>{(m.cleanRate * 100).toFixed(0)}%</td>
                  <td>{(m.stuckRate * 100).toFixed(0)}%</td>
                  <td>{m.meanIter.toFixed(1)}</td>
                  <td>{(m.meanTokensIn + m.meanTokensOut).toFixed(0)}</td>
                  <td>{m.meanDurationS.toFixed(0)}s</td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>

      <h2>Pass Count by Model</h2>
      <div style={{ width: '100%', height: 250 }}>
        <ResponsiveContainer>
          <BarChart data={chartData} margin={{ top: 8, right: 8, bottom: 8, left: 8 }}>
            <XAxis dataKey="name" tick={{ fill: '#666666', fontSize: 12 }} />
            <YAxis tick={{ fill: '#666666', fontSize: 12 }} />
            <Tooltip contentStyle={{ background: '#ebebeb', border: '1px solid #dcdcdc', color: '#001135' }} />
            <Bar dataKey="passed" radius={[4, 4, 0, 0]}>
              {chartData.map((entry, i) => (
                <Cell key={i} fill={entry.passed === entry.total ? '#37cc73' : entry.passed >= entry.total * 0.7 ? '#005aff' : '#e23b3b'} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>

      <h2>Sample x Model Heatmap</h2>
      <div className="heatmap-container">
        <table className="heatmap">
          <thead>
            <tr>
              <th></th>
              {models.map(m => <th key={m} className="heatmap-header">{m.split(':')[0]}</th>)}
            </tr>
          </thead>
          <tbody>
            {samples.map(sample => (
              <tr key={sample}>
                <td className="heatmap-label">{sample}</td>
                {models.map(model => {
                  const pt = points.find(p => p.sample === sample && p.model === model)
                  const cls = pt ? (pt.testsPassed ? 'cell-pass' : pt.timedOut ? 'cell-timeout' : 'cell-fail') : 'cell-empty'
                  return (
                    <td key={model} className={`heatmap-cell ${cls}`}>
                      {pt ? (
                        <Link to={`/sessions/${suite}/${ts}/points/${pt.pointId}`}>
                          {pt.testsPassed ? 'P' : pt.timedOut ? 'T' : 'F'}
                        </Link>
                      ) : '-'}
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

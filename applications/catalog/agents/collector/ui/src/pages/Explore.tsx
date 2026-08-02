import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import {
  getSpanStats,
  getSpanBreakdown,
  listTraces,
  type SpanStatsResponse,
  type SpanBreakdownResponse,
  type TraceSummary,
} from '../api/client'

interface Cell {
  col: number
  row: number
}

// selectionFromCells maps a drag-selected cell rectangle onto the span filter
// the breakdown route understands: the time-bucket boundaries give the time
// span, the duration-bucket boundaries give the latency band (the last duration
// bucket is the open-ended overflow, so it contributes no max).
function selectionFromCells(stats: SpanStatsResponse, a: Cell, b: Cell) {
  const time = stats.heatmap.time_bucket_boundaries
  const dur = stats.heatmap.duration_bucket_boundaries
  const c0 = Math.min(a.col, b.col)
  const c1 = Math.max(a.col, b.col)
  const r0 = Math.min(a.row, b.row)
  const r1 = Math.max(a.row, b.row)
  const sel: Record<string, number> = {
    selection_start_ms: time[c0],
    selection_end_ms: time[c1 + 1],
    selection_min_duration_ms: dur[r0],
  }
  if (r1 < dur.length - 1) sel.selection_max_duration_ms = dur[r1 + 1]
  return sel
}

export default function Explore() {
  const [stats, setStats] = useState<SpanStatsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [dragStart, setDragStart] = useState<Cell | null>(null)
  const [dragEnd, setDragEnd] = useState<Cell | null>(null)
  const [breakdown, setBreakdown] = useState<SpanBreakdownResponse | null>(null)
  const [breakdownError, setBreakdownError] = useState('')
  const [exemplars, setExemplars] = useState<TraceSummary[]>([])

  useEffect(() => {
    setLoading(true)
    getSpanStats({ group_by: 'service.name', time_buckets: 24 })
      .then(data => {
        setStats(data)
        setError('')
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
    listTraces(20, 0)
      .then(d => setExemplars(d.traces ?? []))
      .catch(() => setExemplars([]))
  }, [])

  function applySelection(a: Cell, b: Cell) {
    if (!stats) return
    setBreakdownError('')
    getSpanBreakdown(selectionFromCells(stats, a, b))
      .then(setBreakdown)
      .catch(e => setBreakdownError(e.message))
  }

  if (loading) return <div className="loading">Loading span analytics...</div>
  if (error) return <div className="error">{error}</div>
  if (!stats || stats.matched === 0) {
    return (
      <div>
        <h1>Explore</h1>
        <div className="empty">No spans in the spool yet.</div>
      </div>
    )
  }

  return (
    <div>
      <h1>Explore</h1>
      <p style={{ color: 'var(--text-secondary)' }}>
        {stats.matched} spans. Drag a region of the heatmap to explain what makes it distinct.
      </p>
      <Heatmap
        stats={stats}
        dragStart={dragStart}
        dragEnd={dragEnd}
        onDown={c => {
          setDragStart(c)
          setDragEnd(c)
        }}
        onMove={c => dragStart && setDragEnd(c)}
        onUp={() => {
          if (dragStart && dragEnd) applySelection(dragStart, dragEnd)
        }}
      />
      <div className="explore-panels">
        <BreakdownPanel breakdown={breakdown} error={breakdownError} />
        <GroupPanel stats={stats} />
      </div>
      <h2>Traces</h2>
      {exemplars.length === 0 ? (
        <div className="empty">No traces to show.</div>
      ) : (
        <ul className="exemplar-list">
          {exemplars.map(t => (
            <li key={t.trace_id}>
              <Link className="mono" to={`/traces/${t.trace_id}`}>
                {t.trace_id}
              </Link>{' '}
              <span style={{ color: 'var(--text-secondary)' }}>
                {t.root_service} · {t.root_span_name}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

interface HeatmapProps {
  stats: SpanStatsResponse
  dragStart: Cell | null
  dragEnd: Cell | null
  onDown: (c: Cell) => void
  onMove: (c: Cell) => void
  onUp: () => void
}

function Heatmap({ stats, dragStart, dragEnd, onDown, onMove, onUp }: HeatmapProps) {
  const cells = stats.heatmap.cells
  const cols = cells.length
  const rows = cols > 0 ? cells[0].length : 0
  const dur = stats.heatmap.duration_bucket_boundaries
  const cellW = 22
  const cellH = 18
  const padL = 70
  const padB = 16
  let max = 0
  for (const c of cells) for (const v of c) if (v > max) max = v

  const inSelection = (col: number, row: number) => {
    if (!dragStart || !dragEnd) return false
    return (
      col >= Math.min(dragStart.col, dragEnd.col) &&
      col <= Math.max(dragStart.col, dragEnd.col) &&
      row >= Math.min(dragStart.row, dragEnd.row) &&
      row <= Math.max(dragStart.row, dragEnd.row)
    )
  }

  return (
    <svg
      className="heatmap"
      width={padL + cols * cellW + 10}
      height={rows * cellH + padB + 10}
      onMouseLeave={onUp}
    >
      {Array.from({ length: rows }).map((_, row) => (
        <text key={`yl-${row}`} x={padL - 6} y={row * cellH + cellH - 4} className="heatmap-label" textAnchor="end">
          {durationLabel(dur, row)}
        </text>
      ))}
      {cells.map((colCells, col) =>
        colCells.map((count, row) => (
          <rect
            key={`${col}-${row}`}
            x={padL + col * cellW}
            y={row * cellH}
            width={cellW - 1}
            height={cellH - 1}
            className={inSelection(col, row) ? 'heatmap-cell heatmap-cell-selected' : 'heatmap-cell'}
            fill={cellColor(count, max)}
            onMouseDown={() => onDown({ col, row })}
            onMouseEnter={() => onMove({ col, row })}
            onMouseUp={onUp}
          >
            <title>{`${count} span(s)`}</title>
          </rect>
        )),
      )}
    </svg>
  )
}

function BreakdownPanel({ breakdown, error }: { breakdown: SpanBreakdownResponse | null; error: string }) {
  if (error) return <div className="error">{error}</div>
  if (!breakdown) return <div className="panel"><h2>Breakdown</h2><div className="empty">Drag the heatmap to select a region.</div></div>
  const ranked = breakdown.ranked ?? []
  return (
    <div className="panel">
      <h2>Breakdown</h2>
      <div style={{ color: 'var(--text-secondary)' }}>
        {breakdown.inside_total} in selection · {breakdown.outside_total} outside
      </div>
      {ranked.length === 0 ? (
        <div className="empty">No distinguishing attributes.</div>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Attribute</th>
              <th>In</th>
              <th>Out</th>
              <th>Score</th>
            </tr>
          </thead>
          <tbody>
            {ranked.map(e => (
              <tr key={`${e.key}=${e.value}`}>
                <td className="mono">{e.key}={e.value}</td>
                <td>{e.inside_count}</td>
                <td>{e.outside_count}</td>
                <td className="mono">{e.score.toFixed(3)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

function GroupPanel({ stats }: { stats: SpanStatsResponse }) {
  const groups = stats.groups ?? []
  return (
    <div className="panel">
      <h2>By {stats.group_by || 'group'}</h2>
      {groups.length === 0 ? (
        <div className="empty">No groups.</div>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Value</th>
              <th>Spans</th>
            </tr>
          </thead>
          <tbody>
            {groups.map(g => (
              <tr key={g.value}>
                <td className="mono">{g.value}</td>
                <td>{g.count}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {stats.dropped_groups > 0 && (
        <div style={{ color: 'var(--text-secondary)' }}>
          +{stats.dropped_groups} more ({stats.dropped_span_total} spans)
        </div>
      )}
    </div>
  )
}

function durationLabel(dur: number[], row: number): string {
  if (row >= dur.length - 1) return `≥${dur[dur.length - 1]}ms`
  return `${dur[row]}–${dur[row + 1]}ms`
}

// cellColor scales from the surface color to the accent as the count rises, so
// denser cells read darker. Zero-count cells stay at the surface color.
function cellColor(count: number, max: number): string {
  if (count === 0 || max === 0) return 'var(--bg-tertiary)'
  const t = Math.min(1, 0.15 + (0.85 * count) / max)
  return `color-mix(in srgb, var(--accent) ${Math.round(t * 100)}%, var(--bg-tertiary))`
}

import { usePanelPath as usePath } from '@declarative-agents/ui-kit'
import Dashboard from './Dashboard'
import SessionDetail from './SessionDetail'
import TraceViewer from './TraceViewer'

// The experiments panel owns the session list and its drill-downs below
// /sessions/; the shell also mounts it at / as the default panel.
export default function Experiments() {
  const [head, suite, ts, points, pointId, ...rest] = usePath().split('/').slice(1).map(decodeURIComponent)
  if (head !== 'sessions' || !suite || !ts || rest.length > 0) return <Dashboard />
  if (points === undefined) return <SessionDetail suite={suite} ts={ts} />
  if (points === 'points' && pointId) return <TraceViewer suite={suite} ts={ts} pointId={pointId} />
  return <Dashboard />
}

import { usePanelPath as usePath } from '@declarative-agents/ui-kit'
import TraceDetail from './TraceDetail'
import TraceList from './TraceList'

// The traces panel owns /traces and its deep link /traces/{trace_id}; the
// shell also mounts it at / as the default panel.
export default function Traces() {
  const match = /^\/traces\/([^/]+)$/.exec(usePath())
  return match ? <TraceDetail traceId={decodeURIComponent(match[1])} /> : <TraceList />
}

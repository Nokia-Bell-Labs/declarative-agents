import { useEffect, useState } from 'react'
import { MachineWalks, SpanTreeToggle, Timeline, useKitClient, type TraceModel } from '@declarative-agents/ui-kit'
import { getTrace } from '../api/client'
import { PanelLink as Link } from '@declarative-agents/ui-kit'
import { formatDuration } from './TraceList'

type Loaded = { status: 'loading' } | { status: 'error'; message: string } | { status: 'ok'; trace: TraceModel | undefined }

// One trace read same-origin from /query/traces/{trace_id} and drawn with the
// kit trace views: the agent timeline, the span tree, and the request walks.
export default function TraceDetail({ traceId }: { traceId: string }) {
  const client = useKitClient()
  const [state, setState] = useState<Loaded>({ status: 'loading' })

  useEffect(() => {
    setState({ status: 'loading' })
    getTrace(client, traceId)
      .then(trace => setState({ status: 'ok', trace }))
      .catch(e => setState({ status: 'error', message: e.message }))
  }, [client, traceId])

  return (
    <div>
      <Link to="/traces" className="back-link">&larr; All traces</Link>
      <h1>Trace {traceId}</h1>
      <TraceBody state={state} />
    </div>
  )
}

function TraceBody({ state }: { state: Loaded }) {
  if (state.status === 'loading') return <div className="loading">Loading trace...</div>
  if (state.status === 'error') return <div className="error">{state.message}</div>
  const { trace } = state
  if (!trace) return <div className="empty">No spans found.</div>
  return (
    <>
      <div className="trace-stats">
        <span><strong>{trace.spans.length}</strong> spans</span>
        <span><strong>{formatDuration((trace.endUs - trace.startUs) / 1000)}</strong> total</span>
        <span>services: <strong>{trace.services.join(', ') || 'unknown'}</strong></span>
      </div>
      <div className="dak-trace">
        <Timeline trace={trace} />
        <SpanTreeToggle trace={trace} />
        <MachineWalks walks={trace.walks} />
      </div>
    </>
  )
}

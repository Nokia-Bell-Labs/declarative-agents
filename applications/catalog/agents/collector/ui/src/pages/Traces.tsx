import { navigateTo, TraceView, usePanelPath, type PanelProps } from '@declarative-agents/ui-kit'

// The traces panel is the kit TraceView driven by the URL: /traces lists the
// collector's traces and /traces/{trace_id} opens one, so a trace is a deep
// link (srd020 R7.2). The shell mounts it at / too, as the default panel.
// ui.yaml's trace_backend.query_path makes the backend the origin root ("/"):
// the collector serves this UI beside its own /query/* (srd004 R2.4).
export default function Traces({ traceBackend }: PanelProps) {
  const match = /^\/traces\/([^/]+)$/.exec(usePanelPath())
  return (
    <TraceView
      backend={traceBackend ?? '/'}
      openTraceId={match ? decodeURIComponent(match[1]) : undefined}
      onOpen={traceId => navigateTo(traceId ? `/traces/${encodeURIComponent(traceId)}` : '/traces')}
    />
  )
}

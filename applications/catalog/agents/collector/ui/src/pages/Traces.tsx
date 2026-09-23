import { useEffect, useState } from 'react'
import {
  navigateTo,
  TraceView,
  useKitClient,
  usePanelPath,
  type PanelProps,
} from '@declarative-agents/ui-kit'
import { getTraceStorageStatus, type StorageStatus } from '../api/client'

// The traces panel is the kit TraceView driven by the URL: /traces lists the
// collector's traces and /traces/{trace_id} opens one, so a trace is a deep
// link (srd020 R7.2). The shell mounts it at / too, as the default panel.
// ui.yaml's trace_backend.query_path makes the backend the origin root ("/"):
// the collector serves this UI beside its own /query/* (srd004 R2.4).
export default function Traces({ traceBackend }: PanelProps) {
  const match = /^\/traces\/([^/]+)$/.exec(usePanelPath())
  const client = useKitClient()
  const [storage, setStorage] = useState<StorageStatus | 'unknown'>('unknown')

  useEffect(() => {
    getTraceStorageStatus(client)
      .then(response => setStorage(response.storage_status))
      .catch(() => setStorage('unknown'))
  }, [client])

  return (
    <div className="traces-page">
      <StorageStatusBanner status={storage} />
      <TraceView
        backend={traceBackend ?? '/'}
        openTraceId={match ? decodeURIComponent(match[1]) : undefined}
        onOpen={traceId => navigateTo(traceId ? `/traces/${encodeURIComponent(traceId)}` : '/traces')}
      />
    </div>
  )
}

function StorageStatusBanner({ status }: { status: StorageStatus | 'unknown' }) {
  const descriptions: Record<typeof status, string> = {
    complete: 'Retained object history and pending WAL are fully readable.',
    partial: 'History is partial: some retained objects or pending WAL evidence could not be read.',
    unavailable: 'Retained storage is unavailable. Results must not be treated as complete history.',
    unknown: 'Storage completeness could not be determined.',
  }
  return (
    <div className={`storage-status storage-status-${status}`} role="status">
      <strong>Storage: {status}</strong>
      <span>{descriptions[status]}</span>
    </div>
  )
}

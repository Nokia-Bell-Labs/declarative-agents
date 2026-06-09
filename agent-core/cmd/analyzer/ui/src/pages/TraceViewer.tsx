import { useParams } from 'react-router-dom'

export default function TraceViewer() {
  const { id, pointId } = useParams()
  return (
    <div>
      <h1>Trace: {pointId}</h1>
      <p style={{ color: 'var(--text-secondary)' }}>Session: {id}</p>
    </div>
  )
}

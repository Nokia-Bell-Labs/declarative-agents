import { useParams } from 'react-router-dom'

export default function SessionDetail() {
  const { id } = useParams()
  return (
    <div>
      <h1>Session: {id}</h1>
      <p style={{ color: 'var(--text-secondary)' }}>Loading session data...</p>
    </div>
  )
}

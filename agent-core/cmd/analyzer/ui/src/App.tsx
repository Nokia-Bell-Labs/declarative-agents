import { Routes, Route, Link } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import SessionDetail from './pages/SessionDetail'
import TraceViewer from './pages/TraceViewer'

export default function App() {
  return (
    <div className="app">
      <header className="app-header">
        <Link to="/" className="app-title">Analyzer</Link>
      </header>
      <main className="app-main">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/sessions/:suite/:ts" element={<SessionDetail />} />
          <Route path="/sessions/:suite/:ts/points/:pointId" element={<TraceViewer />} />
        </Routes>
      </main>
    </div>
  )
}

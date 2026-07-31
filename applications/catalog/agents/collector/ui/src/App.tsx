import { Routes, Route, NavLink } from 'react-router'
import TraceList from './pages/TraceList'
import TraceDetail from './pages/TraceDetail'

export default function App() {
  return (
    <div className="app">
      <header className="app-header">
        <nav className="app-nav">
          <NavLink to="/traces" className={({ isActive }) => `nav-tab ${isActive ? 'nav-tab-active' : ''}`}>
            Traces
          </NavLink>
        </nav>
      </header>
      <main className="app-main">
        <Routes>
          <Route path="/" element={<TraceList />} />
          <Route path="/traces" element={<TraceList />} />
          <Route path="/traces/:traceId" element={<TraceDetail />} />
        </Routes>
      </main>
    </div>
  )
}

import { Routes, Route, NavLink } from 'react-router'
import TraceList from './pages/TraceList'
import TraceDetail from './pages/TraceDetail'
import Explore from './pages/Explore'

export default function App() {
  return (
    <div className="app">
      <header className="app-header">
        <nav className="app-nav">
          <NavLink to="/traces" className={({ isActive }) => `nav-tab ${isActive ? 'nav-tab-active' : ''}`}>
            Traces
          </NavLink>
          <NavLink to="/explore" className={({ isActive }) => `nav-tab ${isActive ? 'nav-tab-active' : ''}`}>
            Explore
          </NavLink>
        </nav>
      </header>
      <main className="app-main">
        <Routes>
          <Route path="/" element={<TraceList />} />
          <Route path="/traces" element={<TraceList />} />
          <Route path="/traces/:traceId" element={<TraceDetail />} />
          <Route path="/explore" element={<Explore />} />
        </Routes>
      </main>
    </div>
  )
}

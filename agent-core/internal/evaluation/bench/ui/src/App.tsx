import { Routes, Route, NavLink } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import SessionDetail from './pages/SessionDetail'
import TraceViewer from './pages/TraceViewer'
import Documentation from './pages/Documentation'
import Launcher from './pages/Launcher'

export default function App() {
  return (
    <div className="app">
      <header className="app-header">
        <nav className="app-nav">
          <NavLink to="/" className={({ isActive }) => `nav-tab ${isActive ? 'nav-tab-active' : ''}`} end>
            Experiments
          </NavLink>
          <NavLink to="/launch" className={({ isActive }) => `nav-tab ${isActive ? 'nav-tab-active' : ''}`}>
            Launch
          </NavLink>
          <NavLink to="/docs" className={({ isActive }) => `nav-tab ${isActive ? 'nav-tab-active' : ''}`}>
            Documentation
          </NavLink>
        </nav>
      </header>
      <main className="app-main">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/launch" element={<Launcher />} />
          <Route path="/sessions/:suite/:ts" element={<SessionDetail />} />
          <Route path="/sessions/:suite/:ts/points/:pointId" element={<TraceViewer />} />
          <Route path="/docs" element={<Documentation />} />
          <Route path="/docs/*" element={<Documentation />} />
        </Routes>
      </main>
    </div>
  )
}

import { Navigate, Route, Routes } from 'react-router-dom'
import Documentation from './Documentation'

export default function App() {
  return (
    <div className="app app-documentation">
      <main className="app-main">
        <Routes>
          <Route path="/" element={<Navigate to="/docs" replace />} />
          <Route path="/docs" element={<Documentation />} />
          <Route path="/docs/*" element={<Documentation />} />
        </Routes>
      </main>
    </div>
  )
}

import { Routes, Route, Navigate } from 'react-router-dom'

const DashboardPage = () => (
  <div style={{ padding: '2rem', fontFamily: 'sans-serif' }}>
    <h1>Mosaic Admin</h1>
    <p>系统管理后台</p>
  </div>
)

export default function App() {
  return (
    <Routes>
      <Route path="/dashboard" element={<DashboardPage />} />
      <Route path="/" element={<Navigate to="/dashboard" replace />} />
    </Routes>
  )
}

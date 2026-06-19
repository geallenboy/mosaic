import { Routes, Route, Navigate } from 'react-router-dom'

// 页面路由（后续逐步实现）
const WorkspacePage = () => (
  <div style={{ padding: '2rem', fontFamily: 'sans-serif' }}>
    <h1>Mosaic 万象集</h1>
    <p>你的云端 AI 团队，把一个想法生成一整套项目资产。</p>
  </div>
)

export default function App() {
  return (
    <Routes>
      <Route path="/workspace" element={<WorkspacePage />} />
      <Route path="/" element={<Navigate to="/workspace" replace />} />
    </Routes>
  )
}

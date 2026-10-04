import { Navigate, Route, Routes } from 'react-router-dom'
import { useAuth } from './lib/auth-context'
import Login from './pages/Login'
import Sites from './pages/Sites'
import Dashboard from './pages/Dashboard'
import type { ReactNode } from 'react'

function RequireAuth({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth()
  if (loading) return <div className="p-10 text-sm text-zinc-500">Loading...</div>
  if (!user) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/sites" element={<RequireAuth><Sites /></RequireAuth>} />
      <Route path="/sites/:id" element={<RequireAuth><Dashboard /></RequireAuth>} />
      <Route path="*" element={<Navigate to="/sites" replace />} />
    </Routes>
  )
}
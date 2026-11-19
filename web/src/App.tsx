import { Navigate, Route, Routes } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useAuth } from './auth'
import Layout from './components/Layout'
import Login from './pages/Login'
import Register from './pages/Register'
import Dashboard from './pages/Dashboard'
import Keys from './pages/Keys'
import Usage from './pages/Usage'
import Billing from './pages/Billing'
import Security from './pages/Security'
import AdminUsers from './pages/admin/Users'
import AdminChannels from './pages/admin/Channels'
import AdminPrices from './pages/admin/Prices'
import AdminUsage from './pages/admin/AllUsage'

function RequireAuth({ admin, children }: { admin?: boolean; children: ReactNode }) {
  const { user, loading } = useAuth()
  if (loading) return <div className="center muted">Loading…</div>
  if (!user) return <Navigate to="/login" replace />
  if (admin && user.role !== 'admin') return <Navigate to="/" replace />
  return children
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/register" element={<Register />} />
      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<Dashboard />} />
        <Route path="keys" element={<Keys />} />
        <Route path="usage" element={<Usage />} />
        <Route path="billing" element={<Billing />} />
        <Route path="security" element={<Security />} />
        <Route path="admin/users" element={<RequireAuth admin><AdminUsers /></RequireAuth>} />
        <Route path="admin/channels" element={<RequireAuth admin><AdminChannels /></RequireAuth>} />
        <Route path="admin/prices" element={<RequireAuth admin><AdminPrices /></RequireAuth>} />
        <Route path="admin/usage" element={<RequireAuth admin><AdminUsage /></RequireAuth>} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

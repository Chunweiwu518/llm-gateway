import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useAuth } from './auth'
import Layout from './components/Layout'
import Login from './pages/Login'
import Register from './pages/Register'
import { ErrorBox } from './components/ui'

// Console and admin pages load on demand, so the login screen ships only its own code.
const Dashboard = lazy(() => import('./pages/Dashboard'))
const Keys = lazy(() => import('./pages/Keys'))
const Usage = lazy(() => import('./pages/Usage'))
const Billing = lazy(() => import('./pages/Billing'))
const Security = lazy(() => import('./pages/Security'))
const AdminUsers = lazy(() => import('./pages/admin/Users'))
const AdminChannels = lazy(() => import('./pages/admin/Channels'))
const AdminPrices = lazy(() => import('./pages/admin/Prices'))
const AdminUsage = lazy(() => import('./pages/admin/AllUsage'))

function RequireAuth({ admin, children }: { admin?: boolean; children: ReactNode }) {
  const { user, loading, error, refresh } = useAuth()
  if (loading) return <div className="center muted">Loading…</div>
  if (error)
    return (
      <div className="center">
        <ErrorBox error={error} />
        <button onClick={() => void refresh()}>Retry</button>
      </div>
    )
  if (!user) return <Navigate to="/login" replace />
  if (admin && user.role !== 'admin') return <Navigate to="/" replace />
  return children
}

export default function App() {
  return (
    <Suspense fallback={<div className="center muted">Loading…</div>}>
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
    </Suspense>
  )
}

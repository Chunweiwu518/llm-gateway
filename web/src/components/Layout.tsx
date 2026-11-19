import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth'
import { usd } from '../api'

const devLinks = [
  ['/', 'Overview'],
  ['/keys', 'API Keys'],
  ['/usage', 'Usage'],
  ['/billing', 'Billing'],
  ['/security', 'Security'],
]

const adminLinks = [
  ['/admin/users', 'Users'],
  ['/admin/channels', 'Channels'],
  ['/admin/prices', 'Prices'],
  ['/admin/usage', 'All usage'],
]

export default function Layout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  if (!user) return null
  const link = ([to, label]: string[]) => (
    <NavLink key={to} to={to} end={to === '/'}>
      {label}
    </NavLink>
  )
  return (
    <div className="shell">
      <aside className="nav">
        <div className="brand">LLM Gateway</div>
        <nav>
          {devLinks.map(link)}
          {user.role === 'admin' && (
            <>
              <div className="nav-section" data-testid="admin-nav">
                Admin
              </div>
              {adminLinks.map(link)}
            </>
          )}
        </nav>
        <div className="nav-foot">
          <div className="muted small">{user.email}</div>
          <div className="small">Balance {usd(user.balance, 2)}</div>
          <button
            className="link"
            onClick={() => {
              logout()
                .then(() => navigate('/login'))
                .catch((e) => console.error('logout failed', e))
            }}
          >
            Sign out
          </button>
        </div>
      </aside>
      <main className="content">
        <Outlet />
      </main>
    </div>
  )
}

import { useState, type FormEvent } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { api, errMsg } from '../api'
import { useAuth } from '../auth'
import { ErrorBox } from '../components/ui'

export default function Login() {
  const { user, refresh } = useAuth()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [needCode, setNeedCode] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  if (user) return <Navigate to="/" replace />

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      if (needCode) {
        await api('POST', '/api/auth/login/totp', { code })
      } else {
        const r = await api<{ totp_required?: boolean }>('POST', '/api/auth/login', { email, password })
        if (r.totp_required) {
          setNeedCode(true)
          return
        }
      }
      await refresh()
      navigate('/')
    } catch (err) {
      setError(errMsg(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="auth-page">
      <form className="card auth-card" onSubmit={submit}>
        <h1>Sign in</h1>
        <ErrorBox error={error} />
        {needCode ? (
          <label>
            Authenticator code
            <input
              name="code"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.trim())}
              autoFocus
              required
            />
          </label>
        ) : (
          <>
            <label>
              Email
              <input name="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            </label>
            <label>
              Password
              <input name="password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
            </label>
          </>
        )}
        <button className="primary" disabled={busy}>
          {needCode ? 'Verify' : 'Sign in'}
        </button>
        {!needCode && (
          <p className="muted small">
            No account? <Link to="/register">Create one</Link>
          </p>
        )}
      </form>
    </div>
  )
}

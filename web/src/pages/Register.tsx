import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, errMsg } from '../api'
import { useAuth } from '../auth'
import { ErrorBox } from '../components/ui'

export default function Register() {
  const { refresh } = useAuth()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await api('POST', '/api/auth/register', { email, password })
      await api('POST', '/api/auth/login', { email, password })
      await refresh()
      navigate('/')
    } catch (err) {
      setError(errMsg(err))
    }
  }

  return (
    <div className="auth-page">
      <form className="card auth-card" onSubmit={submit}>
        <h1>Create account</h1>
        <ErrorBox error={error} />
        <label>
          Email
          <input name="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </label>
        <label>
          Password (8+ characters)
          <input name="password" type="password" minLength={8} value={password} onChange={(e) => setPassword(e.target.value)} required />
        </label>
        <button className="primary">Create account</button>
        <p className="muted small">
          Have an account? <Link to="/login">Sign in</Link>
        </p>
      </form>
    </div>
  )
}

import { useCallback, useState, type FormEvent } from 'react'
import { api, errMsg, type ApiKey } from '../api'
import { Card, ErrorBox, fmtTime, useLoad } from '../components/ui'

export default function Keys() {
  const { data, error, reload } = useLoad(useCallback(() => api<ApiKey[]>('GET', '/api/keys'), []))
  const [name, setName] = useState('')
  const [created, setCreated] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  async function create(e: FormEvent) {
    e.preventDefault()
    setActionError(null)
    try {
      const r = await api<{ key: string }>('POST', '/api/keys', { name })
      setCreated(r.key)
      setName('')
      reload()
    } catch (err) {
      setActionError(errMsg(err))
    }
  }

  async function toggle(k: ApiKey) {
    setActionError(null)
    try {
      await api('PATCH', `/api/keys/${k.id}`, { disabled: !k.disabled })
      reload()
    } catch (err) {
      setActionError(errMsg(err))
    }
  }

  return (
    <>
      <h1>API Keys</h1>
      <Card title="Create key">
        <form className="row" onSubmit={create}>
          <input name="name" placeholder="Key name, e.g. my-app" value={name} onChange={(e) => setName(e.target.value)} required maxLength={100} />
          <button className="primary">Create</button>
        </form>
        {created && (
          <div className="notice" data-testid="new-key">
            <p>Copy this key now. It will not be shown again.</p>
            <code className="secret">{created}</code>
            <button onClick={() => navigator.clipboard.writeText(created).catch((e) => console.error('copy failed', e))}>Copy</button>
          </div>
        )}
      </Card>
      <ErrorBox error={error ?? actionError} />
      <Card title="Your keys">
        <div className="table-wrap">
          <table>
            <thead>
              <tr><th>Name</th><th>Key</th><th>Limit</th><th>Last used</th><th>Status</th><th /></tr>
            </thead>
            <tbody>
              {data?.map((k) => (
                <tr key={k.id}>
                  <td>{k.name}</td>
                  <td><code>{k.key_prefix}…</code></td>
                  <td>{k.rpm_limit}/min</td>
                  <td>{fmtTime(k.last_used_at)}</td>
                  <td><span className={k.disabled ? 'tag bad' : 'tag ok'}>{k.disabled ? 'Disabled' : 'Active'}</span></td>
                  <td><button onClick={() => toggle(k)}>{k.disabled ? 'Enable' : 'Disable'}</button></td>
                </tr>
              ))}
              {data?.length === 0 && <tr><td colSpan={6} className="muted">No keys yet</td></tr>}
            </tbody>
          </table>
        </div>
      </Card>
    </>
  )
}

import { useCallback, useState } from 'react'
import { api, errMsg, usd, type User } from '../../api'
import { useAuth } from '../../auth'
import { Card, ErrorBox, useLoad } from '../../components/ui'

export default function Users() {
  const { user: me } = useAuth()
  const { data, error, reload } = useLoad(useCallback(() => api<User[]>('GET', '/api/admin/users'), []))
  const [actionError, setActionError] = useState<string | null>(null)

  async function act(fn: () => Promise<unknown>) {
    setActionError(null)
    try {
      await fn()
      reload()
    } catch (err) {
      setActionError(errMsg(err))
    }
  }

  function credit(u: User) {
    const v = prompt(`Adjust balance for ${u.email} (USD, negative to deduct)`, '10')
    if (v === null) return
    void act(() => api('POST', `/api/admin/users/${u.id}/credit`, { amount_usd: Number(v) }))
  }

  return (
    <>
      <h1>Users</h1>
      <ErrorBox error={error ?? actionError} />
      <Card>
        <div className="table-wrap">
          <table>
            <thead>
              <tr><th>Email</th><th>Role</th><th>2FA</th><th>Balance</th><th>Status</th><th>Actions</th></tr>
            </thead>
            <tbody>
              {data?.map((u) => (
                <tr key={u.id}>
                  <td>{u.email}</td>
                  <td>
                    <select
                      aria-label={`role of ${u.email}`}
                      value={u.role}
                      disabled={u.id === me?.id}
                      onChange={(e) => act(() => api('PATCH', `/api/admin/users/${u.id}`, { role: e.target.value }))}
                    >
                      <option value="user">Developer</option>
                      <option value="admin">Admin</option>
                    </select>
                  </td>
                  <td>{u.totp_enabled ? 'On' : 'Off'}</td>
                  <td>{usd(u.balance, 4)}</td>
                  <td><span className={u.disabled ? 'tag bad' : 'tag ok'}>{u.disabled ? 'Disabled' : 'Active'}</span></td>
                  <td><div className="row">
                    <button onClick={() => credit(u)}>Adjust balance</button>
                    {u.id !== me?.id && (
                      <button onClick={() => act(() => api('PATCH', `/api/admin/users/${u.id}`, { disabled: !u.disabled }))}>
                        {u.disabled ? 'Enable' : 'Disable'}
                      </button>
                    )}
                  </div></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </>
  )
}

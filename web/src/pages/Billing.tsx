import { useCallback, useState, type FormEvent } from 'react'
import { api, errMsg, usd } from '../api'
import { useAuth } from '../auth'
import { Card, ErrorBox, fmtTime, useLoad } from '../components/ui'

interface Topup {
  id: number
  amount: number
  source: 'self' | 'admin'
  created_at: string
}

export default function Billing() {
  const { refresh } = useAuth()
  const balance = useLoad(useCallback(() => api<{ balance: number }>('GET', '/api/billing/balance'), []))
  const history = useLoad(useCallback(() => api<Topup[]>('GET', '/api/billing/topups'), []))
  const [amount, setAmount] = useState('10')
  const [error, setError] = useState<string | null>(null)

  async function topup(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await api('POST', '/api/billing/topup', { amount_usd: Number(amount) })
      balance.reload()
      history.reload()
      await refresh()
    } catch (err) {
      setError(errMsg(err))
    }
  }

  return (
    <>
      <h1>Billing</h1>
      <div className="grid">
        <Card title="Balance">
          <ErrorBox error={balance.error} />
          <div className="big" data-testid="balance">{balance.data ? usd(balance.data.balance) : '—'}</div>
          <p className="muted small">Each request is charged by prompt and completion tokens at the model's price.</p>
        </Card>
        <Card title="Top up">
          <form onSubmit={topup}>
            <ErrorBox error={error} />
            <label>
              Amount (USD, 1 – 1000)
              <input name="amount" type="number" min={1} max={1000} step="0.01" value={amount} onChange={(e) => setAmount(e.target.value)} required />
            </label>
            <button className="primary">Top up</button>
            <p className="muted small">Demo payment: the amount is credited immediately.</p>
          </form>
        </Card>
      </div>
      <Card title="History">
        <ErrorBox error={history.error} />
        <div className="table-wrap">
          <table>
            <thead><tr><th>Time</th><th>Amount</th><th>Source</th></tr></thead>
            <tbody>
              {history.data?.map((t) => (
                <tr key={t.id}>
                  <td>{fmtTime(t.created_at)}</td>
                  <td>{usd(t.amount, 2)}</td>
                  <td>{t.source === 'admin' ? 'Admin adjustment' : 'Top-up'}</td>
                </tr>
              ))}
              {history.data?.length === 0 && <tr><td colSpan={3} className="muted">No top-ups yet</td></tr>}
            </tbody>
          </table>
        </div>
      </Card>
    </>
  )
}

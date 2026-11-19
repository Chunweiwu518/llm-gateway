import { useCallback } from 'react'
import { Link } from 'react-router-dom'
import { api, usd } from '../api'
import { Card, ErrorBox, useLoad } from '../components/ui'

interface Day {
  day: string
  requests: number
  tokens: number
  cost: number
}

export default function Dashboard() {
  const balance = useLoad(useCallback(() => api<{ balance: number }>('GET', '/api/billing/balance'), []))
  const daily = useLoad(useCallback(() => api<Day[]>('GET', '/api/usage/daily'), []))
  const max = Math.max(1, ...(daily.data ?? []).map((d) => d.requests))
  const curl = `curl ${location.origin}/v1/chat/completions \\
  -H "Authorization: Bearer $GATEWAY_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}'`

  return (
    <>
      <h1>Overview</h1>
      <div className="grid">
        <Card title="Balance">
          <ErrorBox error={balance.error} />
          <div className="big" data-testid="balance">{balance.data ? usd(balance.data.balance, 6) : '—'}</div>
          <Link to="/billing">Top up →</Link>
        </Card>
        <Card title="Last 7 days">
          <ErrorBox error={daily.error} />
          {daily.data && daily.data.length === 0 && <p className="muted">No requests in the last 7 days.</p>}
          <div className="bars">
            {daily.data?.map((d) => (
              <div key={d.day} className="bar-row" title={`${d.tokens} tokens, ${usd(d.cost, 6)}`}>
                <span className="muted small">{d.day.slice(5)}</span>
                <div className="bar" style={{ width: `${(d.requests / max) * 100}%` }} />
                <span className="small">{d.requests} req · {usd(d.cost, 6)}</span>
              </div>
            ))}
          </div>
        </Card>
      </div>
      <Card title="Quick start">
        <p className="muted">Create a key on the <Link to="/keys">API Keys</Link> page, then call the OpenAI-compatible endpoint:</p>
        <pre>{curl}</pre>
      </Card>
    </>
  )
}

import { useCallback, useState } from 'react'
import { api, usd, type UsagePage } from '../api'
import { Card, ErrorBox, fmtTime, useLoad } from './ui'

/** Usage list with date filter and paging. endpoint is /api/usage or /api/admin/usage. */
export default function UsageTable({ endpoint, showUser }: { endpoint: string; showUser?: boolean }) {
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [page, setPage] = useState(1)
  const load = useCallback(() => {
    const q = new URLSearchParams({ page: String(page) })
    if (from) q.set('from', from)
    if (to) q.set('to', to)
    return api<UsagePage>('GET', `${endpoint}?${q}`)
  }, [endpoint, from, to, page])
  const { data, error } = useLoad(load)
  const pages = data ? Math.max(1, Math.ceil(data.count / data.page_size)) : 1

  return (
    <Card
      title="Requests"
      actions={
        <div className="row">
          <input type="date" aria-label="from" value={from} onChange={(e) => { setFrom(e.target.value); setPage(1) }} />
          <input type="date" aria-label="to" value={to} onChange={(e) => { setTo(e.target.value); setPage(1) }} />
        </div>
      }
    >
      <ErrorBox error={error} />
      {data && (
        <>
          <div className="stats">
            <div><span className="muted">Requests</span><b>{data.count}</b></div>
            <div><span className="muted">Prompt tokens</span><b>{data.prompt_tokens}</b></div>
            <div><span className="muted">Completion tokens</span><b>{data.completion_tokens}</b></div>
            <div><span className="muted">Cost</span><b>{usd(data.cost, 6)}</b></div>
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Time</th>
                  {showUser && <th>User</th>}
                  <th>Model</th>
                  <th>Channel</th>
                  <th>Status</th>
                  <th>Tokens (in/out)</th>
                  <th>Cost</th>
                  <th>Latency</th>
                  <th>Failover</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((u) => (
                  <tr key={u.id}>
                    <td>{fmtTime(u.created_at)}</td>
                    {showUser && <td>{u.user_id}</td>}
                    <td>{u.model}</td>
                    <td>{u.channel_name || '—'}</td>
                    <td><span className={u.status_code === 200 ? 'tag ok' : 'tag bad'}>{u.status_code}</span></td>
                    <td>{u.prompt_tokens} / {u.completion_tokens}</td>
                    <td>{usd(u.cost, 6)}</td>
                    <td>{u.latency_ms} ms</td>
                    <td>{u.failover_count}</td>
                  </tr>
                ))}
                {data.items.length === 0 && (
                  <tr><td colSpan={9} className="muted">No requests yet</td></tr>
                )}
              </tbody>
            </table>
          </div>
          <div className="row pager">
            <button disabled={page <= 1} onClick={() => setPage(page - 1)}>Previous</button>
            <span className="muted">Page {page} / {pages}</span>
            <button disabled={page >= pages} onClick={() => setPage(page + 1)}>Next</button>
          </div>
        </>
      )}
    </Card>
  )
}

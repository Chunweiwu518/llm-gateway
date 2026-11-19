import { useCallback, useState, type FormEvent } from 'react'
import { api, errMsg, type Price } from '../../api'
import { Card, ErrorBox, useLoad } from '../../components/ui'

/** Prices are entered in USD per 1M tokens and stored as micro-USD. */
const toMicro = (usd: string) => Math.round(Number(usd) * 1_000_000)
const toUsd = (micro: number) => (micro / 1_000_000).toString()

export default function Prices() {
  const { data, error, reload } = useLoad(useCallback(() => api<Price[]>('GET', '/api/admin/prices'), []))
  const [form, setForm] = useState({ model: '', input: '', output: '' })
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

  function save(e: FormEvent) {
    e.preventDefault()
    void act(async () => {
      await api('PUT', `/api/admin/prices/${encodeURIComponent(form.model)}`, {
        input_per_m: toMicro(form.input),
        output_per_m: toMicro(form.output),
      })
      setForm({ model: '', input: '', output: '' })
    })
  }

  return (
    <>
      <h1>Model prices</h1>
      <ErrorBox error={error ?? actionError} />
      <Card title="Add or update">
        <form className="row wrap" onSubmit={save}>
          <input placeholder="Model, e.g. gpt-4o-mini" value={form.model} onChange={(e) => setForm({ ...form, model: e.target.value })} required />
          <input type="number" min={0} step="0.000001" placeholder="Input $ / 1M tokens" value={form.input} onChange={(e) => setForm({ ...form, input: e.target.value })} required />
          <input type="number" min={0} step="0.000001" placeholder="Output $ / 1M tokens" value={form.output} onChange={(e) => setForm({ ...form, output: e.target.value })} required />
          <button className="primary">Save</button>
        </form>
      </Card>
      <Card>
        <div className="table-wrap">
          <table>
            <thead><tr><th>Model</th><th>Input $ / 1M</th><th>Output $ / 1M</th><th /></tr></thead>
            <tbody>
              {data?.map((p) => (
                <tr key={p.model}>
                  <td>{p.model}</td>
                  <td>{toUsd(p.input_per_m)}</td>
                  <td>{toUsd(p.output_per_m)}</td>
                  <td><div className="row">
                    <button onClick={() => setForm({ model: p.model, input: toUsd(p.input_per_m), output: toUsd(p.output_per_m) })}>Edit</button>
                    <button onClick={() => confirm(`Delete price for ${p.model}?`) && act(() => api('DELETE', `/api/admin/prices/${encodeURIComponent(p.model)}`))}>Delete</button>
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

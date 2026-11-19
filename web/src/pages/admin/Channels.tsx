import { useCallback, useState, type FormEvent } from 'react'
import { api, errMsg, type Channel } from '../../api'
import { Card, ErrorBox, useLoad } from '../../components/ui'

const empty = { id: 0, name: '', type: 'openai', base_url: '', api_key: '', models: '', model_map: '', priority: '1' }
type Form = typeof empty

/** model_map is edited as "from=to" pairs separated by commas. */
const mapToText = (m: Record<string, string>) => Object.entries(m).map(([k, v]) => `${k}=${v}`).join(', ')
const textToMap = (s: string) =>
  Object.fromEntries(s.split(',').map((p) => p.split('=').map((x) => x.trim())).filter((p) => p.length === 2 && p[0] && p[1]))
const list = (s: string) => s.split(',').map((x) => x.trim()).filter(Boolean)

export default function Channels() {
  const { data, error, reload } = useLoad(useCallback(() => api<Channel[]>('GET', '/api/admin/channels'), []))
  const [form, setForm] = useState<Form>(empty)
  const [actionError, setActionError] = useState<string | null>(null)
  const set = (k: keyof Form) => (e: { target: { value: string } }) => setForm({ ...form, [k]: e.target.value })

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
    const body = {
      name: form.name, type: form.type, base_url: form.base_url, api_key: form.api_key,
      models: list(form.models), model_map: textToMap(form.model_map), priority: Number(form.priority),
    }
    void act(async () => {
      if (form.id) await api('PATCH', `/api/admin/channels/${form.id}`, body)
      else await api('POST', '/api/admin/channels', { ...body, enabled: true })
      setForm(empty)
    })
  }

  function edit(c: Channel) {
    setForm({ id: c.id, name: c.name, type: c.type, base_url: c.base_url, api_key: '', models: c.models.join(', '),
      model_map: mapToText(c.model_map), priority: String(c.priority) })
  }

  return (
    <>
      <h1>Channels</h1>
      <p className="muted">Requests try enabled channels serving the model in priority order (lowest first). Upstream 5xx, 429 and timeouts move on to the next channel.</p>
      <ErrorBox error={error ?? actionError} />
      <Card title={form.id ? `Edit channel #${form.id}` : 'Add channel'}>
        <form className="form-grid" onSubmit={save}>
          <label>Name<input value={form.name} onChange={set('name')} required /></label>
          <label>Type
            <select value={form.type} onChange={set('type')}>
              <option value="openai">OpenAI</option>
              <option value="anthropic">Anthropic</option>
            </select>
          </label>
          <label>Base URL<input type="url" value={form.base_url} onChange={set('base_url')} required /></label>
          <label>API key{form.id ? ' (blank = keep)' : ''}<input type="password" value={form.api_key} onChange={set('api_key')} required={!form.id} /></label>
          <label>Models (comma separated)<input value={form.models} onChange={set('models')} /></label>
          <label>Model map (from=to)<input value={form.model_map} onChange={set('model_map')} placeholder="gpt-4o-mini=claude-haiku-4-5" /></label>
          <label>Priority<input type="number" min={0} value={form.priority} onChange={set('priority')} /></label>
          <div className="row">
            <button className="primary">{form.id ? 'Save' : 'Add'}</button>
            {form.id > 0 && <button type="button" onClick={() => setForm(empty)}>Cancel</button>}
          </div>
        </form>
      </Card>
      <Card>
        <div className="table-wrap">
          <table>
            <thead><tr><th>Priority</th><th>Name</th><th>Type</th><th>Models</th><th>Key</th><th>Status</th><th /></tr></thead>
            <tbody>
              {data?.map((c) => (
                <tr key={c.id}>
                  <td>{c.priority}</td>
                  <td>{c.name}<div className="muted small">{c.base_url}</div></td>
                  <td>{c.type}</td>
                  <td>{c.models.join(', ')}{Object.keys(c.model_map).length > 0 && <div className="muted small">{mapToText(c.model_map)}</div>}</td>
                  <td><code>{c.api_key_masked}</code></td>
                  <td><span className={c.enabled ? 'tag ok' : 'tag bad'}>{c.enabled ? 'Enabled' : 'Disabled'}</span></td>
                  <td><div className="row">
                    <button onClick={() => edit(c)}>Edit</button>
                    <button onClick={() => act(() => api('PATCH', `/api/admin/channels/${c.id}`, { enabled: !c.enabled }))}>{c.enabled ? 'Disable' : 'Enable'}</button>
                    <button onClick={() => confirm(`Delete ${c.name}?`) && act(() => api('DELETE', `/api/admin/channels/${c.id}`))}>Delete</button>
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

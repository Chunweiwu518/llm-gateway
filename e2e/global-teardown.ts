import { request } from '@playwright/test'
import { ADMIN_EMAIL, ADMIN_PASSWORD, MOCK_URL } from './helpers'

/** Resets the mock upstream and disables every e2e-* account created by this suite. */
export default async function globalTeardown() {
  const ctx = await request.newContext({ baseURL: process.env.BASE_URL ?? 'http://localhost:5173' })
  for (const provider of ['openai', 'anthropic']) {
    await ctx.post(`${MOCK_URL}/_control`, { data: { provider, mode: 'ok' } })
  }
  if (!ADMIN_PASSWORD) {
    console.warn('ADMIN_PASSWORD not set: e2e accounts were left enabled')
    return
  }
  const login = await ctx.post('/api/auth/login', { data: { email: ADMIN_EMAIL, password: ADMIN_PASSWORD } })
  if (!login.ok()) throw new Error(`admin login for cleanup failed: ${login.status()}`)
  const users: { id: number; email: string; disabled: boolean }[] = await (await ctx.get('/api/admin/users')).json()
  const left = users.filter((u) => u.email.startsWith('e2e-') && !u.disabled)
  for (const u of left) {
    const r = await ctx.patch(`/api/admin/users/${u.id}`, { data: { disabled: true } })
    if (!r.ok()) throw new Error(`disable ${u.email} failed: ${r.status()}`)
  }
  console.log(`cleanup: disabled ${left.length} e2e accounts`)
  await ctx.dispose()
}

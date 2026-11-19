import { request } from '@playwright/test'

/** Fails fast when the stack is down or runs a different build than expected. */
export default async function globalSetup() {
  const ctx = await request.newContext({ baseURL: process.env.BASE_URL ?? 'http://localhost:5173' })
  const r = await ctx.get('/healthz')
  if (!r.ok()) throw new Error(`gateway not healthy: ${r.status()}`)
  const { commit } = await r.json()
  const expected = process.env.EXPECTED_COMMIT
  if (expected && commit !== expected) throw new Error(`gateway runs ${commit}, expected ${expected}`)
  console.log(`gateway commit ${commit}`)
  await ctx.dispose()
}

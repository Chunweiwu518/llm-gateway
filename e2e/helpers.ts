import { createHmac } from 'node:crypto'
import { expect, type APIRequestContext, type Page } from '@playwright/test'

export const MOCK_URL = process.env.MOCK_URL ?? 'http://localhost:9000'
export const ADMIN_EMAIL = process.env.ADMIN_EMAIL ?? 'admin@example.com'
export const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD ?? ''
export const RUN_ID = process.env.E2E_RUN_ID ?? `${Date.now()}`
export const PASSWORD = 'e2e-password-123'

/** Unique test email with the e2e-<runId>- prefix so teardown can find it. */
export function testEmail(tag: string): string {
  return `e2e-${RUN_ID}-${tag}-${Math.random().toString(36).slice(2, 7)}@example.com`
}

/** RFC 6238 TOTP (SHA-1, 30 s, 6 digits) for a base32 secret. */
export function totp(secret: string, at = Date.now()): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const c of secret.replace(/=+$/, '').toUpperCase()) bits += alphabet.indexOf(c).toString(2).padStart(5, '0')
  const key = Buffer.from(bits.match(/.{8}/g)!.map((b) => parseInt(b, 2)))
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 30000)))
  const h = createHmac('sha1', key).update(counter).digest()
  const o = h[h.length - 1] & 0xf
  return String((h.readUInt32BE(o) & 0x7fffffff) % 1_000_000).padStart(6, '0')
}

/** Registers and logs in through the API; the request context keeps the session cookie. */
export async function apiUser(request: APIRequestContext, tag: string): Promise<string> {
  const email = testEmail(tag)
  expect((await request.post('/api/auth/register', { data: { email, password: PASSWORD } })).status()).toBe(201)
  expect((await request.post('/api/auth/login', { data: { email, password: PASSWORD } })).status()).toBe(200)
  return email
}

export async function createKey(request: APIRequestContext): Promise<string> {
  const r = await request.post('/api/keys', { data: { name: `e2e-${RUN_ID}` } })
  expect(r.status()).toBe(201)
  return (await r.json()).key
}

export async function chat(request: APIRequestContext, key: string, content: string) {
  return request.post('/v1/chat/completions', {
    headers: { Authorization: `Bearer ${key}` },
    data: { model: 'gpt-4o-mini', messages: [{ role: 'user', content }] },
  })
}

export async function setMock(request: APIRequestContext, provider: string, mode: string) {
  const r = await request.post(`${MOCK_URL}/_control`, { data: { provider, mode } })
  expect(r.status()).toBe(200)
}

export async function uiLogin(page: Page, email: string, password = PASSWORD) {
  await page.goto('/login')
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
}

import { expect, test } from '@playwright/test'
import { PASSWORD, apiUser, chat, createKey, setMock, testEmail, totp, uiLogin } from '../helpers'

test('developer flow: register, top up, create key, call API, see usage', async ({ page, request }) => {
  const email = testEmail('dev')
  await page.goto('/register')
  await page.getByLabel('Email').fill(email)
  await page.getByLabel(/Password/).fill(PASSWORD)
  await page.getByRole('button', { name: 'Create account' }).click()
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()

  await page.getByRole('link', { name: 'Billing' }).click()
  await page.getByLabel(/Amount/).fill('5')
  await page.getByRole('button', { name: 'Top up' }).click()
  await expect(page.getByTestId('balance')).toHaveText('$5.000000')

  await page.getByRole('link', { name: 'API Keys' }).click()
  await page.getByPlaceholder(/Key name/).fill('e2e-key')
  await page.getByRole('button', { name: 'Create' }).click()
  const key = (await page.getByTestId('new-key').locator('code').textContent())!.trim()
  expect(key).toMatch(/^sk-.{48}$/)

  const r = await chat(request, key, 'hello from e2e')
  expect(r.status()).toBe(200)
  const body = await r.json()
  expect(body.choices[0].message.content).toBe('[mock-openai] hello from e2e')

  await page.getByRole('link', { name: 'Usage' }).click()
  await expect(page.getByRole('cell', { name: 'OpenAI (mock)' })).toBeVisible()
  await page.getByRole('link', { name: 'Billing' }).click()
  await expect(page.getByTestId('balance')).not.toHaveText('$5.000000')
})

test('two-factor login asks for an authenticator code', async ({ page, request }) => {
  const email = await apiUser(request, 'totp')
  const { secret } = await (await request.post('/api/me/totp/setup')).json()
  expect((await request.post('/api/me/totp/enable', { data: { code: totp(secret) } })).status()).toBe(200)

  await uiLogin(page, email)
  await page.getByLabel('Authenticator code').fill('000000')
  await page.getByRole('button', { name: 'Verify' }).click()
  await expect(page.getByRole('alert')).toContainText('invalid code')

  await page.getByLabel('Authenticator code').fill(totp(secret))
  await page.getByRole('button', { name: 'Verify' }).click()
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
})

test('fails over to Anthropic when OpenAI returns 500', async ({ page, request }) => {
  const email = await apiUser(request, 'failover')
  await request.post('/api/billing/topup', { data: { amount_usd: 1 } })
  const key = await createKey(request)
  await setMock(request, 'openai', '500')
  try {
    const r = await chat(request, key, 'please fail over')
    expect(r.status()).toBe(200)
    expect((await r.json()).choices[0].message.content).toBe('[mock-anthropic] please fail over')
  } finally {
    await setMock(request, 'openai', 'ok')
  }
  await uiLogin(page, email)
  await page.getByRole('link', { name: 'Usage' }).click()
  const row = page.getByRole('row', { name: /Anthropic \(mock\)/ })
  await expect(row).toBeVisible()
  await expect(row.getByRole('cell').last()).toHaveText('1')
})

test('returns 402 when the balance is empty', async ({ page, request }) => {
  const email = await apiUser(request, 'broke')
  const key = await createKey(request)
  const r = await chat(request, key, 'no money')
  expect(r.status()).toBe(402)
  expect((await r.json()).error.code).toBe('insufficient_quota')
  await uiLogin(page, email)
  await expect(page.getByTestId('balance')).toHaveText('$0.000000')
})

test('developer cannot see or call admin features', async ({ page, request }) => {
  const email = await apiUser(request, 'noadmin')
  for (const path of ['/api/admin/users', '/api/admin/channels', '/api/admin/prices', '/api/admin/usage']) {
    expect((await request.get(path)).status(), path).toBe(403)
  }
  expect((await request.put('/api/admin/prices/gpt-4o-mini', { data: { input_per_m: 0, output_per_m: 0 } })).status()).toBe(403)
  await uiLogin(page, email)
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  await expect(page.getByTestId('admin-nav')).toHaveCount(0)
  await page.goto('/admin/channels')
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  await expect(page).toHaveURL(/\/$/)
})

test('anonymous requests are rejected', async ({ request }) => {
  expect((await request.get('/api/me')).status()).toBe(401)
  expect((await request.get('/api/admin/users')).status()).toBe(401)
  const r = await request.post('/v1/chat/completions', { data: { model: 'gpt-4o-mini', messages: [] } })
  expect(r.status()).toBe(401)
})

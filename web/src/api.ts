export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

/** Calls the console API. Throws ApiError with the server's message when the response is not ok. */
export async function api<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await res.text()
  let data: unknown = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    console.error('non-JSON response', res.status, text.slice(0, 200))
  }
  if (!res.ok) {
    const err = (data as { error?: unknown } | null)?.error
    const msg = typeof err === 'string' ? err : (err as { message?: string } | undefined)?.message
    throw new ApiError(res.status, msg ?? `Request failed (${res.status})`)
  }
  return data as T
}

export const MICRO = 1_000_000

/** Formats micro-USD as dollars. */
export function usd(micro: number, digits = 4): string {
  return `$${(micro / MICRO).toFixed(digits)}`
}

export function errMsg(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export interface User {
  id: number
  email: string
  role: 'user' | 'admin'
  totp_enabled: boolean
  balance: number
  disabled: boolean
  created_at: string
}

export interface ApiKey {
  id: number
  name: string
  key_prefix: string
  rpm_limit: number
  disabled: boolean
  last_used_at: string | null
  created_at: string
}

export interface UsageLog {
  id: number
  user_id: number
  model: string
  channel_name: string
  prompt_tokens: number
  completion_tokens: number
  cost: number
  status_code: number
  latency_ms: number
  failover_count: number
  created_at: string
}

export interface UsagePage {
  items: UsageLog[]
  page: number
  page_size: number
  count: number
  prompt_tokens: number
  completion_tokens: number
  cost: number
}

export interface Channel {
  id: number
  name: string
  type: 'openai' | 'anthropic'
  base_url: string
  models: string[]
  model_map: Record<string, string>
  priority: number
  enabled: boolean
  api_key_masked: string
}

export interface Price {
  model: string
  input_per_m: number
  output_per_m: number
}

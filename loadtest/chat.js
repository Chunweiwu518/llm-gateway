// k6 script: constant virtual users POSTing /v1/chat/completions (mockupstream answers).
// Keys come from keys.json (made by bootstrap.sh); each VU sticks to one key.
import http from 'k6/http'
import { check } from 'k6'

const keys = JSON.parse(open('/loadtest/keys.json'))
const base = __ENV.BASE_URL || 'http://127.0.0.1:8080'

export const options = {
  scenarios: {
    steady: { executor: 'constant-vus', vus: Number(__ENV.VUS || 64), duration: __ENV.DURATION || '30s' },
  },
  // A run only counts when (almost) every call was a real 200.
  thresholds: { checks: ['rate>0.99'], http_req_failed: ['rate<0.01'] },
}

const body = JSON.stringify({
  model: 'gpt-4o-mini',
  max_tokens: 64,
  messages: [{ role: 'user', content: 'Hello from k6' }],
})

export default function () {
  const key = keys[(__VU - 1) % keys.length]
  const res = http.post(`${base}/v1/chat/completions`, body, {
    headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' },
  })
  check(res, { 'status 200': (r) => r.status === 200 })
}

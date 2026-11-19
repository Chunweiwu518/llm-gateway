import { useEffect, useState, type FormEvent } from 'react'
import QRCode from 'qrcode'
import { api, errMsg } from '../api'
import { useAuth } from '../auth'
import { Card, ErrorBox } from '../components/ui'

export default function Security() {
  const { user, refresh } = useAuth()
  const [setup, setSetup] = useState<{ otpauth_url: string; secret: string } | null>(null)
  const [qr, setQr] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!setup) return
    QRCode.toDataURL(setup.otpauth_url)
      .then(setQr)
      .catch((e) => {
        console.error('QR render failed', e)
        setError('Could not render the QR code; enter the secret manually.')
      })
  }, [setup])

  async function run(fn: () => Promise<void>) {
    setError(null)
    try {
      await fn()
    } catch (err) {
      setError(errMsg(err))
    }
  }

  const start = () => run(async () => setSetup(await api('POST', '/api/me/totp/setup')))

  const confirm = (path: string) => (e: FormEvent) => {
    e.preventDefault()
    void run(async () => {
      await api('POST', path, { code })
      setSetup(null)
      setQr(null)
      setCode('')
      await refresh()
    })
  }

  const codeInput = (
    <label>
      6-digit code
      <input name="code" inputMode="numeric" maxLength={6} value={code} onChange={(e) => setCode(e.target.value.trim())} required />
    </label>
  )

  return (
    <>
      <h1>Security</h1>
      <Card title="Two-factor authentication">
        <ErrorBox error={error} />
        {user?.totp_enabled ? (
          <form onSubmit={confirm('/api/me/totp/disable')}>
            <p><span className="tag ok">Enabled</span> Sign-in asks for a code from your authenticator app.</p>
            {codeInput}
            <button>Disable two-factor</button>
          </form>
        ) : setup ? (
          <form onSubmit={confirm('/api/me/totp/enable')}>
            <p>Scan with Google Authenticator, 1Password or similar, then enter the current code.</p>
            {qr && <img className="qr" src={qr} alt="TOTP QR code" />}
            <p className="small">Secret: <code data-testid="totp-secret">{setup.secret}</code></p>
            {codeInput}
            <button className="primary">Enable</button>
          </form>
        ) : (
          <>
            <p><span className="tag bad">Off</span> Protect your account with an authenticator app.</p>
            <button className="primary" onClick={start}>Set up two-factor</button>
          </>
        )}
      </Card>
    </>
  )
}

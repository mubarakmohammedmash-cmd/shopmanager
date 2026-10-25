import { useEffect, useState } from 'react'
import { api } from './api.js'

export default function Login({ onLogin }) {
  const [setupNeeded, setSetupNeeded] = useState(null)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api('/auth/status')
      .then((d) => setSetupNeeded(d.setup_needed))
      .catch(() => setError('Cannot reach the server. Is the Go server running?'))
  }, [])

  async function submit(e) {
    e.preventDefault()
    setError('')
    if (setupNeeded && password !== confirm) {
      setError('The two passwords do not match')
      return
    }
    setBusy(true)
    try {
      const data = await api(setupNeeded ? '/auth/setup' : '/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      })
      onLogin(data)
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div style={{ maxWidth: 380, margin: '10vh auto', padding: 20 }}>
      <div className="card">
        <h2 style={{ fontSize: 22 }}>ShopManager</h2>
        {setupNeeded === null && !error && <div className="empty">Loading...</div>}
        {setupNeeded && (
          <p className="muted">First time here. Create the manager account. Only the manager sees cost prices and profit.</p>
        )}
        {setupNeeded !== null && (
          <form onSubmit={submit} style={{ display: 'grid', gap: 12 }}>
            <label className="field"><span>Username</span>
              <input required autoFocus autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} />
            </label>
            <label className="field"><span>Password</span>
              <input required type="password" autoComplete={setupNeeded ? 'new-password' : 'current-password'} value={password} onChange={(e) => setPassword(e.target.value)} />
            </label>
            {setupNeeded && (
              <label className="field"><span>Repeat password (at least 6 characters)</span>
                <input required type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
              </label>
            )}
            <button type="submit" disabled={busy}>
              {busy ? 'Please wait...' : setupNeeded ? 'Create manager account' : 'Log in'}
            </button>
          </form>
        )}
        {error && <div className="low" style={{ marginTop: 12 }}>{error}</div>}
      </div>
    </div>
  )
}

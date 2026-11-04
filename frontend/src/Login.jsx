import { useEffect, useState } from 'react'
import { api } from './api.js'

export default function Login({ onLogin }) {
  const [setupNeeded, setSetupNeeded] = useState(null)
  const [mode, setMode] = useState('signup')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [storeName, setStoreName] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api('/auth/status')
      .then((d) => {
        setSetupNeeded(d.setup_needed)
        setMode(d.setup_needed ? 'signup' : 'signin')
      })
      .catch(() => setError('Cannot reach the server. Is the Go server running?'))
  }, [])

  async function submit(e) {
    e.preventDefault()
    setError('')
    const signingUp = mode === 'signup'
    if (signingUp && password !== confirm) {
      setError('The two passwords do not match')
      return
    }
    setBusy(true)
    try {
      const data = await api(signingUp ? '/auth/signup' : '/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password, store_name: storeName }),
      })
      onLogin(data)
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-shell">
      <div className="card">
        <h2 style={{ fontSize: 22 }}>ShopManager</h2>
        {setupNeeded === null && !error && <div className="empty">Loading...</div>}
        {setupNeeded !== null && (
          <div className="auth-options" role="tablist" aria-label="Account options">
            <button type="button" className={mode === 'signin' ? 'active' : ''} onClick={() => { setMode('signin'); setError('') }}>
              Sign in
            </button>
            <button type="button" className={mode === 'signup' ? 'active' : ''} onClick={() => { setMode('signup'); setError('') }}>
              Sign up
            </button>
          </div>
        )}
        {setupNeeded !== null && (
          <form onSubmit={submit} style={{ display: 'grid', gap: 12 }}>
            {mode === 'signup' && (
              <>
                <p className="muted">Create the manager account for your store.</p>
                <label className="field"><span>Store name</span>
                  <input required autoFocus maxLength={80} autoComplete="organization" value={storeName} onChange={(e) => setStoreName(e.target.value)} />
                </label>
              </>
            )}
            <label className="field"><span>Username</span>
              <input required autoFocus={mode === 'signin'} autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} />
            </label>
            <label className="field"><span>Password</span>
              <input required type="password" autoComplete={mode === 'signup' ? 'new-password' : 'current-password'} value={password} onChange={(e) => setPassword(e.target.value)} />
            </label>
            {mode === 'signup' && (
              <label className="field"><span>Repeat password (at least 6 characters)</span>
                <input required type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
              </label>
            )}
            <button type="submit" disabled={busy}>
              {busy ? 'Please wait...' : mode === 'signup' ? 'Create store account' : 'Sign in'}
            </button>
          </form>
        )}
        {error && <div className="low" style={{ marginTop: 12 }}>{error}</div>}
      </div>
    </div>
  )
}

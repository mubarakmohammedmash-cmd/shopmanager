export const API = import.meta.env.VITE_API_URL || (import.meta.env.DEV ? 'http://localhost:8080' : '/api')

const KEY = 'shopmanager-auth'
const sessionChecks = new Map()

export function getAuth() {
  try {
    localStorage.removeItem(KEY)
  } catch {
    // Continue with this tab's session if shared storage is unavailable.
  }
  try {
    return JSON.parse(sessionStorage.getItem(KEY))
  } catch {
    return null
  }
}

export function setAuth(auth) {
  localStorage.removeItem(KEY)
  sessionStorage.setItem(KEY, JSON.stringify(auth))
}

export function clearAuth() {
  localStorage.removeItem(KEY)
  sessionStorage.removeItem(KEY)
}

async function sessionIsInvalid(auth) {
  if (!sessionChecks.has(auth.token)) {
    const check = fetch(API + '/auth/me', {
      headers: { Authorization: 'Bearer ' + auth.token },
      cache: 'no-store',
    })
      .then(async (response) => {
        if (response.status === 401) return true
        if (!response.ok) {
          throw new Error(`session check returned HTTP ${response.status}`)
        }
        const user = await response.json()
        return !user?.id || user.id !== auth.user?.id
      })
      .finally(() => {
        sessionChecks.delete(auth.token)
      })
    sessionChecks.set(auth.token, check)
  }
  return sessionChecks.get(auth.token)
}

export async function api(path, options = {}) {
  const auth = getAuth()
  const headers = { ...(options.headers || {}) }
  if (auth && auth.token) headers.Authorization = 'Bearer ' + auth.token
  const res = await fetch(API + path, { ...options, cache: 'no-store', headers })
  let invalidSession = false
  if (res.status === 401 && !path.startsWith('/auth/')) {
    if (!auth?.token) {
      clearAuth()
      window.location.reload()
      throw new Error('Your sign-in is missing. Please sign in again.')
    }
    try {
      invalidSession = await sessionIsInvalid(auth)
    } catch (error) {
      throw new Error(`The server rejected ${path}, and sign-in could not be verified: ${error.message}`)
    }
    if (invalidSession) {
      clearAuth()
      window.location.reload()
    }
  }
  if (!res.ok) {
    const message = (await res.text()).trim()
    if (res.status === 401 && auth && !path.startsWith('/auth/') && !invalidSession) {
      throw new Error(`The server rejected ${path} even though /auth/me confirmed this account. ${message}`)
    }
    throw new Error(message)
  }
  if (res.status === 204) return null
  const text = await res.text()
  return text ? JSON.parse(text) : null
}

export async function downloadCSV(from, to) {
  const auth = getAuth()
  const res = await fetch(`${API}/reports/export?from=${from}&to=${to}`, {
    headers: { Authorization: 'Bearer ' + (auth ? auth.token : '') },
  })
  if (!res.ok) throw new Error((await res.text()).trim())
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `sales-${from}-to-${to}.csv`
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

export const naira = (kobo) =>
  '₦' + (kobo / 100).toLocaleString('en-NG', { maximumFractionDigits: 2 })

export const toKobo = (v) => Math.round(Number(v) * 100)

export const when = (s) =>
  new Date(s.replace(' ', 'T') + 'Z').toLocaleString('en-NG', {
    dateStyle: 'medium',
    timeStyle: 'short',
  })

// Lagos date (UTC+1) as YYYY-MM-DD, optionally some days ago
export const lagos = (daysAgo = 0) =>
  new Date(Date.now() + 3600 * 1000 - daysAgo * 86400000).toISOString().slice(0, 10)

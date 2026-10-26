export const API = import.meta.env.DEV ? 'http://localhost:8080' : '/api'

const KEY = 'shopmanager-auth'

export function getAuth() {
  try {
    return JSON.parse(localStorage.getItem(KEY))
  } catch {
    return null
  }
}

export function setAuth(auth) {
  localStorage.setItem(KEY, JSON.stringify(auth))
}

export function clearAuth() {
  localStorage.removeItem(KEY)
}

export async function api(path, options = {}) {
  const auth = getAuth()
  const headers = { ...(options.headers || {}) }
  if (auth && auth.token) headers.Authorization = 'Bearer ' + auth.token
  const res = await fetch(API + path, { ...options, headers })
  if (res.status === 401 && auth && !path.startsWith('/auth/')) {
    clearAuth()
    window.location.reload()
  }
  if (!res.ok) throw new Error((await res.text()).trim())
  return res.json()
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


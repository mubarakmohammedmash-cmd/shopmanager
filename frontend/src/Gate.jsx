import { useState } from 'react'
import App from './App.jsx'
import Login from './Login.jsx'
import { api, getAuth, setAuth, clearAuth } from './api.js'

export default function Gate() {
  const [user, setUser] = useState(() => {
    const auth = getAuth()
    return auth ? auth.user : null
  })

  async function logout() {
    try {
      await api('/auth/logout', { method: 'POST' })
    } catch {
      // ignore: we log out locally either way
    }
    clearAuth()
    setUser(null)
  }

  function updateStoreName(storeName) {
    const auth = getAuth()
    if (!auth) return
    const updatedAuth = { ...auth, user: { ...auth.user, store_name: storeName } }
    setAuth(updatedAuth)
    setUser(updatedAuth.user)
  }

  if (!user) {
    return (
      <Login
        onLogin={(auth) => {
          setAuth(auth)
          setUser(auth.user)
        }}
      />
    )
  }
  return <App user={user} onLogout={logout} onStoreNameChange={updateStoreName} />
}

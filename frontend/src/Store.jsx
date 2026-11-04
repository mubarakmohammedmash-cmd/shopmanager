import { useEffect, useState } from 'react'
import { api } from './api.js'

export default function StoreScreen({ notify, onStoreNameChange }) {
  const [storeName, setStoreName] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api('/store')
      .then((store) => setStoreName(store.store_name))
      .catch((error) => notify(error.message, 'error'))
  }, [notify])

  async function save(e) {
    e.preventDefault()
    setBusy(true)
    try {
      const store = await api('/store', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ store_name: storeName }),
      })
      setStoreName(store.store_name)
      onStoreNameChange(store.store_name)
      notify('Store name updated')
    } catch (error) {
      notify(error.message, 'error')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="card">
      <h2>Store details</h2>
      <p className="muted">Set the name shown in the app header after you sign in.</p>
      <form className="form-grid" onSubmit={save}>
        <label className="field"><span>Store name</span>
          <input required maxLength={80} value={storeName} onChange={(e) => setStoreName(e.target.value)} />
        </label>
        <button type="submit" disabled={busy}>
          {busy ? 'Saving...' : 'Save store name'}
        </button>
      </form>
    </div>
  )
}

import { useEffect, useState } from 'react'
import { api, when } from './api.js'

export default function UsersScreen({ notify, currentUserId }) {
  const [users, setUsers] = useState([])
  const [form, setForm] = useState({ username: '', password: '', role: 'rep' })
  const [resetId, setResetId] = useState(null)
  const [newPw, setNewPw] = useState('')

  function loadUsers() {
    api('/users')
      .then(setUsers)
      .catch((e) => notify(e.message, 'error'))
  }

  useEffect(() => {
    loadUsers()
  }, [])

  async function create(e) {
    e.preventDefault()
    try {
      await api('/users', { method: 'POST', body: JSON.stringify(form) })
      setForm({ username: '', password: '', role: 'rep' })
      notify('Account created')
      loadUsers()
    } catch (err) {
      notify(err.message, 'error')
    }
  }

  async function savePassword(u) {
    try {
      await api(`/users/${u.id}/password`, { method: 'POST', body: JSON.stringify({ password: newPw }) })
      setResetId(null)
      setNewPw('')
      notify(`Password changed for ${u.username}`)
    } catch (err) {
      notify(err.message, 'error')
    }
  }

  async function toggle(u) {
    try {
      await api(`/users/${u.id}/active`, { method: 'POST', body: JSON.stringify({ active: !u.active }) })
      notify(u.active ? `${u.username} can no longer log in` : `${u.username} can log in again`)
      loadUsers()
    } catch (err) {
      notify(err.message, 'error')
    }
  }

  return (
    <>
      <div className="card">
        <h2>Add a staff account</h2>
        <form className="form-grid" onSubmit={create}>
          <label className="field"><span>Username</span>
            <input required autoComplete="off" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} />
          </label>
          <label className="field"><span>Password (at least 6 characters)</span>
            <input required type="password" autoComplete="new-password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} />
          </label>
          <label className="field"><span>Role</span>
            <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
              <option value="rep">Sales rep (sells and voids own sales)</option>
              <option value="admin">Manager (sees everything)</option>
            </select>
          </label>
          <button type="submit">Create account</button>
        </form>
      </div>

      <div className="card">
        <h2>Staff accounts</h2>
        <div className="table-wrap">
          <table>
            <thead>
              <tr><th>Username</th><th>Role</th><th>Status</th><th>Created</th><th></th></tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id}>
                  <td>{u.username}{u.id === currentUserId ? ' (you)' : ''}</td>
                  <td>{u.role === 'admin' ? 'Manager' : 'Sales rep'}</td>
                  <td className={u.active ? 'ok' : 'low'}>{u.active ? 'Active' : 'Disabled'}</td>
                  <td>{when(u.created_at)}</td>
                  <td>
                    {resetId === u.id ? (
                      <div className="actions">
                        <input
                          type="password"
                          autoComplete="new-password"
                          placeholder="New password"
                          value={newPw}
                          onChange={(e) => setNewPw(e.target.value)}
                          style={{ width: 150 }}
                        />
                        <button onClick={() => savePassword(u)}>Save</button>
                        <button className="ghost" onClick={() => { setResetId(null); setNewPw('') }}>Cancel</button>
                      </div>
                    ) : (
                      <div className="actions">
                        <button className="ghost" onClick={() => { setResetId(u.id); setNewPw('') }}>Reset password</button>
                        {u.id !== currentUserId && (
                          <button className={u.active ? 'danger' : ''} onClick={() => toggle(u)}>
                            {u.active ? 'Disable' : 'Enable'}
                          </button>
                        )}
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  )
}

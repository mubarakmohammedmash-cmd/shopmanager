import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, when } from './api.js'

export default function SuperAdminScreen({ notify, currentUserId }) {
  const [users, setUsers] = useState(null)
  const [search, setSearch] = useState('')
  const [busyId, setBusyId] = useState(null)
  const [loadError, setLoadError] = useState('')

  const loadUsers = useCallback(async () => {
    setLoadError('')
    try {
      setUsers(await api('/superadmin/users'))
    } catch (error) {
      setLoadError(error.message)
      notify(error.message, 'error')
    }
  }, [notify])

  useEffect(() => {
    loadUsers()
  }, [loadUsers])

  const shown = useMemo(() => {
    if (!users) return []
    const query = search.trim().toLowerCase()
    if (!query) return users
    return users.filter((user) =>
      `${user.username} ${user.store_name} ${user.role}`.toLowerCase().includes(query),
    )
  }, [users, search])

  async function toggleActive(user) {
    const action = user.active ? 'disable' : 'reactivate'
    if (user.active && !window.confirm(`Disable ${user.username} from ${user.store_name}? They will be signed out and cannot log in.`)) {
      return
    }
    setBusyId(user.id)
    try {
      await api(`/superadmin/users/${user.id}/active`, {
        method: 'POST',
        body: JSON.stringify({ active: !user.active }),
      })
      setUsers((current) =>
        current.map((item) => item.id === user.id ? { ...item, active: !user.active } : item),
      )
      notify(`${user.username} ${action === 'disable' ? 'disabled' : 'reactivated'}`)
    } catch (error) {
      notify(error.message, 'error')
    } finally {
      setBusyId(null)
    }
  }

  return (
    <>
      <div className="card">
        <h2>Super admin</h2>
        <p className="muted">Manage user access across every store. Disabling an account preserves its sales history and revokes active sessions.</p>
        <div className="stats">
          <div className="stat">
            <span>All users</span>
            <strong>{users ? users.length : '—'}</strong>
          </div>
          <div className="stat">
            <span>Active users</span>
            <strong>{users ? users.filter((user) => user.active).length : '—'}</strong>
          </div>
          <div className="stat">
            <span>Stores</span>
            <strong>{users ? new Set(users.map((user) => user.store_id)).size : '—'}</strong>
          </div>
        </div>
      </div>

      <div className="card">
        <h2>Users across stores</h2>
        <input
          aria-label="Search users and stores"
          placeholder="Search username, store, or role..."
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          style={{ marginBottom: 12 }}
        />
        {!users && !loadError && <div className="empty">Loading users...</div>}
        {loadError && (
          <div className="low">
            {loadError}{' '}
            <button type="button" className="ghost" onClick={loadUsers}>Try again</button>
          </div>
        )}
        {users && shown.length === 0 && <div className="empty">No users match this search.</div>}
        {users && shown.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr><th>Username</th><th>Store</th><th>Role</th><th>Status</th><th>Created</th><th></th></tr>
              </thead>
              <tbody>
                {shown.map((user) => (
                  <tr key={user.id}>
                    <td>{user.username}{user.id === currentUserId ? ' (you)' : ''}</td>
                    <td>{user.store_name}</td>
                    <td>{user.role === 'super_admin' ? 'Super admin' : user.role === 'admin' ? 'Manager' : 'Sales rep'}</td>
                    <td className={user.active ? 'ok' : 'low'}>{user.active ? 'Active' : 'Disabled'}</td>
                    <td>{when(user.created_at)}</td>
                    <td>
                      {user.id === currentUserId || user.role === 'super_admin' ? (
                        <span className="muted">Protected</span>
                      ) : (
                        <button
                          className={user.active ? 'danger' : ''}
                          disabled={busyId === user.id}
                          onClick={() => toggleActive(user)}
                        >
                          {busyId === user.id ? 'Saving...' : user.active ? 'Disable' : 'Reactivate'}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </>
  )
}

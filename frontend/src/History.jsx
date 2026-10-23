import { useEffect, useState } from 'react'

const API = 'http://localhost:8080'

const naira = (kobo) =>
  '₦' + (kobo / 100).toLocaleString('en-NG', { maximumFractionDigits: 2 })
const when = (s) =>
  new Date(s.replace(' ', 'T') + 'Z').toLocaleString('en-NG', {
    dateStyle: 'medium',
    timeStyle: 'short',
  })

async function api(path, options) {
  const res = await fetch(API + path, options)
  if (!res.ok) throw new Error((await res.text()).trim())
  return res.json()
}

const label = { cash: 'Cash', transfer: 'Transfer', pos: 'POS' }

export default function HistoryScreen({ notify, reload }) {
  const [sales, setSales] = useState(null)

  function loadHistory() {
    api('/sales/history')
      .then(setSales)
      .catch(() => setSales([]))
  }

  useEffect(() => {
    loadHistory()
  }, [])

  async function voidSale(s) {
    if (!window.confirm(`Void receipt #${s.id} (${naira(s.total)})? The items go back into stock.`)) return
    try {
      await api(`/sales/${s.id}/void`, { method: 'POST' })
      notify('Sale voided and stock restored')
      loadHistory()
      reload()
    } catch (e) {
      notify(e.message, 'error')
    }
  }

  return (
    <div className="card">
      <h2>Transaction history</h2>
      {sales === null && <div className="empty">Loading...</div>}
      {sales && sales.length === 0 && <div className="empty">No sales recorded yet.</div>}
      {sales && sales.length > 0 && (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Receipt</th><th>Time</th><th>Items sold</th><th>Paid by</th>
                <th>Total</th><th>Profit</th><th></th>
              </tr>
            </thead>
            <tbody>
              {sales.map((s) => {
                const voided = s.status === 'voided'
                return (
                  <tr key={s.id} style={voided ? { opacity: 0.5 } : undefined}>
                    <td>#{s.id}</td>
                    <td>{when(s.created_at)}</td>
                    <td style={voided ? { textDecoration: 'line-through' } : undefined}>{s.items}</td>
                    <td>{label[s.payment_method] || s.payment_method}</td>
                    <td>{naira(s.total)}</td>
                    <td className={voided ? '' : 'ok'}>{voided ? '-' : naira(s.profit)}</td>
                    <td>
                      {voided ? (
                        <span className="low">Voided</span>
                      ) : (
                        <button className="ghost" onClick={() => voidSale(s)}>Void</button>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

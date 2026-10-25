import { useEffect, useState } from 'react'
import { api, naira, when } from './api.js'

const label = { cash: 'Cash', transfer: 'Transfer', pos: 'POS' }

export default function HistoryScreen({ notify, reload, isAdmin }) {
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
      <h2>{isAdmin ? 'Transaction history' : 'My sales'}</h2>
      {sales === null && <div className="empty">Loading...</div>}
      {sales && sales.length === 0 && <div className="empty">No sales recorded yet.</div>}
      {sales && sales.length > 0 && (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Receipt</th><th>Time</th><th>Items sold</th><th>Paid by</th>
                {isAdmin && <th>Sold by</th>}
                <th>Total</th>
                {isAdmin && <th>Profit</th>}
                <th></th>
              </tr>
            </thead>
            <tbody>
              {sales.map((s) => {
                const voided = s.status === 'voided'
                return (
                  <tr key={s.id} style={voided ? { opacity: 0.55 } : undefined}>
                    <td>#{s.id}</td>
                    <td>{when(s.created_at)}</td>
                    <td style={voided ? { textDecoration: 'line-through' } : undefined}>{s.items}</td>
                    <td>{label[s.payment_method] || s.payment_method}</td>
                    {isAdmin && <td>{s.sold_by || '-'}</td>}
                    <td>{naira(s.total)}</td>
                    {isAdmin && (
                      <td className={voided ? '' : 'ok'}>
                        {voided || s.profit == null ? '-' : naira(s.profit)}
                      </td>
                    )}
                    <td>
                      {voided ? (
                        <span className="low">
                          Voided{isAdmin && s.voided_by ? ` by ${s.voided_by}` : ''}
                        </span>
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

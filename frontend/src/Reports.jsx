import { useEffect, useState } from 'react'
import { api, downloadCSV, naira, lagos } from './api.js'

const method = { cash: 'Cash', transfer: 'Transfer', pos: 'POS' }

function Stat({ label, value, tone }) {
  return (
    <div className={'stat ' + (tone || '')}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}

export default function ReportsScreen() {
  const [from, setFrom] = useState(lagos(6))
  const [to, setTo] = useState(lagos())
  const [data, setData] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!from || !to) return
    let cancelled = false
    api(`/reports/range?from=${from}&to=${to}`)
      .then((d) => {
        if (!cancelled) {
          setData(d)
          setError('')
        }
      })
      .catch((e) => {
        if (!cancelled) {
          setData(null)
          setError(e.message)
        }
      })
    return () => {
      cancelled = true
    }
  }, [from, to])

  function quick(f) {
    setFrom(f)
    setTo(lagos())
  }

  async function download() {
    try {
      await downloadCSV(from, to)
    } catch (e) {
      setError(e.message)
    }
  }

  return (
    <>
      <div className="card">
        <h2>Reports</h2>
        <div className="form-grid">
          <label className="field"><span>From</span>
            <input type="date" value={from} max={to} onChange={(e) => setFrom(e.target.value)} />
          </label>
          <label className="field"><span>To</span>
            <input type="date" value={to} min={from} max={lagos()} onChange={(e) => setTo(e.target.value)} />
          </label>
          <button className="ghost" onClick={() => quick(lagos())}>Today</button>
          <button className="ghost" onClick={() => quick(lagos(6))}>Last 7 days</button>
          <button className="ghost" onClick={() => quick(lagos(29))}>Last 30 days</button>
          <button className="ghost" onClick={() => quick(lagos().slice(0, 8) + '01')}>This month</button>
          <button onClick={download}>Download CSV</button>
        </div>
      </div>

      {error && <div className="card low">{error}</div>}

      {data && (
        <>
          <div className="stats">
            <Stat label="Sales" value={naira(data.revenue)} />
            <Stat label="Gross profit" value={naira(data.profit)} />
            <Stat label="Expenses" value={naira(data.expenses)} />
            <Stat
              label="Net profit"
              value={naira(data.net_profit)}
              tone={data.net_profit < 0 ? 'bad' : 'good'}
            />
            <Stat label="Transactions" value={data.sales_count} />
            <Stat label="Voided sales" value={data.voided_count} />
            <Stat label="Cash received" value={naira(data.cash_received ?? 0)} />
            <Stat label="Transfers received" value={naira(data.transfer_received ?? 0)} />
            <Stat label="POS received" value={naira(data.pos_received ?? 0)} />
            <Stat label="Cash change paid out" value={naira(data.cash_change_paid ?? 0)} />
            <Stat
              label="Net cash drawer movement"
              value={naira(data.cash_drawer_net ?? 0)}
              tone={(data.cash_drawer_net ?? 0) < 0 ? 'bad' : 'good'}
            />
          </div>

          <div className="grid-2">
            <div className="card">
              <h2>Money received by payment method</h2>
              {data.payments.length === 0 && <div className="empty">No sales in this period.</div>}
              {data.payments.map((p) => (
                <div key={p.method} className="cart-line">
                  <span>{method[p.method] || p.method}</span>
                  <strong>{naira(p.amount)}</strong>
                </div>
              ))}
            </div>

            <div className="card">
              <h2>Top sellers</h2>
              {data.top_products.length === 0 && <div className="empty">No sales in this period.</div>}
              {data.top_products.map((t) => (
                <div key={t.name} className="cart-line">
                  <span>{t.name}</span>
                  <span className="muted">
                    {t.units} sold, <span className="ok">{naira(t.profit)} profit</span>
                  </span>
                </div>
              ))}
            </div>
          </div>

          <div className="card">
            <h2>Day by day</h2>
            {data.days.length === 0 && <div className="empty">No sales in this period.</div>}
            {data.days.length > 0 && (
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr><th>Date</th><th>Sales</th><th>Gross profit</th></tr>
                  </thead>
                  <tbody>
                    {data.days.map((d) => (
                      <tr key={d.date}>
                        <td>{d.date}</td>
                        <td>{naira(d.revenue)}</td>
                        <td className="ok">{naira(d.profit)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </>
      )}
    </>
  )
}

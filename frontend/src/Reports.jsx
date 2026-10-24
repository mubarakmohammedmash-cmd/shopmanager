import { useEffect, useState } from 'react'

const API = 'http://localhost:8080'

const naira = (kobo) =>
  '₦' + (kobo / 100).toLocaleString('en-NG', { maximumFractionDigits: 2 })

// Lagos date (UTC+1) as YYYY-MM-DD, optionally some days ago
const lagos = (daysAgo = 0) =>
  new Date(Date.now() + 3600 * 1000 - daysAgo * 86400000).toISOString().slice(0, 10)

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
    fetch(`${API}/reports/range?from=${from}&to=${to}`)
      .then(async (r) => {
        if (!r.ok) throw new Error((await r.text()).trim())
        return r.json()
      })
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

  function download() {
    window.location.href = `${API}/reports/export?from=${from}&to=${to}`
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
          </div>

          <div className="grid-2">
            <div className="card">
              <h2>Paid by</h2>
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

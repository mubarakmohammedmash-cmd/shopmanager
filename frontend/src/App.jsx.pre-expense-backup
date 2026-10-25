import { useEffect, useState } from 'react'
import './App.css'
import { api, naira, toKobo, when } from './api.js'
import SaleScreen from './Sale.jsx'
import HistoryScreen from './History.jsx'
import ReportsScreen from './Reports.jsx'
import UsersScreen from './Users.jsx'

const ADMIN_TABS = [
  ['dashboard', 'Dashboard'],
  ['sale', 'New sale'],
  ['history', 'History'],
  ['reports', 'Reports'],
  ['products', 'Products'],
  ['expenses', 'Expenses'],
  ['users', 'Staff'],
]
const REP_TABS = [
  ['rep-dashboard', 'Dashboard'],
  ['sale', 'New sale'],
  ['history', 'My sales'],
]

export default function App({ user, onLogout }) {
  const isAdmin = user.role === 'admin'
  const tabs = isAdmin ? ADMIN_TABS : REP_TABS
  const [tab, setTab] = useState(tabs[0][0])
  const [products, setProducts] = useState([])
  const [report, setReport] = useState(null)
  const [expenses, setExpenses] = useState([])
  const [recent, setRecent] = useState([])
  const [message, setMessage] = useState(null)
  const [theme, setTheme] = useState(
    () =>
      localStorage.getItem('theme') ||
      (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'),
  )

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
    localStorage.setItem('theme', theme)
  }, [theme])

  function notify(text, type = 'ok') {
    setMessage({ text, type })
    setTimeout(() => setMessage(null), 3500)
  }

  async function load() {
    try {
      if (isAdmin) {
        const [p, r, e, s] = await Promise.all([
          api('/products'),
          api('/reports/today'),
          api('/expenses'),
          api('/sales/recent'),
        ])
        setProducts(p)
        setReport(r)
        setExpenses(e)
        setRecent(s)
      } else {
        setProducts(await api('/products'))
      }
    } catch (err) {
      notify(
        err instanceof TypeError ? 'Cannot reach the server. Is the Go server still running?' : err.message,
        'error',
      )
    }
  }

  useEffect(() => {
    load()
  }, [])

  return (
    <div className="app">
      <div className="topbar">
        <div className="topbar-inner">
          <div className="brand">ShopManager</div>
          <div className="tabs">
            {tabs.map(([key, label]) => (
              <button
                key={key}
                className={'tab' + (tab === key ? ' active' : '')}
                onClick={() => setTab(key)}
              >
                {label}
              </button>
            ))}
          </div>
          <div style={{ marginLeft: 'auto', display: 'flex', gap: 8, alignItems: 'center', color: '#fff', fontSize: 14 }}>
            <span>{user.username} ({isAdmin ? 'Manager' : 'Sales rep'})</span>
            <button className="theme-btn" style={{ margin: 0 }} onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>
              {theme === 'dark' ? '☀ Light' : '🌙 Dark'}
            </button>
            <button className="theme-btn" style={{ margin: 0 }} onClick={onLogout}>Log out</button>
          </div>
        </div>
      </div>

      <div className="page">
        {tab === 'dashboard' && isAdmin && <Dashboard report={report} recent={recent} />}
        {tab === 'rep-dashboard' && !isAdmin && <RepDashboard />}
        {tab === 'sale' && <SaleScreen products={products} reload={load} notify={notify} isAdmin={isAdmin} />}
        {tab === 'history' && <HistoryScreen notify={notify} reload={load} isAdmin={isAdmin} />}
        {tab === 'reports' && isAdmin && <ReportsScreen />}
        {tab === 'products' && isAdmin && <Products products={products} reload={load} notify={notify} />}
        {tab === 'expenses' && isAdmin && (
          <Expenses expenses={expenses} report={report} reload={load} notify={notify} />
        )}
        {tab === 'users' && isAdmin && <UsersScreen notify={notify} currentUserId={user.id} />}
      </div>

      {message && <div className={'toast ' + message.type}>{message.text}</div>}
    </div>
  )
}

function Stat({ label, value, tone }) {
  return (
    <div className={'stat ' + (tone || '')}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}


function RepDashboard() {
  const [summary, setSummary] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let alive = true
    api('/sales/my-summary')
      .then(data => {
        if (alive) setSummary(data)
      })
      .catch(err => {
        if (alive) setError(err.message || 'Could not load your sales summary')
      })
    return () => { alive = false }
  }, [])

  if (error) {
    return <div className="card">
      <h2>My dashboard</h2>
      <p className="low">{error}</p>
      <button onClick={() => window.location.reload()}>Try again</button>
    </div>
  }

  if (!summary) return <div className="card">Loading your dashboard...</div>

  return (
    <>
      <div className="card">
        <h2>My dashboard</h2>
        <p>Today's figures for your sales account.</p>
      </div>
      <div className="stats">
        <Stat label="Sales revenue today" value={naira(summary.revenue)} />
        <Stat label="Transactions" value={summary.transactions} />
        <Stat label="Items sold" value={summary.items_sold} />
        <Stat label="Cash received" value={naira(summary.cash_received)} />
        <Stat label="Transfers received" value={naira(summary.transfer_received)} />
        <Stat label="POS received" value={naira(summary.pos_received)} />
        <Stat label="Cash change paid" value={naira(summary.cash_change_paid)} />
        <Stat
          label="Net cash drawer movement"
          value={naira(summary.cash_drawer_net)}
          tone={summary.cash_drawer_net < 0 ? 'bad' : 'good'}
        />
      </div>
    </>
  )
}

function Dashboard({ report, recent }) {
  if (!report) return <div className="card">Loading...</div>
  return (
    <>
      <div className="stats">
        <Stat label="Sales today" value={naira(report.revenue)} />
        <Stat label="Gross profit" value={naira(report.profit)} />
        <Stat label="Expenses today" value={naira(report.expenses)} />
        <Stat
          label="Net profit today"
          value={naira(report.net_profit)}
          tone={report.net_profit < 0 ? 'bad' : 'good'}
        />
        <Stat label="Transactions" value={report.sales_count} />
      </div>

      <div className="grid-2">
        <div className="card">
          <h2>Low stock alerts</h2>
          {report.low_stock.length === 0 && <div className="empty">All products are well stocked.</div>}
          {report.low_stock.map((p) => (
            <div key={p.id} className="cart-line">
              <span>{p.name}</span>
              <span className="low">{p.stock} left</span>
            </div>
          ))}
        </div>

        <div className="card">
          <h2>Top sellers (last 30 days)</h2>
          {report.top_products.length === 0 && <div className="empty">No sales yet.</div>}
          {report.top_products.map((t) => (
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
        <h2>Recent sales</h2>
        {recent.length === 0 && <div className="empty">No sales yet.</div>}
        {recent.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr><th>Receipt</th><th>Time</th><th>Items</th><th>Total</th><th>Profit</th></tr>
              </thead>
              <tbody>
                {recent.map((s) => (
                  <tr key={s.id}>
                    <td>#{s.id}</td>
                    <td>{when(s.created_at)}</td>
                    <td>{s.units}</td>
                    <td>{naira(s.total)}</td>
                    <td className="ok">{naira(s.profit)}</td>
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

function Products({ products, reload, notify }) {
  const empty = { name: '', cost: '', price: '', stock: '', low: '5' }
  const [form, setForm] = useState(empty)
  const [search, setSearch] = useState('')
  const [editId, setEditId] = useState(null)
  const [draft, setDraft] = useState({})

  const set = (k) => (e) => setForm({ ...form, [k]: e.target.value })
  const shown = products.filter((p) => p.name.toLowerCase().includes(search.toLowerCase()))

  async function add(e) {
    e.preventDefault()
    try {
      await api('/products', {
        method: 'POST',
        body: JSON.stringify({
          name: form.name,
          cost_price: toKobo(form.cost),
          selling_price: toKobo(form.price),
          stock: Number(form.stock),
          low_stock_level: Number(form.low),
        }),
      })
      setForm(empty)
      notify('Product added')
      reload()
    } catch (err) {
      notify(err.message, 'error')
    }
  }

  async function restock(p) {
    const input = window.prompt(`How many units of ${p.name} did you receive?`)
    const qty = Number(input)
    if (!input || !Number.isInteger(qty) || qty <= 0) return
    try {
      await api(`/products/${p.id}/restock`, { method: 'POST', body: JSON.stringify({ qty }) })
      notify(`Added ${qty} to ${p.name}`)
      reload()
    } catch (err) {
      notify(err.message, 'error')
    }
  }

  function startEdit(p) {
    setEditId(p.id)
    setDraft({ name: p.name, cost: p.cost_price / 100, price: p.selling_price / 100, low: p.low_stock_level })
  }

  async function saveEdit(p) {
    try {
      await api(`/products/${p.id}`, {
        method: 'PUT',
        body: JSON.stringify({
          name: draft.name,
          cost_price: toKobo(draft.cost),
          selling_price: toKobo(draft.price),
          low_stock_level: Number(draft.low),
        }),
      })
      setEditId(null)
      notify('Saved')
      reload()
    } catch (err) {
      notify(err.message, 'error')
    }
  }

  return (
    <>
      <div className="card">
        <h2>Add product</h2>
        <form className="form-grid" onSubmit={add}>
          <label className="field"><span>Name</span>
            <input required value={form.name} onChange={set('name')} placeholder="e.g. Peak Milk 400g" />
          </label>
          <label className="field"><span>Cost price (₦)</span>
            <input required type="number" min="0" step="any" value={form.cost} onChange={set('cost')} />
          </label>
          <label className="field"><span>Selling price (₦)</span>
            <input required type="number" min="0" step="any" value={form.price} onChange={set('price')} />
          </label>
          <label className="field"><span>Opening stock</span>
            <input required type="number" min="0" value={form.stock} onChange={set('stock')} />
          </label>
          <label className="field"><span>Alert when stock is at or below</span>
            <input required type="number" min="1" value={form.low} onChange={set('low')} />
          </label>
          <button type="submit">Save product</button>
        </form>
      </div>

      <div className="card">
        <h2>All products ({products.length})</h2>
        <input
          placeholder="Search products..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ marginBottom: 12 }}
        />
        <div className="table-wrap">
          <table>
            <thead>
              <tr><th>Product</th><th>Cost</th><th>Price</th><th>In stock</th><th>Alert at</th><th></th></tr>
            </thead>
            <tbody>
              {shown.map((p) =>
                editId === p.id ? (
                  <tr key={p.id}>
                    <td><input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} /></td>
                    <td><input type="number" min="0" step="any" value={draft.cost} onChange={(e) => setDraft({ ...draft, cost: e.target.value })} /></td>
                    <td><input type="number" min="0" step="any" value={draft.price} onChange={(e) => setDraft({ ...draft, price: e.target.value })} /></td>
                    <td>{p.stock}</td>
                    <td><input type="number" min="1" value={draft.low} onChange={(e) => setDraft({ ...draft, low: e.target.value })} /></td>
                    <td className="actions">
                      <button onClick={() => saveEdit(p)}>Save</button>
                      <button className="ghost" onClick={() => setEditId(null)}>Cancel</button>
                    </td>
                  </tr>
                ) : (
                  <tr key={p.id}>
                    <td>{p.name}</td>
                    <td>{naira(p.cost_price)}</td>
                    <td>{naira(p.selling_price)}</td>
                    <td className={p.stock <= p.low_stock_level ? 'low' : ''}>{p.stock}</td>
                    <td>{p.low_stock_level}</td>
                    <td className="actions">
                      <button onClick={() => restock(p)}>Restock</button>
                      <button className="ghost" onClick={() => startEdit(p)}>Edit</button>
                    </td>
                  </tr>
                ),
              )}
            </tbody>
          </table>
        </div>
      </div>
    </>
  )
}

function Expenses({ expenses, report, reload, notify }) {
  const [description, setDescription] = useState('')
  const [amount, setAmount] = useState('')
  const [date, setDate] = useState('')

  async function add(e) {
    e.preventDefault()
    try {
      await api('/expenses', {
        method: 'POST',
        body: JSON.stringify({ description, amount: toKobo(amount), date }),
      })
      setDescription('')
      setAmount('')
      setDate('')
      notify('Expense recorded')
      reload()
    } catch (err) {
      notify(err.message, 'error')
    }
  }

  return (
    <>
      {report && (
        <div className="stats">
          <Stat label="Expenses today" value={naira(report.expenses)} />
        </div>
      )}

      <div className="card">
        <h2>Record an expense</h2>
        <form className="form-grid" onSubmit={add}>
          <label className="field"><span>What was it for?</span>
            <input required value={description} onChange={(e) => setDescription(e.target.value)} placeholder="e.g. Generator fuel" />
          </label>
          <label className="field"><span>Amount (₦)</span>
            <input required type="number" min="1" step="any" value={amount} onChange={(e) => setAmount(e.target.value)} />
          </label>
          <label className="field"><span>Date (leave empty for today)</span>
            <input type="date" max={new Date().toISOString().slice(0, 10)} value={date} onChange={(e) => setDate(e.target.value)} />
          </label>
          <button type="submit">Save expense</button>
        </form>
      </div>

      <div className="card">
        <h2>Recent expenses</h2>
        {expenses.length === 0 && <div className="empty">No expenses recorded yet.</div>}
        {expenses.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr><th>When</th><th>Description</th><th>Amount</th></tr>
              </thead>
              <tbody>
                {expenses.map((x) => (
                  <tr key={x.id}>
                    <td>{when(x.created_at)}</td>
                    <td>{x.description}</td>
                    <td>{naira(x.amount)}</td>
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

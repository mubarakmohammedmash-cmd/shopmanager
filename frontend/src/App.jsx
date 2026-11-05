import { useCallback, useEffect, useRef, useState } from 'react'
import './App.css'
import { api, naira, toKobo, when } from './api.js'
import SaleScreen from './Sale.jsx'
import HistoryScreen from './History.jsx'
import ReportsScreen from './Reports.jsx'
import UsersScreen from './Users.jsx'
import StoreScreen from './Store.jsx'

const ADMIN_TABS = [
  ['dashboard', 'Dashboard'],
  ['sale', 'New sale'],
  ['history', 'History'],
  ['reports', 'Reports'],
  ['products', 'Products'],
  ['expenses', 'Expenses'],
  ['store', 'Store'],
  ['users', 'Staff'],
]
const REP_TABS = [
  ['rep-dashboard', 'Dashboard'],
  ['sale', 'New sale'],
  ['history', 'My sales'],
  ['expenses', 'Expenses'],
]

export default function App({ user, onLogout, onStoreNameChange }) {
  const isAdmin = user.role === 'admin'
  const tabs = isAdmin ? ADMIN_TABS : REP_TABS
  const [tab, setTab] = useState(tabs[0][0])
  const [products, setProducts] = useState([])
  const [report, setReport] = useState(null)
  const [expenses, setExpenses] = useState([])
  const [recent, setRecent] = useState([])
  const [outstanding, setOutstanding] = useState([])
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

  const notify = useCallback((text, type = 'ok') => {
    setMessage({ text, type })
    setTimeout(() => setMessage(null), 3500)
  }, [])

  async function load() {
    try {
      if (isAdmin) {
        const [p, r, e, s, o] = await Promise.all([
          api('/products'),
          api('/reports/today'),
          api('/expenses'),
          api('/sales/recent'),
          api('/sales/outstanding'),
        ])
        setProducts(p)
        setReport(r)
        setExpenses(e)
        setRecent(s)
        setOutstanding(o)
      } else {
        const [p, e] = await Promise.all([
          api('/products'),
          api('/expenses'),
        ])
        setProducts(p)
        setExpenses(e)
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
          <div className="brand">{user.store_name || 'ShopManager'}</div>
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
          <div className="account-actions">
            <span>{user.username} ({isAdmin ? 'Manager' : 'Sales rep'})</span>
            <button className="theme-btn" onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>
              {theme === 'dark' ? '☀ Light' : '🌙 Dark'}
            </button>
            <button className="theme-btn" onClick={onLogout}>Log out</button>
          </div>
        </div>
      </div>

      <div className="page">
        {tab === 'dashboard' && isAdmin && <Dashboard report={report} recent={recent} outstanding={outstanding} onOpenHistory={() => setTab('history')} />}
        {tab === 'rep-dashboard' && !isAdmin && <RepDashboard onOpenHistory={() => setTab('history')} />}
        {tab === 'sale' && <SaleScreen products={products} reload={load} notify={notify} isAdmin={isAdmin} />}
        {tab === 'history' && <HistoryScreen notify={notify} reload={load} isAdmin={isAdmin} />}
        {tab === 'reports' && isAdmin && <ReportsScreen />}
        {tab === 'products' && isAdmin && <Products products={products} reload={load} notify={notify} />}
        {tab === 'expenses' && (
          <Expenses expenses={expenses} report={report} reload={load} notify={notify} isAdmin={isAdmin} />
        )}
        {tab === 'store' && isAdmin && <StoreScreen notify={notify} onStoreNameChange={onStoreNameChange} />}
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


function RepDashboard({ onOpenHistory }) {
  const [summary, setSummary] = useState(null)
  const [error, setError] = useState('')
  const [outstanding, setOutstanding] = useState([])

  useEffect(() => {
    let alive = true
    api('/sales/my-summary')
      .then(data => {
        if (alive) setSummary(data)
      })
      .catch(err => {
        if (alive) setError(err.message || 'Could not load your sales summary')
      })
    api('/sales/outstanding')
      .then(data => {
        if (alive) setOutstanding(data)
      })
      .catch(err => {
        if (alive) setError(err.message || 'Could not load outstanding balances')
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
      <OutstandingList sales={outstanding} onOpenHistory={onOpenHistory} />
    </>
  )
}

function Dashboard({ report, recent, outstanding, onOpenHistory }) {
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

      <OutstandingList sales={outstanding} onOpenHistory={onOpenHistory} />

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

function OutstandingList({ sales, onOpenHistory }) {
  const totalOwed = sales.reduce((sum, sale) => sum + sale.balance, 0)

  return (
    <div className="card">
      <h2>Outstanding balances</h2>
      {sales.length === 0 ? (
        <div className="empty">No unpaid sales.</div>
      ) : (
        <>
          <div className="cart-line">
            <span>{sales.length} unpaid sale{sales.length === 1 ? '' : 's'}</span>
            <strong className="low">{naira(totalOwed)} owed</strong>
          </div>
          {sales.slice(0, 5).map((sale) => (
            <div className="cart-line" key={sale.id}>
              <span>{sale.customer_name} · #{sale.id}</span>
              <strong className="low">{naira(sale.balance)}</strong>
            </div>
          ))}
          <button className="ghost" style={{ width: '100%', marginTop: 12 }} onClick={onOpenHistory}>
            Manage payments in History
          </button>
        </>
      )}
    </div>
  )
}

function Products({ products, reload, notify }) {
  const empty = { name: '', cost: '', price: '', stock: '', low: '5' }
  const [form, setForm] = useState(empty)
  const [search, setSearch] = useState('')
  const [editId, setEditId] = useState(null)
  const [draft, setDraft] = useState({})
  const [voiceListening, setVoiceListening] = useState(false)
  const [voiceTranscript, setVoiceTranscript] = useState('')
  const recognitionRef = useRef(null)

  const set = (k) => (e) => setForm({ ...form, [k]: e.target.value })
  const shown = products.filter((p) => p.name.toLowerCase().includes(search.toLowerCase()))

  useEffect(() => () => {
    if (recognitionRef.current) {
      recognitionRef.current.onend = null
      recognitionRef.current.stop()
    }
  }, [])

  function startVoiceEntry() {
    const SpeechRecognition =
      window.SpeechRecognition || window.webkitSpeechRecognition
    if (!SpeechRecognition) {
      notify('Voice entry is not supported in this browser. Try Chrome or Edge.', 'error')
      return
    }

    const recognition = new SpeechRecognition()
    recognition.lang = 'en-NG'
    recognition.interimResults = false
    recognition.maxAlternatives = 1
    recognitionRef.current = recognition
    setVoiceTranscript('')

    recognition.onresult = (event) => {
      const transcript = event.results[0][0].transcript.trim()
      const findNumber = (pattern) => {
        const match = transcript.match(new RegExp(
          `\\b(?:${pattern})\\s*(?:(?:is|of|at)\\s*)?(?:₦\\s*)?(\\d[\\d,]*(?:\\.\\d+)?)`,
          'i',
        ))
        return match ? match[1].replace(/,/g, '') : ''
      }

      const firstField = transcript.search(
        /\b(?:cost(?:\s+price)?|buying\s+price|selling\s+price|sell\s+price|price|stock|quantity|qty|alert|low\s+stock)\b/i,
      )
      const spokenName = (firstField === -1 ? transcript : transcript.slice(0, firstField))
        .replace(/^\s*(?:product\s+name|name)\s*/i, '')
        .replace(/[\s,:-]+$/, '')

      setVoiceTranscript(transcript)
      setForm((current) => ({
        ...current,
        name: spokenName || current.name,
        cost: findNumber('cost(?:\\s+price)?|buying\\s+price') || current.cost,
        price: findNumber('selling\\s+price|sell\\s+price') || current.price,
        stock: findNumber('stock|quantity|qty') || current.stock,
        low: findNumber('alert(?:\\s+at)?|low\\s+stock(?:\\s+level)?') || current.low,
      }))
      notify('Voice entry filled what it recognized. Review the fields before saving.')
    }

    recognition.onerror = (event) => {
      const message = event.error === 'not-allowed'
        ? 'Microphone access was denied. Allow microphone access and try again.'
        : event.error === 'no-speech'
          ? 'No speech was detected. Try again and speak clearly.'
          : `Voice entry failed: ${event.error}`
      notify(message, 'error')
    }

    recognition.onend = () => {
      setVoiceListening(false)
      recognitionRef.current = null
    }

    try {
      setVoiceListening(true)
      recognition.start()
    } catch (error) {
      setVoiceListening(false)
      recognitionRef.current = null
      notify(`Could not start voice entry: ${error.message}`, 'error')
    }
  }

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
        <p className="muted">
          Speak a product name, cost, selling price, stock, and optional alert level.
          For example: “Product name Peak Milk, cost 200, selling price 300, stock 12, alert 5”.
          Voice recognition is provided by your browser. Check the fields before saving.
        </p>
        <div className="voice-entry">
          <button type="button" className="ghost" onClick={startVoiceEntry} disabled={voiceListening}>
            {voiceListening ? 'Listening...' : 'Use voice'}
          </button>
          {voiceListening && (
            <button type="button" className="ghost" onClick={() => recognitionRef.current?.stop()}>
              Stop listening
            </button>
          )}
        </div>
        {voiceTranscript && (
          <p className="muted" aria-live="polite">Heard: “{voiceTranscript}”</p>
        )}
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

function Expenses({ expenses, report, reload, notify, isAdmin }) {
  const [description, setDescription] = useState('')
  const [amount, setAmount] = useState('')
  const [date, setDate] = useState('')
  const [paymentSource, setPaymentSource] = useState('company')

  async function add(e) {
    e.preventDefault()
    try {
      await api('/expenses', {
        method: 'POST',
        body: JSON.stringify({
          description,
          amount: toKobo(amount),
          date,
          payment_source: paymentSource,
        }),
      })
      setDescription('')
      setAmount('')
      setDate('')
      setPaymentSource('company')
      notify(isAdmin ? 'Expense recorded' : 'Expense submitted for manager review')
      reload()
    } catch (err) {
      notify(err.message, 'error')
    }
  }

  async function review(x, status) {
    try {
      await api(`/expenses/${x.id}/review`, {
        method: 'POST',
        body: JSON.stringify({ status, note: '' }),
      })
      notify(`Expense ${status}`)
      reload()
    } catch (err) {
      notify(err.message, 'error')
    }
  }

  async function reimburse(x) {
    try {
      await api(`/expenses/${x.id}/reimburse`, { method: 'POST' })
      notify('Expense marked as reimbursed')
      reload()
    } catch (err) {
      notify(err.message, 'error')
    }
  }

  return (
    <>
      {report && isAdmin && (
        <div className="stats">
          <Stat label="Approved expenses today" value={naira(report.expenses)} />
        </div>
      )}

      <div className="card">
        <h2>{isAdmin ? 'Record an expense' : 'Submit an expense'}</h2>
        <form className="form-grid" onSubmit={add}>
          <label className="field"><span>What was it for?</span>
            <input required value={description} onChange={(e) => setDescription(e.target.value)} placeholder="e.g. Generator fuel" />
          </label>
          <label className="field"><span>Amount (₦)</span>
            <input required type="number" min="1" step="any" value={amount} onChange={(e) => setAmount(e.target.value)} />
          </label>
          <label className="field"><span>Payment source</span>
            <select value={paymentSource} onChange={(e) => setPaymentSource(e.target.value)}>
              <option value="company">Company funds</option>
              <option value="personal">Personal pocket (reimbursable)</option>
            </select>
          </label>
          <label className="field"><span>Date (leave empty for today)</span>
            <input type="date" max={new Date().toISOString().slice(0, 10)} value={date} onChange={(e) => setDate(e.target.value)} />
          </label>
          <button type="submit">{isAdmin ? 'Save expense' : 'Submit for review'}</button>
        </form>
      </div>

      <div className="card">
        <h2>{isAdmin ? 'All expenses' : 'My submitted expenses'}</h2>
        {expenses.length === 0 && <div className="empty">No expenses recorded yet.</div>}
        {expenses.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>When</th>
                  <th>Description</th>
                  <th>Amount</th>
                  {isAdmin && <th>Submitted by</th>}
                  <th>Source</th>
                  <th>Status</th>
                  {isAdmin && <th>Actions</th>}
                </tr>
              </thead>
              <tbody>
                {expenses.map((x) => (
                  <tr key={x.id}>
                    <td>{when(x.created_at)}</td>
                    <td>{x.description}</td>
                    <td>{naira(x.amount)}</td>
                    {isAdmin && <td>{x.submitted_by_name || 'Manager'}</td>}
                    <td>{x.payment_source === 'personal' ? 'Personal' : 'Company'}</td>
                    <td>
                      <span className={x.status === 'approved' ? 'ok' : x.status === 'rejected' ? 'low' : 'muted'}>
                        {x.status === 'approved' ? 'Approved' : x.status === 'rejected' ? 'Rejected' : 'Pending review'}
                      </span>
                      {x.payment_source === 'personal' && x.status === 'approved' && (
                        <div style={{ fontSize: 12 }} className={x.reimbursement_status === 'paid' ? 'ok' : 'low'}>
                          {x.reimbursement_status === 'paid' ? 'Reimbursed' : 'Unpaid reimbursement'}
                        </div>
                      )}
                    </td>
                    {isAdmin && (
                      <td className="actions">
                        {x.status === 'pending' && (
                          <>
                            <button onClick={() => review(x, 'approved')}>Approve</button>
                            <button className="ghost danger" onClick={() => review(x, 'rejected')}>Reject</button>
                          </>
                        )}
                        {x.status === 'approved' && x.payment_source === 'personal' && x.reimbursement_status === 'unpaid' && (
                          <button onClick={() => reimburse(x)}>Reimburse</button>
                        )}
                      </td>
                    )}
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

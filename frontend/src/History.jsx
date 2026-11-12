import { useEffect, useMemo, useRef, useState } from 'react'
import { api, naira, toKobo, when } from './api.js'

const paymentLabels = {
  cash: 'Cash',
  transfer: 'Transfer',
  pos: 'POS',
}

function paymentMethodLabel(methods) {
  return String(methods || '')
    .split(',')
    .filter(Boolean)
    .map((method) => paymentLabels[method] || method)
    .join(', ') || '-'
}

function localDate(date = new Date()) {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function downloadHistoryCSV(sales) {
  const columns = [
    'Receipt',
    'Date',
    'Customer',
    'Items',
    'Payment method',
    'Status',
    'Sold by',
    'Total',
    'Paid',
    'Balance',
    'Profit',
    'Voided by',
  ]

  const escapeCSV = (value) => {
    let text = String(value ?? '')

    // Protect spreadsheet users from formula injection.
    if (/^[\s]*[=+\-@]/.test(text)) {
      text = "'" + text
    }

    return `"${text.replace(/"/g, '""')}"`
  }

  const rows = sales.map((sale) => [
    sale.id,
    sale.created_at,
    sale.customer_name,
    sale.items,
    paymentMethodLabel(sale.payment_method),
    sale.status,
    sale.sold_by,
    sale.total,
    sale.paid,
    sale.balance,
    sale.profit ?? '',
    sale.voided_by || '',
  ])

  const csv = [
    columns.map(escapeCSV).join(','),
    ...rows.map((row) => row.map(escapeCSV).join(',')),
  ].join('\r\n')

  const blob = new Blob(['\uFEFF', csv], {
    type: 'text/csv;charset=utf-8;',
  })

  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `shopmanager-history-${localDate()}.csv`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

export default function HistoryScreen({ notify, reload, isAdmin }) {
  const [sales, setSales] = useState(null)
  const [search, setSearch] = useState('')
  const [payment, setPayment] = useState('all')
  const [status, setStatus] = useState('all')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [selectedSale, setSelectedSale] = useState(null)
  const [payments, setPayments] = useState([])
  const [paymentAmount, setPaymentAmount] = useState('')
  const [paymentMethod, setPaymentMethod] = useState('cash')
  const [savingPayment, setSavingPayment] = useState(false)
  const [focusPaymentForm, setFocusPaymentForm] = useState(false)
  const paymentFormRef = useRef(null)
  const paymentAmountInputRef = useRef(null)
  const saleDetailsRef = useRef(null)

  async function loadHistory() {
    try {
      const result = await api('/sales/history')
      const rows = Array.isArray(result) ? result : []
      setSales(rows)
      return rows
    } catch (error) {
      setSales([])
      notify(error.message || 'Could not load transaction history', 'error')
      return []
    }
  }

  async function openSale(sale, focusPayment = false) {
    setSelectedSale(sale)
    setFocusPaymentForm(focusPayment)
    setPayments([])
    setPaymentAmount('')
    try {
      const rows = await api(`/sales/${sale.id}/payments`)
      setPayments(Array.isArray(rows) ? rows : [])
    } catch (error) {
      notify(error.message || 'Could not load payment history', 'error')
    }
  }

  useEffect(() => {
    if (!selectedSale) return

    saleDetailsRef.current?.scrollIntoView({
      behavior: 'smooth',
      block: 'start',
    })
  }, [selectedSale])

  useEffect(() => {
    if (!focusPaymentForm || !selectedSale) return

    paymentFormRef.current?.scrollIntoView({
      behavior: 'smooth',
      block: 'center',
    })
    paymentAmountInputRef.current?.focus({ preventScroll: true })
    setFocusPaymentForm(false)
  }, [focusPaymentForm, selectedSale])

  async function recordPayment(event) {
    event.preventDefault()
    const amount = toKobo(paymentAmount)
    if (!Number.isSafeInteger(amount) || amount <= 0) {
      notify('Enter a valid payment amount', 'error')
      return
    }
    setSavingPayment(true)
    let recorded = false
    try {
      await api(`/sales/${selectedSale.id}/payments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ amount, payment_method: paymentMethod }),
      })
      recorded = true
      notify('Payment recorded')
      setPaymentAmount('')
      const [rows, paymentRows] = await Promise.all([
        loadHistory(),
        api(`/sales/${selectedSale.id}/payments`),
      ])
      setSelectedSale(rows.find((sale) => sale.id === selectedSale.id) || null)
      setPayments(Array.isArray(paymentRows) ? paymentRows : [])
      reload()
    } catch (error) {
      notify(
        recorded
          ? `Payment recorded, but the updated payment history could not be loaded: ${error.message}`
          : error.message || 'Could not record payment',
        'error',
      )
    } finally {
      setSavingPayment(false)
    }
  }

  useEffect(() => {
    loadHistory()
  }, [])

  const filteredSales = useMemo(() => {
    if (!sales) return []

    const term = search.trim().toLowerCase()

    return sales.filter((sale) => {
      const matchesSearch =
        !term ||
        String(sale.id).includes(term) ||
        String(sale.items || '').toLowerCase().includes(term) ||
        String(sale.customer_name || '').toLowerCase().includes(term) ||
        String(sale.sold_by || '').toLowerCase().includes(term) ||
        String(sale.voided_by || '').toLowerCase().includes(term)

      const saleDate = String(sale.created_at || '').slice(0, 10)

      const matchesFrom = !from || saleDate >= from
      const matchesTo = !to || saleDate <= to

      const matchesPayment =
        payment === 'all' || String(sale.payment_method || '').split(',').includes(payment)

      const matchesStatus =
        status === 'all' ||
        (status === 'outstanding'
          ? sale.status === 'completed' && sale.balance > 0
          : sale.status === status)

      return (
        matchesSearch &&
        matchesFrom &&
        matchesTo &&
        matchesPayment &&
        matchesStatus
      )
    })
  }, [sales, search, payment, status, from, to])

  const completedSales = filteredSales.filter(
    (sale) => sale.status === 'completed',
  )

  const totalRevenue = completedSales.reduce(
    (sum, sale) => sum + Number(sale.total || 0),
    0,
  )

  const resetFilters = () => {
    setSearch('')
    setPayment('all')
    setStatus('all')
    setFrom('')
    setTo('')
  }

  async function voidSale(sale) {
    if (sale.status !== 'completed') {
      notify('Only completed sales can be voided.', 'error')
      return
    }

    const confirmed = window.confirm(
      `Void receipt #${sale.id} (${naira(sale.total)})? The items will be returned to stock.`,
    )

    if (!confirmed) return

    try {
      await api(`/sales/${sale.id}/void`, { method: 'POST' })
      notify('Sale voided and stock restored')
      setSelectedSale(null)
      await loadHistory()
      reload()
    } catch (error) {
      notify(error.message || 'Could not void sale', 'error')
    }
  }

  return (
    <>
      <div className="card">
        <h2>{isAdmin ? 'Transaction history' : 'My sales'}</h2>

        <div className="stats">
          <div className="stat">
            <span>Matching transactions</span>
            <strong>{filteredSales.length}</strong>
          </div>

          <div className="stat">
            <span>Completed sales</span>
            <strong>{completedSales.length}</strong>
          </div>

          <div className="stat">
            <span>Completed sales total</span>
            <strong>{naira(totalRevenue)}</strong>
          </div>
        </div>

        <div className="form-grid">
          <label className="field">
            <span>Search</span>
            <input
              type="search"
              placeholder="Receipt, product or staff"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </label>

          <label className="field">
            <span>From date</span>
            <input
              type="date"
              value={from}
              max={to || undefined}
              onChange={(event) => setFrom(event.target.value)}
            />
          </label>

          <label className="field">
            <span>To date</span>
            <input
              type="date"
              value={to}
              min={from || undefined}
              onChange={(event) => setTo(event.target.value)}
            />
          </label>

          <label className="field">
            <span>Payment method</span>
            <select
              value={payment}
              onChange={(event) => setPayment(event.target.value)}
            >
              <option value="all">All methods</option>
              <option value="cash">Cash</option>
              <option value="transfer">Transfer</option>
              <option value="pos">POS</option>
            </select>
          </label>

          <label className="field">
            <span>Transaction status</span>
            <select
              value={status}
              onChange={(event) => setStatus(event.target.value)}
            >
              <option value="all">All statuses</option>
              <option value="outstanding">Outstanding balance</option>
              <option value="completed">Completed</option>
              <option value="voided">Voided</option>
            </select>
          </label>

          <button className="ghost" onClick={resetFilters}>
            Clear filters
          </button>

          <button
            onClick={() => downloadHistoryCSV(filteredSales)}
            disabled={filteredSales.length === 0}
          >
            Export CSV
          </button>
        </div>
      </div>

      {sales === null && (
        <div className="card empty">Loading transactions...</div>
      )}

      {sales && filteredSales.length === 0 && (
        <div className="card empty">
          No transactions match your search and filters.
        </div>
      )}

      {sales && filteredSales.length > 0 && (
        <div className="card">
          <div className="table-wrap history-table-wrap">
            <table className="history-table">
              <thead>
                <tr>
                  <th>Receipt</th>
                  <th>Time</th>
                  <th>Customer</th>
                  <th>Items sold</th>
                  <th>Payment methods</th>
                  {isAdmin && <th>Sold by</th>}
                  <th>Total</th>
                  <th>Paid</th>
                  <th>Balance</th>
                  {isAdmin && <th>Profit</th>}
                  <th>Payment status</th>
                  <th>Sale status</th>
                  <th>Actions</th>
                </tr>
              </thead>

              <tbody>
                {filteredSales.map((sale) => {
                  const voided = sale.status === 'voided'

                  return (
                    <tr
                      key={sale.id}
                      style={voided ? { opacity: 0.7 } : undefined}
                    >
                      <td className="history-receipt" data-label="Receipt">#{sale.id}</td>
                      <td data-label="Time">{when(sale.created_at)}</td>
                      <td data-label="Customer">{sale.customer_name || '-'}</td>
                      <td data-label="Items sold">{sale.items || '-'}</td>
                      <td data-label="Payment methods">
                        {paymentMethodLabel(sale.payment_method)}
                      </td>

                      {isAdmin && <td data-label="Sold by">{sale.sold_by || '-'}</td>}

                      <td data-label="Total">{naira(sale.total)}</td>
                      <td data-label="Paid">{naira(sale.paid)}</td>
                      <td data-label="Balance" className={sale.status === 'voided' ? 'muted' : sale.balance > 0 ? 'low' : 'ok'}>
                        {sale.status === 'voided' ? '-' : sale.balance > 0 ? naira(sale.balance) : 'Paid'}
                      </td>

                      {isAdmin && (
                        <td data-label="Profit" className={voided ? '' : 'ok'}>
                          {voided || sale.profit == null
                            ? '-'
                            : naira(sale.profit)}
                        </td>
                      )}

                      <td data-label="Payment status">
                        {voided ? (
                          <span className="muted">-</span>
                        ) : sale.balance > 0 ? (
                          <span className="low">Balance due</span>
                        ) : (
                          <span className="ok">Paid</span>
                        )}
                      </td>

                      <td data-label="Sale status">
                        {voided ? (
                          <span className="low">Voided</span>
                        ) : (
                          <span className="ok">Completed</span>
                        )}
                      </td>

                      <td className="history-actions-cell" data-label="Actions">
                        <div className="history-actions">
                          <button
                            className="ghost"
                            onClick={() => openSale(sale)}
                          >
                            Details
                          </button>

                          {sale.status === 'completed' && sale.balance > 0 && (
                            <button
                              className="ghost"
                              onClick={() => openSale(sale, true)}
                            >
                              Record payment
                            </button>
                          )}

                          {!voided && (
                            <button
                              className="ghost"
                              onClick={() => voidSale(sale)}
                            >
                              Void
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {selectedSale && (
        <div className="card" ref={saleDetailsRef}>
          <h2>Receipt #{selectedSale.id}</h2>

          <div className="cart-line">
            <span>Date and time</span>
            <strong>{when(selectedSale.created_at)}</strong>
          </div>

          <div className="cart-line">
            <span>Items</span>
            <strong>{selectedSale.items || '-'}</strong>
          </div>

          {selectedSale.customer_name && (
            <div className="cart-line">
              <span>Customer</span>
              <strong>{selectedSale.customer_name}</strong>
            </div>
          )}

          <div className="cart-line">
            <span>Payment methods</span>
            <strong>{paymentMethodLabel(selectedSale.payment_method)}</strong>
          </div>

          <div className="cart-line">
            <span>Status</span>
            <strong>{selectedSale.status}</strong>
          </div>

          <div className="cart-line">
            <span>Total</span>
            <strong>{naira(selectedSale.total)}</strong>
          </div>

          <div className="cart-line">
            <span>Paid so far</span>
            <strong>{naira(selectedSale.paid)}</strong>
          </div>

          <div className="cart-line">
            <span>Balance</span>
            <strong className={selectedSale.balance > 0 ? 'low' : 'ok'}>
              {selectedSale.status === 'voided' ? 'Cancelled' : naira(selectedSale.balance)}
            </strong>
          </div>

          <h3>Payment history</h3>
          {payments.length === 0 && <div className="empty">No payments recorded.</div>}
          {payments.map((item) => (
            <div className="cart-line" key={item.id}>
              <span>
                {paymentLabels[item.payment_method] || item.payment_method}
                {' · '}{when(item.created_at)}
                {item.recorded_by && ` · ${item.recorded_by}`}
              </span>
              <strong>
                {naira(item.amount)}
                {item.change_given > 0 && ` (change ${naira(item.change_given)})`}
              </strong>
            </div>
          ))}

          {selectedSale.status === 'completed' && selectedSale.balance > 0 && (
            <form
              ref={paymentFormRef}
              className="form-grid payment-form"
              onSubmit={recordPayment}
            >
              <label className="field">
                <span>Payment received (₦)</span>
                <input
                  ref={paymentAmountInputRef}
                  required
                  type="number"
                  min="0.01"
                  max={(selectedSale.balance / 100).toFixed(2)}
                  step="0.01"
                  value={paymentAmount}
                  onChange={(event) => setPaymentAmount(event.target.value)}
                />
              </label>
              <label className="field">
                <span>Payment method</span>
                <select
                  value={paymentMethod}
                  onChange={(event) => setPaymentMethod(event.target.value)}
                >
                  {Object.entries(paymentLabels).map(([key, label]) => (
                    <option key={key} value={key}>{label}</option>
                  ))}
                </select>
              </label>
              <button type="submit" disabled={savingPayment}>
                {savingPayment ? 'Saving...' : 'Record payment'}
              </button>
            </form>
          )}

          {isAdmin && (
            <>
              <div className="cart-line">
                <span>Sold by</span>
                <strong>{selectedSale.sold_by || '-'}</strong>
              </div>

              <div className="cart-line">
                <span>Gross profit</span>
                <strong>
                  {selectedSale.profit == null || selectedSale.status === 'voided'
                    ? '-'
                    : naira(selectedSale.profit)}
                </strong>
              </div>
            </>
          )}

          {selectedSale.status === 'voided' && isAdmin && (
            <div className="cart-line">
              <span>Voided by</span>
              <strong>{selectedSale.voided_by || '-'}</strong>
            </div>
          )}

          <button className="ghost" onClick={() => setSelectedSale(null)}>
            Close details
          </button>
        </div>
      )}
    </>
  )
}

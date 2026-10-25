import { useEffect, useMemo, useState } from 'react'
import { api, naira, when } from './api.js'

const paymentLabels = {
  cash: 'Cash',
  transfer: 'Transfer',
  pos: 'POS',
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
    'Items',
    'Payment method',
    'Status',
    'Sold by',
    'Total',
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
    sale.items,
    paymentLabels[sale.payment_method] || sale.payment_method,
    sale.status,
    sale.sold_by,
    sale.total,
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

  async function loadHistory() {
    try {
      const result = await api('/sales/history')
      setSales(Array.isArray(result) ? result : [])
    } catch (error) {
      setSales([])
      notify(error.message || 'Could not load transaction history', 'error')
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
        String(sale.sold_by || '').toLowerCase().includes(term) ||
        String(sale.voided_by || '').toLowerCase().includes(term)

      const saleDate = String(sale.created_at || '').slice(0, 10)

      const matchesFrom = !from || saleDate >= from
      const matchesTo = !to || saleDate <= to

      const matchesPayment =
        payment === 'all' || sale.payment_method === payment

      const matchesStatus =
        status === 'all' || sale.status === status

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
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Receipt</th>
                  <th>Time</th>
                  <th>Items sold</th>
                  <th>Payment</th>
                  {isAdmin && <th>Sold by</th>}
                  <th>Total</th>
                  {isAdmin && <th>Profit</th>}
                  <th>Status</th>
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
                      <td>#{sale.id}</td>
                      <td>{when(sale.created_at)}</td>
                      <td>{sale.items || '-'}</td>
                      <td>
                        {paymentLabels[sale.payment_method] ||
                          sale.payment_method ||
                          '-'}
                      </td>

                      {isAdmin && <td>{sale.sold_by || '-'}</td>}

                      <td>{naira(sale.total)}</td>

                      {isAdmin && (
                        <td className={voided ? '' : 'ok'}>
                          {voided || sale.profit == null
                            ? '-'
                            : naira(sale.profit)}
                        </td>
                      )}

                      <td>
                        {voided ? (
                          <span className="low">Voided</span>
                        ) : (
                          <span className="ok">Completed</span>
                        )}
                      </td>

                      <td>
                        <div className="form-grid">
                          <button
                            className="ghost"
                            onClick={() => setSelectedSale(sale)}
                          >
                            Details
                          </button>

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
        <div className="card">
          <h2>Receipt #{selectedSale.id}</h2>

          <div className="cart-line">
            <span>Date and time</span>
            <strong>{when(selectedSale.created_at)}</strong>
          </div>

          <div className="cart-line">
            <span>Items</span>
            <strong>{selectedSale.items || '-'}</strong>
          </div>

          <div className="cart-line">
            <span>Payment method</span>
            <strong>
              {paymentLabels[selectedSale.payment_method] ||
                selectedSale.payment_method ||
                '-'}
            </strong>
          </div>

          <div className="cart-line">
            <span>Status</span>
            <strong>{selectedSale.status}</strong>
          </div>

          <div className="cart-line">
            <span>Total</span>
            <strong>{naira(selectedSale.total)}</strong>
          </div>

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

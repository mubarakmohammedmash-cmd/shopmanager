import { useState } from 'react'
import { api, naira, toKobo } from './api.js'

// "peak milk : 250" -> { name: "peak milk", price: 250 }
function parseEntry(text) {
  const i = text.lastIndexOf(':')
  if (i === -1) return { name: text.trim(), price: null }
  const priceText = text.slice(i + 1).trim()
  return { name: text.slice(0, i).trim(), price: priceText === '' ? null : Number(priceText) }
}

const METHODS = [
  ['cash', 'Cash'],
  ['transfer', 'Transfer'],
  ['pos', 'POS'],
]

export default function SaleScreen({ products, reload, notify, isAdmin }) {
  const [entry, setEntry] = useState('')
  const [cart, setCart] = useState([])
  const [tenders, setTenders] = useState({ cash: '', transfer: '', pos: '' })
  const [customerName, setCustomerName] = useState('')

  const parsed = parseEntry(entry)
  const shown = products.filter((p) => p.name.toLowerCase().includes(parsed.name.toLowerCase()))
  const total = cart.reduce((sum, c) => sum + toKobo(c.price || 0) * c.qty, 0)
  const hasTenderInput = Object.values(tenders).some((amount) => amount.trim() !== '')
  const paymentAmounts = Object.fromEntries(
    METHODS.map(([method]) => [
      method,
      tenders[method].trim() === '' ? 0 : toKobo(tenders[method]),
    ]),
  )
  const amountReceived = hasTenderInput
    ? Object.values(paymentAmounts).reduce((sum, amount) => sum + amount, 0)
    : total
  const changeDue = Math.max(0, amountReceived - total)
  const shortBy = Math.max(0, total - amountReceived)

  function add(p, price) {
    const inCart = cart.find((c) => c.id === p.id)
    const have = inCart ? inCart.qty : 0
    if (have + 1 > p.stock) {
      notify(`Only ${p.stock} of ${p.name} in stock`, 'error')
      return
    }
    const priceStr = price != null ? String(price) : null
    if (inCart) {
      setCart(
        cart.map((c) =>
          c.id === p.id ? { ...c, qty: c.qty + 1, price: priceStr ?? c.price } : c,
        ),
      )
    } else {
      setCart([
        ...cart,
        {
          id: p.id,
          name: p.name,
          price: priceStr ?? String(p.selling_price / 100),
          cost: p.cost_price,
          stock: p.stock,
          qty: 1,
        },
      ])
    }
  }

  function onEnter(e) {
    if (e.key !== 'Enter') return

    const entries = entry.split(',').map((item) => item.trim()).filter(Boolean)

    if (entries.length === 0) return

    const additions = []
    const errors = []

    for (const text of entries) {
      const item = parseEntry(text)
      const name = item.name.toLowerCase()
      const price = isAdmin ? item.price : null

      if (!name) continue

      if (price !== null && (!Number.isFinite(price) || price < 0)) {
        errors.push(`Invalid price for "${item.name}"`)
        continue
      }

      const matches = products.filter((p) =>
        p.name.toLowerCase().includes(name)
      )

      const product =
        matches.find((p) => p.name.toLowerCase() === name) ||
        (matches.length === 1 ? matches[0] : null)

      if (!product) {
        errors.push(
          matches.length > 1
            ? `"${item.name}" matches multiple products; enter its full name`
            : `Product "${item.name}" not found`
        )
        continue
      }

      additions.push({ product, price })
    }

    if (additions.length > 0) {
      setCart((previousCart) => {
        const nextCart = previousCart.map((c) => ({ ...c }))

        for (const { product, price } of additions) {
          const index = nextCart.findIndex((c) => c.id === product.id)
          const currentQty = index >= 0 ? nextCart[index].qty : 0

          if (currentQty + 1 > product.stock) {
            errors.push(
              `Only ${product.stock} of ${product.name} in stock`
            )
            continue
          }

          if (index >= 0) {
            nextCart[index] = {
              ...nextCart[index],
              qty: nextCart[index].qty + 1,
              price: price !== null ? String(price) : nextCart[index].price,
            }
          } else {
            nextCart.push({
              id: product.id,
              name: product.name,
              price: price !== null
                ? String(price)
                : String(product.selling_price / 100),
              cost: product.cost_price,
              stock: product.stock,
              qty: 1,
            })
          }
        }

        return nextCart
      })
    }

    setEntry('')

    if (errors.length > 0) {
      notify(errors.join('. '), 'error')
    }
  }

  function setPrice(id, value) {
    setCart(cart.map((c) => (c.id === id ? { ...c, price: value } : c)))
  }

  function change(id, delta) {
    const updatedCart = cart.flatMap((c) => {
      if (c.id !== id) return [c]
      const qty = c.qty + delta
      if (qty > c.stock) return [c]
      return qty > 0 ? [{ ...c, qty }] : []
    })
    setCart(updatedCart)
    if (updatedCart.length === 0) {
      setTenders({ cash: '', transfer: '', pos: '' })
      setCustomerName('')
    }
  }

  function clearSale() {
    setCart([])
    setTenders({ cash: '', transfer: '', pos: '' })
    setCustomerName('')
  }

  async function complete() {
    if (
      !Number.isSafeInteger(amountReceived) ||
      amountReceived < 0 ||
      Object.values(paymentAmounts).some((amount) => !Number.isSafeInteger(amount) || amount < 0)
    ) {
      notify('Enter valid payment amounts', 'error')
      return
    }
    if (changeDue > paymentAmounts.cash) {
      notify('Cash received must cover any change given', 'error')
      return
    }
    if (shortBy > 0 && !customerName.trim()) {
      notify('Enter the customer name to record an unpaid balance', 'error')
      return
    }
    for (const c of cart) {
      if (c.price === '' || Number.isNaN(Number(c.price)) || Number(c.price) < 0) {
        notify(`Enter a valid price for ${c.name}`, 'error')
        return
      }
    }
    try {
      const r = await api('/sales', {
        method: 'POST',
        body: JSON.stringify({
          payments: hasTenderInput
            ? METHODS
              .filter(([method]) => paymentAmounts[method] > 0)
              .map(([payment_method]) => ({
                payment_method,
                amount: paymentAmounts[payment_method],
              }))
            : [{ payment_method: 'cash', amount: total }],
          customer_name: customerName.trim(),
          items: cart.map((c) => ({
            product_id: c.id,
            qty: c.qty,
            price: isAdmin ? toKobo(c.price) : undefined,
          })),
        }),
      })
      setCart([])
      setTenders({ cash: '', transfer: '', pos: '' })
      setCustomerName('')
      notify(r.balance > 0
        ? `Sale recorded for ${customerName.trim()}: ${naira(r.balance)} still owed`
        : `Sale complete: ${naira(r.total)}. Change given: ${naira(r.change_given || 0)}`)
      reload()
    } catch (e) {
      notify(e.message, 'error')
    }
  }

  return (
    <div className="pos">
      <div className="card">
        <h2>Add item</h2>
        <input
          autoFocus
          placeholder={
            isAdmin
              ? 'Product, or product : price  (e.g. peak milk : 250), then Enter'
              : 'Type a product name, then press Enter'
          }
          value={entry}
          onChange={(e) => setEntry(e.target.value)}
          onKeyDown={onEnter}
        />
        <div className="muted" style={{ marginTop: 6 }}>
          Enter adds the first match at its saved price.
        </div>
        {products.length === 0 && <div className="empty">No products yet. The manager adds them in the Products tab.</div>}
        <div className="tiles">
          {shown.map((p) => (
            <button key={p.id} className="tile" disabled={p.stock === 0} onClick={() => add(p, null)}>
              <strong>{p.name}</strong>
              <span>{naira(p.selling_price)}</span>
              <span className={p.stock <= p.low_stock_level ? 'low' : ''}>
                {p.stock === 0 ? 'Out of stock' : `${p.stock} in stock`}
              </span>
            </button>
          ))}
        </div>
      </div>

      <div className="card">
        <h2>Current sale</h2>
        {cart.length === 0 && <div className="empty">Type a product and press Enter.</div>}
        {cart.map((c) => (
          <div key={c.id} className="cart-line" style={{ flexWrap: 'wrap' }}>
            <div className="name">
              {c.name}
              {isAdmin && toKobo(c.price || 0) < c.cost && <div className="low">Below cost price</div>}
            </div>
            <div className="qty">
              <button onClick={() => change(c.id, -1)}>-</button>
              <strong>{c.qty}</strong>
              <button onClick={() => change(c.id, 1)}>+</button>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, width: '100%' }}>
              {isAdmin ? (
                <>
                  <span className="muted">Price ₦</span>
                  <input
                    type="number"
                    min="0"
                    step="any"
                    value={c.price}
                    onChange={(e) => setPrice(c.id, e.target.value)}
                    style={{ width: 110 }}
                  />
                  <span className="muted">each</span>
                </>
              ) : (
                <span className="muted">{naira(toKobo(c.price || 0))} each</span>
              )}
              <strong style={{ marginLeft: 'auto' }}>{naira(toKobo(c.price || 0) * c.qty)}</strong>
            </div>
          </div>
        ))}
        {cart.length > 0 && (
          <>
            <h3 style={{ marginTop: 16 }}>Payment breakdown</h3>
            <div className="form-grid">
              {METHODS.map(([key, label]) => (
                <label key={key} className="field">
                  <span>{label} received (₦)</span>
                  <input
                    type="number"
                    min="0"
                    step="0.01"
                    value={tenders[key]}
                    placeholder="0.00"
                    onChange={(event) =>
                      setTenders((previous) => ({
                        ...previous,
                        [key]: event.target.value,
                      }))
                    }
                  />
                </label>
              ))}
            </div>
            {!hasTenderInput && (
              <div className="muted">
                Leave all payment amounts blank to record the full total as cash.
              </div>
            )}
            <div className="cart-line">
              <span>Total received now</span>
              <strong>{naira(amountReceived)}</strong>
            </div>
            <div className="cart-total">
              <span>Total (sale revenue)</span>
              <span>{naira(total)}</span>
            </div>
            {shortBy > 0 && (
              <label className="field" style={{ marginTop: 12 }}>
                <span>Customer name (required for an unpaid balance)</span>
                <input
                  required
                  maxLength="120"
                  value={customerName}
                  onChange={(e) => setCustomerName(e.target.value)}
                  placeholder="Customer name"
                />
              </label>
            )}
            <div className="cart-line">
              <span>{shortBy > 0 ? 'Balance to collect later' : 'Change to give'}</span>
              <strong className={shortBy > 0 ? 'low' : 'ok'}>
                {naira(shortBy > 0 ? shortBy : changeDue)}
              </strong>
            </div>
            {(paymentAmounts.transfer > 0 || paymentAmounts.pos > 0 || changeDue > 0) && (
              <div className="muted">
                {paymentAmounts.transfer > 0 &&
                  'Confirm the transfer has arrived before completing the sale. '}
                {paymentAmounts.pos > 0 &&
                  'Confirm the POS transaction succeeded before completing the sale. '}
                {changeDue > 0 && 'Change is paid from the cash drawer.'}
              </div>
            )}
            <button onClick={complete} disabled={shortBy > 0 && !customerName.trim()}>
              {shortBy > 0 ? 'Record sale with balance' : 'Confirm sale'}
            </button>
            <button className="ghost" style={{ width: '100%', marginTop: 8 }} onClick={clearSale}>
              Clear
            </button>
          </>
        )}
      </div>
    </div>
  )
}

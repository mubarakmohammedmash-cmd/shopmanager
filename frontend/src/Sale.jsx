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
  const [payment, setPayment] = useState('cash')
  const [received, setReceived] = useState('')

  const parsed = parseEntry(entry)
  const shown = products.filter((p) => p.name.toLowerCase().includes(parsed.name.toLowerCase()))
  const total = cart.reduce((sum, c) => sum + toKobo(c.price || 0) * c.qty, 0)
  const amountReceived = received.trim() === '' ? total : toKobo(received)
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
    setCart(
      cart.flatMap((c) => {
        if (c.id !== id) return [c]
        const qty = c.qty + delta
        if (qty > c.stock) return [c]
        return qty > 0 ? [{ ...c, qty }] : []
      }),
    )
  }

  async function complete() {
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
          payment_method: payment,
          amount_received: amountReceived,
          items: cart.map((c) => ({
            product_id: c.id,
            qty: c.qty,
            price: isAdmin ? toKobo(c.price) : undefined,
          })),
        }),
      })
      setCart([])
      setPayment('cash')
      setReceived('')
      notify(
        `Sale complete: ${naira(r.total)}. Change given: ${naira(r.change_given || 0)}`,
      )
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
            <div style={{ display: 'flex', gap: 6, marginTop: 14 }}>
              {METHODS.map(([key, label]) => (
                <button
                  key={key}
                  className={payment === key ? '' : 'ghost'}
                  onClick={() => setPayment(key)}
                  style={{ flex: 1 }}
                >
                  {label}
                </button>
              ))}
            </div>
            <div className="cart-total">
              <span>Total (sale revenue)</span>
              <span>{naira(total)}</span>
            </div>
            <label className="field" style={{ marginTop: 12 }}>
              <span>Amount received (₦)</span>
              <input
                type="number"
                min="0"
                step="0.01"
                value={received}
                placeholder={(total / 100).toFixed(2)}
                onChange={(e) => setReceived(e.target.value)}
              />
            </label>
            <div className="cart-line">
              <span>{shortBy > 0 ? 'Still owed' : 'Change to give'}</span>
              <strong className={shortBy > 0 ? 'low' : 'ok'}>
                {naira(shortBy > 0 ? shortBy : changeDue)}
              </strong>
            </div>
            {payment === 'transfer' && changeDue > 0 && (
              <div className="muted">
                Confirm that the transfer has arrived before completing the sale.
                Any change due is a cash payout from the drawer.
              </div>
            )}
            <button onClick={complete} disabled={shortBy > 0}>
              {shortBy > 0 ? 'Amount received is insufficient' : 'Confirm sale'}
            </button>
            <button className="ghost" style={{ width: '100%', marginTop: 8 }} onClick={() => setCart([])}>
              Clear
            </button>
          </>
        )}
      </div>
    </div>
  )
}

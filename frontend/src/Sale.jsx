import { useState } from 'react'

const API = 'http://localhost:8080'

const naira = (kobo) =>
  '₦' + (kobo / 100).toLocaleString('en-NG', { maximumFractionDigits: 2 })
const toKobo = (v) => Math.round(Number(v) * 100)

async function api(path, options) {
  const res = await fetch(API + path, options)
  if (!res.ok) throw new Error((await res.text()).trim())
  return res.json()
}

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

export default function SaleScreen({ products, reload, notify }) {
  const [entry, setEntry] = useState('')
  const [cart, setCart] = useState([])
  const [payment, setPayment] = useState('cash')

  const parsed = parseEntry(entry)
  const shown = products.filter((p) => p.name.toLowerCase().includes(parsed.name.toLowerCase()))
  const total = cart.reduce((sum, c) => sum + toKobo(c.price || 0) * c.qty, 0)

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
    if (parsed.price !== null && (Number.isNaN(parsed.price) || parsed.price < 0)) {
      notify('Price must be a number, e.g. peak milk : 250', 'error')
      return
    }
    if (shown.length === 0) {
      notify('No product matches that name', 'error')
      return
    }
    add(shown[0], parsed.price)
    setEntry('')
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
          items: cart.map((c) => ({ product_id: c.id, qty: c.qty, price: toKobo(c.price) })),
        }),
      })
      setCart([])
      setPayment('cash')
      notify(`Sale complete: ${naira(r.total)}`)
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
          placeholder="Product, or product : price  (e.g. peak milk : 250), then Enter"
          value={entry}
          onChange={(e) => setEntry(e.target.value)}
          onKeyDown={onEnter}
        />
        <div className="muted" style={{ marginTop: 6 }}>
          Enter adds the first match. Without a price it uses the saved price.
        </div>
        {products.length === 0 && <div className="empty">Add products in the Products tab first.</div>}
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
              {toKobo(c.price || 0) < c.cost && <div className="low">Below cost price</div>}
            </div>
            <div className="qty">
              <button onClick={() => change(c.id, -1)}>-</button>
              <strong>{c.qty}</strong>
              <button onClick={() => change(c.id, 1)}>+</button>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, width: '100%' }}>
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
              <span>Total</span>
              <span>{naira(total)}</span>
            </div>
            <button onClick={complete}>Confirm sale</button>
            <button className="ghost" style={{ width: '100%', marginTop: 8 }} onClick={() => setCart([])}>
              Clear
            </button>
          </>
        )}
      </div>
    </div>
  )
}

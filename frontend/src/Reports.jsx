import { useEffect, useState } from 'react'
import { api, downloadCSV, naira, lagos } from './api.js'

const method = { cash: 'Cash', transfer: 'Transfer', pos: 'POS' }

const chartColors = [
  '#2563eb',
  '#16a34a',
  '#f59e0b',
  '#dc2626',
  '#9333ea',
  '#0891b2',
  '#ea580c',
  '#4f46e5',
  '#db2777',
  '#64748b',
]

function Stat({ label, value, tone }) {
  return (
    <div className={'stat ' + (tone || '')}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}

function ProductPieChart({ products = [] }) {
  const items = products.filter((p) => p.units > 0)
  const total = items.reduce((sum, p) => sum + p.units, 0)

  if (!total) {
    return <div className="empty">No product sales to display.</div>
  }

  let currentAngle = 0

  const slices = items.map((product, index) => {
    const angle = (product.units / total) * 360
    const start = currentAngle
    currentAngle += angle

    return {
      ...product,
      color: chartColors[index % chartColors.length],
      start,
      angle,
    }
  })

  const point = (angle) => {
    const radians = ((angle - 90) * Math.PI) / 180
    return {
      x: 100 + 85 * Math.cos(radians),
      y: 100 + 85 * Math.sin(radians),
    }
  }

  return (
    <div className="chart-layout">
      <svg
        viewBox="0 0 200 200"
        role="img"
        aria-label="Best-selling products by units sold"
        style={{ width: 'min(100%, 240px)', height: 'auto' }}
      >
        {slices.map((slice, index) => {
          if (slice.angle >= 359.99) {
            return (
              <circle
                key={slice.name}
                cx="100"
                cy="100"
                r="85"
                fill={slice.color}
              />
            )
          }

          const start = point(slice.start)
          const end = point(slice.start + slice.angle)
          const largeArc = slice.angle > 180 ? 1 : 0

          return (
            <path
              key={slice.name}
              d={`M 100 100 L ${start.x} ${start.y} A 85 85 0 ${largeArc} 1 ${end.x} ${end.y} Z`}
              fill={slice.color}
              stroke="white"
              strokeWidth="1"
            />
          )
        })}
      </svg>

      <div className="chart-legend">
        {slices.map((product) => (
          <div className="legend-item" key={product.name}>
            <span
              className="legend-dot"
              style={{ backgroundColor: product.color }}
            />
            <span className="legend-name">{product.name}</span>
            <strong>{product.units}</strong>
          </div>
        ))}
      </div>
    </div>
  )
}

function ProductProfitChart({ products = [] }) {
  const items = [...products]
    .filter((p) => p.profit > 0)
    .sort((a, b) => b.profit - a.profit)
    .slice(0, 5)

  const maximum = Math.max(...items.map((p) => p.profit), 0)

  if (!items.length) {
    return <div className="empty">No product profits to display.</div>
  }

  return (
    <div className="chart-bars">
      {items.map((product, index) => (
        <div className="bar-row" key={product.name}>
          <span className="bar-label" title={product.name}>
            {product.name}
          </span>
          <div className="bar-track">
            <div
              className="bar-fill"
              style={{
                width: `${(product.profit / maximum) * 100}%`,
                backgroundColor: chartColors[index % chartColors.length],
              }}
            />
          </div>
          <strong>{naira(product.profit)}</strong>
        </div>
      ))}
    </div>
  )
}

function RevenueLineChart({ days = [] }) {
  const items = days.filter((day) => Number.isFinite(Number(day.revenue)))

  if (!items.length) {
    return <div className="empty">No daily revenue to display.</div>
  }

  const width = 600
  const height = 240
  const paddingLeft = 55
  const paddingRight = 20
  const paddingTop = 20
  const paddingBottom = 45

  const plotWidth = width - paddingLeft - paddingRight
  const plotHeight = height - paddingTop - paddingBottom

  const maximum = Math.max(...items.map((day) => Number(day.revenue)), 1)

  const points = items.map((day, index) => ({
    ...day,
    x:
      paddingLeft +
      (items.length === 1 ? plotWidth / 2 : (index / (items.length - 1)) * plotWidth),
    y:
      paddingTop +
      plotHeight -
      (Number(day.revenue) / maximum) * plotHeight,
  }))

  const line = points.map((p) => `${p.x},${p.y}`).join(' ')
  const area = [
    `${points[0].x},${paddingTop + plotHeight}`,
    ...points.map((p) => `${p.x},${p.y}`),
    `${points[points.length - 1].x},${paddingTop + plotHeight}`,
  ].join(' ')

  const labelIndexes = [...new Set([
    0,
    Math.floor((items.length - 1) / 2),
    items.length - 1,
  ])]

  return (
    <div style={{ width: '100%', overflowX: 'auto' }}>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label="Daily sales revenue trend"
        style={{ width: '100%', minWidth: '280px', height: 'auto' }}
      >
        {[0, 0.5, 1].map((fraction) => {
          const y = paddingTop + plotHeight * (1 - fraction)

          return (
            <g key={fraction}>
              <line
                x1={paddingLeft}
                y1={y}
                x2={width - paddingRight}
                y2={y}
                stroke="#e5e7eb"
                strokeDasharray="4 4"
              />
              <text
                x={paddingLeft - 8}
                y={y + 4}
                textAnchor="end"
                fontSize="11"
                fill="currentColor"
              >
                {Math.round(maximum * fraction).toLocaleString()}
              </text>
            </g>
          )
        })}

        <polygon points={area} fill="#2563eb" opacity="0.12" />

        <polyline
          points={line}
          fill="none"
          stroke="#2563eb"
          strokeWidth="3"
          strokeLinejoin="round"
          strokeLinecap="round"
        />

        {points.map((p) => (
          <circle
            key={p.date}
            cx={p.x}
            cy={p.y}
            r="4"
            fill="#2563eb"
          >
            <title>{`${p.date}: ${naira(p.revenue)}`}</title>
          </circle>
        ))}

        {labelIndexes.map((index) => {
          const p = points[index]

          return (
            <text
              key={p.date}
              x={p.x}
              y={height - 15}
              textAnchor={
                index === 0
                  ? 'start'
                  : index === points.length - 1
                    ? 'end'
                    : 'middle'
              }
              fontSize="11"
              fill="currentColor"
            >
              {p.date}
            </text>
          )
        })}
      </svg>
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
        <h2>Reports & Analytics</h2>

        <div className="form-grid">
          <label className="field">
            <span>From</span>
            <input
              type="date"
              value={from}
              max={to}
              onChange={(e) => setFrom(e.target.value)}
            />
          </label>

          <label className="field">
            <span>To</span>
            <input
              type="date"
              value={to}
              min={from}
              max={lagos()}
              onChange={(e) => setTo(e.target.value)}
            />
          </label>

          <button className="ghost" onClick={() => quick(lagos())}>
            Today
          </button>

          <button className="ghost" onClick={() => quick(lagos(6))}>
            Last 7 days
          </button>

          <button className="ghost" onClick={() => quick(lagos(29))}>
            Last 30 days
          </button>

          <button
            className="ghost"
            onClick={() => quick(lagos().slice(0, 8) + '01')}
          >
            This month
          </button>

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
            <Stat
              label="Transfers received"
              value={naira(data.transfer_received ?? 0)}
            />
            <Stat label="POS received" value={naira(data.pos_received ?? 0)} />
            <Stat
              label="Cash change paid out"
              value={naira(data.cash_change_paid ?? 0)}
            />
            <Stat
              label="Net cash drawer movement"
              value={naira(data.cash_drawer_net ?? 0)}
              tone={(data.cash_drawer_net ?? 0) < 0 ? 'bad' : 'good'}
            />
          </div>

          <div className="grid-2">
            <div className="card">
              <h2>Best-selling products</h2>
              <p className="muted">Units sold for each product</p>
              <ProductPieChart products={data.top_products} />
            </div>

            <div className="card">
              <h2>Most profitable products</h2>
              <p className="muted">Top products by gross profit</p>
              <ProductProfitChart products={data.top_products} />
            </div>
          </div>

          <div className="card">
            <h2>Sales revenue trend</h2>
            <p className="muted">Daily revenue for the selected period</p>
            <RevenueLineChart days={data.days} />
          </div>

          <div className="grid-2">
            <div className="card">
              <h2>Money received by payment method</h2>

              {data.payments.length === 0 && (
                <div className="empty">No sales in this period.</div>
              )}

              {data.payments.map((p) => (
                <div key={p.method} className="cart-line">
                  <span>{method[p.method] || p.method}</span>
                  <strong>{naira(p.amount)}</strong>
                </div>
              ))}
            </div>

            <div className="card">
              <h2>Top sellers</h2>

              {data.top_products.length === 0 && (
                <div className="empty">No sales in this period.</div>
              )}

              {data.top_products.map((t) => (
                <div key={t.name} className="cart-line">
                  <span>{t.name}</span>
                  <span className="muted">
                    {t.units} sold,{' '}
                    <span className="ok">{naira(t.profit)} profit</span>
                  </span>
                </div>
              ))}
            </div>
          </div>

          <div className="card">
            <h2>Day by day</h2>

            {data.days.length === 0 && (
              <div className="empty">No sales in this period.</div>
            )}

            {data.days.length > 0 && (
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Date</th>
                      <th>Sales</th>
                      <th>Gross profit</th>
                    </tr>
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

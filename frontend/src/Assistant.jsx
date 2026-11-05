import { useEffect, useRef, useState } from 'react'
import { api, naira } from './api.js'

const initialMessage = {
  role: 'assistant',
  content: 'Hi! Ask me to open a screen, check today’s sales or product stock, or prepare a product to add.',
}

export default function Assistant({ isAdmin, setTab, reload }) {
  const [open, setOpen] = useState(false)
  const [messages, setMessages] = useState([initialMessage])
  const [input, setInput] = useState('')
  const [busy, setBusy] = useState(false)
  const [listening, setListening] = useState(false)
  const [draft, setDraft] = useState(null)
  const [savingDraft, setSavingDraft] = useState(false)
  const recognitionRef = useRef(null)
  const conversationRef = useRef(null)

  useEffect(() => {
    conversationRef.current?.scrollTo({
      top: conversationRef.current.scrollHeight,
      behavior: 'smooth',
    })
  }, [messages, draft, busy])

  useEffect(() => () => {
    if (recognitionRef.current) {
      recognitionRef.current.onend = null
      recognitionRef.current.stop()
    }
  }, [])

  async function sendMessage(event) {
    event.preventDefault()
    const text = input.trim()
    if (!text || busy) return

    const outgoing = [...messages, { role: 'user', content: text }]
    setMessages(outgoing)
    setInput('')
    setBusy(true)
    setDraft(null)

    try {
      const result = await api('/assistant/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          messages: outgoing
            .filter((message) => message.role === 'user' || message.role === 'assistant')
            .slice(-20),
        }),
      })
      if (result.action?.screen) {
        setTab(result.action.screen)
      }
      setDraft(result.action?.product_draft || null)
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: result.reply || 'I could not prepare a reply.' },
      ])
    } catch (error) {
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: `I couldn’t complete that request: ${error.message}` },
      ])
    } finally {
      setBusy(false)
    }
  }

  function startVoiceInput() {
    const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition
    if (!SpeechRecognition) {
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: 'Voice input is not supported in this browser. You can type your request instead.' },
      ])
      return
    }

    const recognition = new SpeechRecognition()
    recognition.lang = 'en-NG'
    recognition.interimResults = false
    recognition.maxAlternatives = 1
    recognitionRef.current = recognition
    recognition.onresult = (event) => {
      setInput((current) => {
        const spoken = event.results[0][0].transcript.trim()
        return current ? `${current} ${spoken}` : spoken
      })
    }
    recognition.onerror = (event) => {
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: event.error === 'not-allowed'
          ? 'Microphone access was denied. Allow microphone access and try again.'
          : `Voice input failed: ${event.error}. You can type your request instead.` },
      ])
    }
    recognition.onend = () => {
      setListening(false)
      recognitionRef.current = null
    }

    try {
      setListening(true)
      recognition.start()
    } catch (error) {
      setListening(false)
      recognitionRef.current = null
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: `Could not start voice input: ${error.message}` },
      ])
    }
  }

  async function confirmProduct() {
    if (!draft || savingDraft) return
    setSavingDraft(true)
    try {
      const product = await api('/products', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: draft.name,
          cost_price: draft.cost_price,
          selling_price: draft.selling_price,
          stock: draft.stock,
          low_stock_level: draft.low_stock_level,
        }),
      })
      setDraft(null)
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: `${product.name} was added to your products.` },
      ])
      setTab('products')
      await reload()
    } catch (error) {
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: `I couldn’t add the product: ${error.message}` },
      ])
    } finally {
      setSavingDraft(false)
    }
  }

  function cancelProduct() {
    setDraft(null)
    setMessages((current) => [
      ...current,
      { role: 'assistant', content: 'Product cancelled. Nothing was added.' },
    ])
  }

  return (
    <div className="assistant">
      {open && (
        <section className="assistant-panel" aria-label="ShopManager AI assistant">
          <header className="assistant-header">
            <div>
              <strong>Shop assistant</strong>
              <span>AI · {isAdmin ? 'Manager' : 'Sales rep'} access</span>
            </div>
            <button type="button" className="ghost assistant-close" onClick={() => setOpen(false)} aria-label="Close assistant">
              Close
            </button>
          </header>

          <div className="assistant-messages" ref={conversationRef} aria-live="polite">
            {messages.map((message, index) => (
              <div
                className={`assistant-message ${message.role === 'user' ? 'assistant-user-message' : ''}`}
                key={`${index}-${message.role}`}
              >
                {message.content}
              </div>
            ))}

            {draft && (
              <div className="assistant-draft">
                <strong>Review product before adding</strong>
                <div><span>Name</span><b>{draft.name}</b></div>
                <div><span>Cost</span><b>{naira(draft.cost_price)}</b></div>
                <div><span>Selling price</span><b>{naira(draft.selling_price)}</b></div>
                <div><span>Opening stock</span><b>{draft.stock}</b></div>
                <div><span>Low-stock alert</span><b>{draft.low_stock_level}</b></div>
                <div className="assistant-draft-actions">
                  <button type="button" disabled={savingDraft} onClick={confirmProduct}>
                    {savingDraft ? 'Adding...' : 'Confirm and add'}
                  </button>
                  <button type="button" className="ghost" disabled={savingDraft} onClick={cancelProduct}>
                    Cancel
                  </button>
                </div>
              </div>
            )}

            {busy && <div className="assistant-message">Thinking…</div>}
          </div>

          <form className="assistant-composer" onSubmit={sendMessage}>
            <p className="assistant-privacy">
              Messages and relevant shop data go to OpenAI for replies. Voice is transcribed by your browser.
            </p>
            <textarea
              aria-label="Message the assistant"
              placeholder="Ask or give a command..."
              value={input}
              maxLength={4000}
              rows={2}
              onChange={(event) => setInput(event.target.value)}
              disabled={busy || Boolean(draft)}
            />
            <div>
              <button type="button" className="ghost" onClick={startVoiceInput} disabled={busy || Boolean(draft) || listening}>
                {listening ? 'Listening…' : 'Speak'}
              </button>
              {listening && (
                <button type="button" className="ghost" onClick={() => recognitionRef.current?.stop()}>
                  Stop
                </button>
              )}
              <button type="submit" disabled={busy || Boolean(draft) || !input.trim()}>
                Send
              </button>
            </div>
          </form>
        </section>
      )}
      <button
        type="button"
        className="assistant-launcher"
        onClick={() => setOpen((current) => !current)}
        aria-expanded={open}
        aria-label={open ? 'Close AI assistant' : 'Open AI assistant'}
      >
        {open ? 'Close assistant' : 'Ask AI'}
      </button>
    </div>
  )
}

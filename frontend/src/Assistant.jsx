import { useEffect, useRef, useState } from 'react'
import { api, naira } from './api.js'

const MAX_FILE_SIZE = 4 * 1024 * 1024
const MAX_FILES = 3
const ALLOWED_FILE_TYPES = new Set([
  'png', 'jpg', 'jpeg', 'webp', 'gif', 'pdf', 'txt', 'md', 'csv', 'json',
  'html', 'xml', 'doc', 'docx', 'ppt', 'pptx', 'xls', 'xlsx', 'rtf', 'odt',
])

const initialMessage = {
  role: 'assistant',
  content: 'Hi! I can answer general questions, analyze images and files, search the web, or help with your shop.',
}

export default function Assistant({ isAdmin, setTab, reload }) {
  const [open, setOpen] = useState(false)
  const [messages, setMessages] = useState([initialMessage])
  const [input, setInput] = useState('')
  const [attachments, setAttachments] = useState([])
  const [webSearch, setWebSearch] = useState(false)
  const [busy, setBusy] = useState(false)
  const [listening, setListening] = useState(false)
  const [speakReplies, setSpeakReplies] = useState(false)
  const [draft, setDraft] = useState(null)
  const [savingDraft, setSavingDraft] = useState(false)
  const recognitionRef = useRef(null)
  const conversationRef = useRef(null)
  const fileInputRef = useRef(null)

  useEffect(() => {
    conversationRef.current?.scrollTo({
      top: conversationRef.current.scrollHeight,
      behavior: 'smooth',
    })
  }, [messages, draft, busy])

  useEffect(() => () => {
    window.speechSynthesis?.cancel()
    if (recognitionRef.current) {
      recognitionRef.current.onend = null
      recognitionRef.current.stop()
    }
  }, [])

  function readAttachment(file) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onerror = () => reject(new Error(`Could not read ${file.name}`))
      reader.onload = () => {
        const dataURL = String(reader.result || '')
        const separator = dataURL.indexOf(',')
        if (separator < 0) {
          reject(new Error(`Could not encode ${file.name}`))
          return
        }
        resolve({ name: file.name, data: dataURL.slice(separator + 1) })
      }
      reader.readAsDataURL(file)
    })
  }

  async function submitMessage(rawText, files = attachments, searchWeb = webSearch, readReply = speakReplies) {
    const text = rawText.trim() || (files.length > 0 ? 'Please review the attached file(s).' : '')
    if (!text || busy) return

    const attachmentNames = files.map((file) => file.name)
    const outgoing = [...messages, {
      role: 'user',
      content: text,
      attachments: attachmentNames,
    }]
    setMessages(outgoing)
    setInput('')
    setAttachments([])
    setBusy(true)
    setDraft(null)

    try {
      const encodedAttachments = await Promise.all(files.map(readAttachment))
      const result = await api('/assistant/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          attachments: encodedAttachments,
          web_search: searchWeb,
          messages: outgoing
            .filter((message) => message.role === 'user' || message.role === 'assistant')
            .slice(-20),
        }),
      })
      if (result.action?.screen) {
        setTab(result.action.screen)
      }
      setDraft(result.action?.product_draft || null)
      const reply = result.reply || 'I could not prepare a reply.'
      setMessages((current) => [
        ...current,
        {
          role: 'assistant',
          content: reply,
          citations: result.citations || [],
          chart: result.action?.chart || null,
        },
      ])
      if (readReply && window.speechSynthesis) {
        window.speechSynthesis.cancel()
        const utterance = new SpeechSynthesisUtterance(reply)
        utterance.lang = 'en-NG'
        window.speechSynthesis.speak(utterance)
      }
    } catch (error) {
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: `I couldn’t complete that request: ${error.message}` },
      ])
    } finally {
      setBusy(false)
    }
  }

  function sendMessage(event) {
    event.preventDefault()
    submitMessage(input)
  }

  function selectFiles(event) {
    const selected = Array.from(event.target.files || [])
    event.target.value = ''
    if (selected.length === 0) return

    for (const file of selected) {
      const extension = file.name.split('.').pop()?.toLowerCase()
      if (!extension || !ALLOWED_FILE_TYPES.has(extension)) {
        setMessages((current) => [
          ...current,
          { role: 'assistant', content: `${file.name} is not a supported image or document type.` },
        ])
        return
      }
      if (file.size === 0 || file.size > MAX_FILE_SIZE) {
        setMessages((current) => [
          ...current,
          { role: 'assistant', content: `$          {file.name} must be 4 MB or smaller.` },
        ])
        return
      }
    }

    if (attachments.length + selected.length > MAX_FILES) {
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: 'Attach no more than 3 files per message.' },
      ])
      return
    }
    setAttachments((current) => [...current, ...selected])
  }

  function removeAttachment(index) {
    setAttachments((current) => current.filter((_, itemIndex) => itemIndex !== index))
  }

  function toggleVoiceReplies() {
    if (speakReplies) window.speechSynthesis?.cancel()
    setSpeakReplies(!speakReplies)
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
      const spoken = event.results[0][0].transcript.trim()
      const text = [input.trim(), spoken].filter(Boolean).join(' ')
      setInput(text)
      setSpeakReplies(true)
      submitMessage(text, [], false, true)
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
      if ('speechSynthesis' in window) setSpeakReplies(true)
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
                {message.attachments?.length > 0 && (
                  <div className="assistant-attachment-list" aria-label="Attached files">
                    {message.attachments.map((name) => <span key={name}>{name}</span>)}
                  </div>
                )}
                {message.citations?.length > 0 && (
                  <div className="assistant-citations">
                    <strong>Sources</strong>
                    {message.citations.map((citation) => (
                      <a key={citation.url} href={citation.url} target="_blank" rel="noreferrer">
                        {citation.title || citation.url}
                      </a>
                    ))}
                  </div>
                )}
                {message.chart && <AssistantChart chart={message.chart} />}
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
              Voice uses your browser. Recognized text, messages, files, and web searches go to OpenAI; usage may incur API charges.
            </p>
            {attachments.length > 0 && (
              <div className="assistant-selected-files">
                {attachments.map((file, index) => (
                  <span key={`${file.name}-${index}`}>
                    {file.name}
                    <button type="button" className="ghost" aria-label={`Remove ${file.name}`} onClick={() => removeAttachment(index)}>
                      ×
                    </button>
                  </span>
                ))}
              </div>
            )}
            <textarea
              aria-label="Message the assistant"
              placeholder="Ask anything, or ask about your shop..."
              value={input}
              maxLength={4000}
              rows={2}
              onChange={(event) => setInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
                  event.preventDefault()
                  event.currentTarget.form?.requestSubmit()
                }
              }}
              disabled={busy || Boolean(draft) || listening}
            />
            <div>
              <input
                ref={fileInputRef}
                className="assistant-file-input"
                type="file"
                multiple
                accept="image/*,.pdf,.txt,.md,.csv,.json,.html,.xml,.doc,.docx,.ppt,.pptx,.xls,.xlsx,.rtf,.odt"
                onChange={selectFiles}
                disabled={busy || Boolean(draft)}
              />
              <button type="button" className="ghost" onClick={() => fileInputRef.current?.click()} disabled={busy || Boolean(draft) || attachments.length >= MAX_FILES}>
                Attach
              </button>
              <button type="button" className={webSearch ? '' : 'ghost'} aria-pressed={webSearch} onClick={() => setWebSearch((current) => !current)} disabled={busy || Boolean(draft)}>
                {webSearch ? 'Web search on' : 'Search web'}
              </button>
              <button type="button" className={speakReplies ? '' : 'ghost'} aria-pressed={speakReplies} onClick={toggleVoiceReplies} disabled={!('speechSynthesis' in window) || busy || Boolean(draft)}>
                {speakReplies ? 'Voice replies on' : 'Voice replies'}
              </button>
              <button type="button" className="ghost" onClick={startVoiceInput} disabled={busy || Boolean(draft) || listening}>
                {listening ? 'Listening…' : 'Speak'}
              </button>
              {listening && (
                <button type="button" className="ghost" onClick={() => recognitionRef.current?.stop()}>
                  Stop
                </button>
              )}
              <button type="submit" disabled={busy || Boolean(draft) || (!input.trim() && attachments.length === 0)}>
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

function AssistantChart({ chart }) {
  const maximum = Math.max(0, ...chart.points.map((point) => point.value))
  return (
    <div className="assistant-chart" role="img" aria-label={chart.title}>
      <strong>{chart.title}</strong>
      {chart.points.map((point) => (
        <div className="assistant-chart-row" key={point.label}>
          <span>{point.label.slice(5)}</span>
          <div className="assistant-chart-track">
            <div style={{ width: maximum > 0 ? `${(point.value / maximum) * 100}%` : '0%' }} />
          </div>
          <b>{naira(point.value)}</b>
        </div>
      ))}
    </div>
  )
}

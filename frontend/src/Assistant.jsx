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
  const [transcribing, setTranscribing] = useState(false)
  const [voiceConversation, setVoiceConversation] = useState(false)
  const [draft, setDraft] = useState(null)
  const [savingDraft, setSavingDraft] = useState(false)
  const recorderRef = useRef(null)
  const audioStreamRef = useRef(null)
  const audioContextRef = useRef(null)
  const voiceMonitorRef = useRef(null)
  const audioChunksRef = useRef([])
  const voiceConversationRef = useRef(false)
  const startVoiceInputRef = useRef(null)
  const submitMessageRef = useRef(null)
  const webSearchRef = useRef(webSearch)
  webSearchRef.current = webSearch
  const conversationRef = useRef(null)
  const fileInputRef = useRef(null)

  useEffect(() => {
    conversationRef.current?.scrollTo({
      top: conversationRef.current.scrollHeight,
      behavior: 'smooth',
    })
  }, [messages, draft, busy])

  useEffect(() => () => {
    voiceConversationRef.current = false
    window.speechSynthesis?.cancel()
    window.clearInterval(voiceMonitorRef.current)
    audioContextRef.current?.close()
    if (recorderRef.current) {
      recorderRef.current.onstop = null
      recorderRef.current.onerror = null
      if (recorderRef.current.state === 'recording') recorderRef.current.stop()
    }
    audioStreamRef.current?.getTracks().forEach((track) => track.stop())
  }, [])

  useEffect(() => {
    startVoiceInputRef.current = startVoiceInput
    submitMessageRef.current = submitMessage
  })

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

  async function submitMessage(rawText, files = attachments, searchWeb = webSearch, readReply = false) {
    const text = rawText.trim() || (files.length > 0 ? 'Please review the attached file(s).' : '')
    if (!text || busy) return false

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
      if (readReply && voiceConversationRef.current) {
        if (!window.speechSynthesis || typeof SpeechSynthesisUtterance === 'undefined') {
          setMessages((current) => [
            ...current,
            { role: 'assistant', content: 'This browser does not support spoken replies. Try a recent browser with speech synthesis enabled.' },
          ])
        } else {
          window.speechSynthesis.cancel()
          const utterance = new SpeechSynthesisUtterance(reply)
          utterance.lang = 'en-NG'
          let speechErrored = false
          utterance.onend = () => {
            if (!speechErrored && voiceConversationRef.current) startVoiceInputRef.current?.()
          }
          utterance.onerror = (event) => {
            speechErrored = true
            setMessages((current) => [
              ...current,
              { role: 'assistant', content: `Could not speak the reply: ${event.error}. Check your device's speech settings.` },
            ])
            if (voiceConversationRef.current) startVoiceInputRef.current?.()
          }
          window.speechSynthesis.speak(utterance)
        }
      }
      return true
    } catch (error) {
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: `I couldn’t complete that request: ${error.message}` },
      ])
      return false
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
          { role: 'assistant', content: `${file.name} must be 4 MB or smaller.` },
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

  function stopVoiceMonitor() {
    window.clearInterval(voiceMonitorRef.current)
    voiceMonitorRef.current = null
    if (audioContextRef.current) {
      audioContextRef.current.close()
      audioContextRef.current = null
    }
  }

  function toggleVoiceConversation() {
    if (voiceConversationRef.current) {
      voiceConversationRef.current = false
      setVoiceConversation(false)
      window.speechSynthesis?.cancel()
      stopVoiceMonitor()
      if (recorderRef.current) {
        recorderRef.current.onstop = null
        if (recorderRef.current.state === 'recording') recorderRef.current.stop()
        recorderRef.current = null
      }
      audioStreamRef.current?.getTracks().forEach((track) => track.stop())
      audioStreamRef.current = null
      audioChunksRef.current = []
      setListening(false)
      return
    }
    if (!window.speechSynthesis || typeof SpeechSynthesisUtterance === 'undefined') {
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: 'Spoken replies are not supported in this browser. Try a recent browser with speech synthesis enabled.' },
      ])
      return
    }
    voiceConversationRef.current = true
    setVoiceConversation(true)
    startVoiceInputRef.current?.()
  }

  function audioExtension(mimeType) {
    if (mimeType.includes('mp4')) return 'm4a'
    if (mimeType.includes('ogg')) return 'ogg'
    if (mimeType.includes('mpeg') || mimeType.includes('mp3')) return 'mp3'
    return 'webm'
  }

  async function startVoiceInput() {
    if (!voiceConversationRef.current) return
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
      voiceConversationRef.current = false
      setVoiceConversation(false)
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: 'Voice recording is not supported in this browser. Use a recent browser over HTTPS, or type your message.' },
      ])
      return
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      if (!voiceConversationRef.current) {
        stream.getTracks().forEach((track) => track.stop())
        return
      }
      audioStreamRef.current = stream
      audioChunksRef.current = []
      const mimeType = ['audio/webm;codecs=opus', 'audio/mp4', 'audio/ogg;codecs=opus']
        .find((type) => MediaRecorder.isTypeSupported(type))
      const recorder = mimeType ? new MediaRecorder(stream, { mimeType }) : new MediaRecorder(stream)
      recorderRef.current = recorder
      const audioContext = new AudioContext()
      audioContextRef.current = audioContext
      await audioContext.resume()
      const analyser = audioContext.createAnalyser()
      analyser.fftSize = 2048
      audioContext.createMediaStreamSource(stream).connect(analyser)
      const samples = new Float32Array(analyser.fftSize)
      let heardSpeech = false
      let quietSince = 0
      const startedAt = Date.now()
      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) audioChunksRef.current.push(event.data)
      }
      recorder.onerror = () => {
        stopVoiceMonitor()
        recorder.onstop = null
        stream.getTracks().forEach((track) => track.stop())
        audioStreamRef.current = null
        recorderRef.current = null
        audioChunksRef.current = []
        setListening(false)
        voiceConversationRef.current = false
        setVoiceConversation(false)
        setMessages((current) => [
          ...current,
          { role: 'assistant', content: 'Microphone recording failed. Check microphone access and try again.' },
        ])
      }
      recorder.onstop = async () => {
        stopVoiceMonitor()
        stream.getTracks().forEach((track) => track.stop())
        audioStreamRef.current = null
        recorderRef.current = null
        setListening(false)
        const blob = new Blob(audioChunksRef.current, { type: recorder.mimeType || 'audio/webm' })
        audioChunksRef.current = []
        if (!blob.size || !heardSpeech) {
          if (voiceConversationRef.current) startVoiceInputRef.current?.()
          return
        }

        setTranscribing(true)
        try {
          const form = new FormData()
          form.append('audio', blob, `voice-message.${audioExtension(blob.type)}`)
          const result = await api('/assistant/transcribe', {
            method: 'POST',
            body: form,
          })
          const transcript = String(result.text || '').trim()
          if (!transcript) throw new Error('No speech was detected. Please try again.')
          if (!voiceConversationRef.current) return
          setTranscribing(false)
          const submitted = await submitMessageRef.current?.(transcript, attachments, webSearchRef.current, true)
          if (!submitted && voiceConversationRef.current) startVoiceInputRef.current?.()
        } catch (error) {
          setMessages((current) => [
            ...current,
            { role: 'assistant', content: `Voice request failed: ${error.message}` },
          ])
          if (voiceConversationRef.current) startVoiceInputRef.current?.()
        } finally {
          setTranscribing(false)
        }
      }
      recorder.start()
      setListening(true)
      voiceMonitorRef.current = window.setInterval(() => {
        analyser.getFloatTimeDomainData(samples)
        let energy = 0
        for (const sample of samples) energy += sample * sample
        const volume = Math.sqrt(energy / samples.length)
        if (volume > 0.015) {
          heardSpeech = true
          quietSince = 0
        } else if (heardSpeech) {
          if (!quietSince) quietSince = Date.now()
          if (Date.now() - quietSince >= 1200) stopVoiceInput()
        }
        if (Date.now() - startedAt >= 60000) stopVoiceInput()
      }, 100)
    } catch (error) {
      stopVoiceMonitor()
      recorderRef.current = null
      audioStreamRef.current?.getTracks().forEach((track) => track.stop())
      audioStreamRef.current = null
      setListening(false)
      voiceConversationRef.current = false
      setVoiceConversation(false)
      setMessages((current) => [
        ...current,
        { role: 'assistant', content: error.name === 'NotAllowedError'
          ? 'Microphone access was denied. Allow microphone access in your browser and try again.'
          : `Could not start microphone recording: ${error.message}` },
      ])
    }
  }

  function stopVoiceInput() {
    if (recorderRef.current?.state === 'recording') {
      recorderRef.current.stop()
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

  function closeAssistant() {
    if (voiceConversationRef.current) toggleVoiceConversation()
    setOpen(false)
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
            <button type="button" className="ghost assistant-close" onClick={closeAssistant} aria-label="Close assistant">
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

            {listening && <div className="assistant-message">Listening… Speak naturally. I’ll send your message after a pause.</div>}
            {voiceConversation && !listening && !transcribing && !busy && <div className="assistant-message">Voice conversation is on.</div>}
            {transcribing && <div className="assistant-message">Transcribing your voice…</div>}
            {busy && <div className="assistant-message">Thinking…</div>}
          </div>

          <form className="assistant-composer" onSubmit={sendMessage}>
            <p className="assistant-privacy">
              Voice recordings, messages, files, and web searches go to OpenAI; usage may incur API charges. Replies use your browser's voice.
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
              disabled={busy || transcribing || Boolean(draft) || voiceConversation}
            />
            <div>
              <input
                ref={fileInputRef}
                className="assistant-file-input"
                type="file"
                multiple
                accept="image/*,.pdf,.txt,.md,.csv,.json,.html,.xml,.doc,.docx,.ppt,.pptx,.xls,.xlsx,.rtf,.odt"
                onChange={selectFiles}
                disabled={busy || transcribing || Boolean(draft) || voiceConversation}
              />
              <button type="button" className="ghost" onClick={() => fileInputRef.current?.click()} disabled={busy || transcribing || Boolean(draft) || voiceConversation || attachments.length >= MAX_FILES}>
                Attach
              </button>
              <button type="button" className={webSearch ? '' : 'ghost'} aria-pressed={webSearch} onClick={() => setWebSearch((current) => !current)} disabled={busy || transcribing || Boolean(draft) || voiceConversation}>
                {webSearch ? 'Web search on' : 'Search web'}
              </button>
              <button
                type="button"
                className={voiceConversation ? '' : 'ghost'}
                onClick={toggleVoiceConversation}
                disabled={Boolean(draft)}
                aria-pressed={voiceConversation}
              >
                {voiceConversation ? 'End voice conversation' : 'Start voice conversation'}
              </button>
              <button type="submit" disabled={busy || transcribing || Boolean(draft) || voiceConversation || (!input.trim() && attachments.length === 0)}>
                Send
              </button>
            </div>
          </form>
        </section>
      )}
      <button
        type="button"
        className="assistant-launcher"
        onClick={() => (open ? closeAssistant() : setOpen(true))}
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

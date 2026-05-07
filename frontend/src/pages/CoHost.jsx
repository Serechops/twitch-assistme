import { useEffect, useRef, useState } from 'react'
import { CoHostRespondToChat, CoHostSpeakText, ClearCoHostSession, GetSettings } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

const MAX_CHAT = 60
const MAX_LOG  = 100

// Decode a base64 MP3 string into a playable Audio object.
function b64ToAudio(b64) {
  const bytes = Uint8Array.from(atob(b64), c => c.charCodeAt(0))
  const blob  = new Blob([bytes], { type: 'audio/mpeg' })
  return new Audio(URL.createObjectURL(blob))
}

export default function CoHost() {
  const [settings, setSettings]     = useState(null)
  const [chatFeed, setChatFeed]      = useState([])
  const [coHostLog, setCoHostLog]    = useState([])
  const [speakText, setSpeakText]    = useState('')
  const [autoRespond, setAutoRespond] = useState(
    () => localStorage.getItem('cohost_autoRespond') === 'true'
  )
  const [busy, setBusy]              = useState(false)
  const [speakBusy, setSpeakBusy]    = useState(false)
  const [error, setError]            = useState('')
  const [notice, setNotice]          = useState('')

  // Persist autoRespond across navigation
  useEffect(() => {
    localStorage.setItem('cohost_autoRespond', autoRespond ? 'true' : 'false')
  }, [autoRespond])

  const audioRef    = useRef(null)
  const chatRef     = useRef(null)
  const logRef      = useRef(null)
  // Prevent duplicate auto-responses for the same message
  const respondedIds = useRef(new Set())

  // Load settings on mount
  useEffect(() => {
    GetSettings()
      .then(s => setSettings(s))
      .catch(() => {})

    function onSettingsChanged() {
      GetSettings().then(s => setSettings(s)).catch(() => {})
    }
    window.addEventListener('settings:changed', onSettingsChanged)
    return () => window.removeEventListener('settings:changed', onSettingsChanged)
  }, [])

  // Subscribe to Twitch chat messages
  useEffect(() => {
    const off = EventsOn('chat:message', evt => {
      const id = `${evt.messageId || evt.chatterUserId}-${Date.now()}`
      const msg = { id, username: evt.chatterUserName, text: evt.message?.text || '', ts: Date.now() }
      setChatFeed(prev => {
        const next = [...prev, msg]
        return next.length > MAX_CHAT ? next.slice(next.length - MAX_CHAT) : next
      })
    })
    return () => off()
  }, [])

  // Auto-scroll chat feed
  useEffect(() => {
    if (chatRef.current) chatRef.current.scrollTop = chatRef.current.scrollHeight
  }, [chatFeed])

  // Auto-scroll co-host log
  useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight
  }, [coHostLog])

  // Auto-respond to new chat messages
  useEffect(() => {
    if (!autoRespond || chatFeed.length === 0) return
    const latest = chatFeed[chatFeed.length - 1]
    if (!latest || respondedIds.current.has(latest.id)) return
    respondedIds.current.add(latest.id)

    // Don't block the UI for auto-responses
    handleRespond(latest.username, latest.text, latest.id, /* silent */ true)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chatFeed, autoRespond])

  function stopCurrentAudio() {
    if (audioRef.current) {
      audioRef.current.pause()
      try { URL.revokeObjectURL(audioRef.current.src) } catch (_) {}
      audioRef.current = null
    }
  }

  function playAudio(b64) {
    stopCurrentAudio()
    const audio = b64ToAudio(b64)
    audioRef.current = audio
    audio.onended = () => { audioRef.current = null; try { URL.revokeObjectURL(audio.src) } catch (_) {} }
    audio.onerror = () => { audioRef.current = null; try { URL.revokeObjectURL(audio.src) } catch (_) {} }
    audio.play().catch(() => {})
  }

  function addToLog(username, chatText, coHostText, audioB64, activeGame) {
    setCoHostLog(prev => {
      const entry = { id: Date.now() + Math.random(), username, chatText, coHostText, audioB64, activeGame, ts: Date.now() }
      const next = [...prev, entry]
      return next.length > MAX_LOG ? next.slice(next.length - MAX_LOG) : next
    })
  }

  async function handleRespond(username, text, msgId, silent = false) {
    if (!silent) setBusy(true)
    setError('')
    try {
      const result = await CoHostRespondToChat(text, username)
      addToLog(username, text, result.text, result.audioB64 || '', result.activeGame || '')
      if (result.audioB64) playAudio(result.audioB64)
    } catch (e) {
      if (!silent) setError(String(e))
    } finally {
      if (!silent) setBusy(false)
    }
  }

  async function handleSpeak() {
    const trimmed = speakText.trim()
    if (!trimmed) return
    setSpeakBusy(true)
    setError('')
    try {
      const b64 = await CoHostSpeakText(trimmed)
      addToLog('(you typed)', trimmed, trimmed, b64, '')
      if (b64) playAudio(b64)
      setSpeakText('')
    } catch (e) {
      setError(String(e))
    } finally {
      setSpeakBusy(false)
    }
  }

  async function handleClearSession() {
    try { await ClearCoHostSession() } catch (_) {}
    setCoHostLog([])
    setNotice('Co-host session cleared.')
    setTimeout(() => setNotice(''), 3000)
  }

  function handleSpeakKeyDown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSpeak()
    }
  }

  const configured = settings && (settings.elevenLabsApiKey || settings.openAIApiKey)
  const coHostName = (settings?.coHostName?.trim()) || 'Spark'

  return (
    <>
      <h1 className="page-title">AI Co-Host</h1>

      {notice && <div className="notice success">{notice}</div>}
      {error  && <div className="notice error">{error}</div>}

      {!configured && (
        <div className="notice" style={{ background: 'var(--surface2)', border: '1px solid var(--border)', borderRadius: 8, padding: '12px 16px', marginBottom: 16 }}>
          Configure an <strong>ElevenLabs API key</strong> (for the premium co-host voice) or an
          {' '}<strong>OpenAI API key</strong> (fallback TTS) in{' '}
          <a href="#" onClick={e => { e.preventDefault(); window.location.hash = '/settings' }}
            style={{ color: 'var(--accent)' }}>Settings</a>.
        </div>
      )}

      {/* Status bar */}
      <div className="card" style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 16, flexWrap: 'wrap' }}>
          <div style={{ flex: 1 }}>
            <span style={{ fontWeight: 600, color: 'var(--text)' }}>{coHostName}</span>
            {settings?.coHostPersonality && (
              <span style={{ fontSize: 12, color: 'var(--text-muted)', marginLeft: 8 }}>
                {settings.coHostPersonality}
              </span>
            )}
            <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 2 }}>
              Voice: {settings?.elevenLabsApiKey && settings?.elevenLabsVoiceId
                ? 'ElevenLabs ✓'
                : settings?.openAIApiKey
                  ? 'OpenAI TTS (fallback)'
                  : 'No voice configured'}
            </div>
          </div>

          <label className="toggle-wrapper" title="Auto-respond to every chat message">
            <input type="checkbox" checked={autoRespond} onChange={e => setAutoRespond(e.target.checked)} />
            <span className="toggle-track" />
          </label>
          <span style={{ fontSize: 13, color: autoRespond ? 'var(--accent)' : 'var(--text-muted)' }}>
            Auto-respond
          </span>

          <button className="btn btn-secondary btn-sm" onClick={handleClearSession}>
            New Session
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, alignItems: 'start' }}>

        {/* Live chat feed */}
        <div className="card" style={{ display: 'flex', flexDirection: 'column', maxHeight: 480 }}>
          <div className="card-title" style={{ marginBottom: 8 }}>Live Chat</div>
          <div
            ref={chatRef}
            style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 6 }}
          >
            {chatFeed.length === 0 && (
              <div style={{ color: 'var(--text-muted)', fontSize: 13, padding: '8px 0' }}>
                Waiting for chat messages…
              </div>
            )}
            {chatFeed.map(msg => (
              <div key={msg.id}
                style={{
                  display: 'flex', alignItems: 'flex-start', gap: 8,
                  padding: '6px 8px', borderRadius: 6,
                  background: 'var(--surface2)',
                }}
              >
                <div style={{ flex: 1, minWidth: 0 }}>
                  <span style={{ fontWeight: 600, color: 'var(--accent)', fontSize: 13 }}>
                    {msg.username}
                  </span>
                  <span style={{ color: 'var(--text)', fontSize: 13, marginLeft: 6, wordBreak: 'break-word' }}>
                    {msg.text}
                  </span>
                </div>
                <button
                  className="btn btn-secondary btn-sm"
                  style={{ flexShrink: 0, fontSize: 11, padding: '2px 8px' }}
                  disabled={busy}
                  onClick={() => handleRespond(msg.username, msg.text, msg.id)}
                  title="Have the co-host respond to this message"
                >
                  Respond
                </button>
              </div>
            ))}
          </div>
        </div>

        {/* Co-host log */}
        <div className="card" style={{ display: 'flex', flexDirection: 'column', maxHeight: 480 }}>
          <div className="card-title" style={{ marginBottom: 8 }}>
            {coHostName} said…
          </div>
          <div
            ref={logRef}
            style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 8 }}
          >
            {coHostLog.length === 0 && (
              <div style={{ color: 'var(--text-muted)', fontSize: 13, padding: '8px 0' }}>
                Nothing yet — pick a chat message to respond to, or type something below.
              </div>
            )}
            {coHostLog.map(entry => (
              <div key={entry.id}
                style={{
                  padding: '8px 10px', borderRadius: 6,
                  background: 'var(--surface2)',
                  borderLeft: '3px solid var(--accent)',
                }}
              >
                {entry.username !== '(you typed)' && (
                  <div style={{ fontSize: 11, color: 'var(--text-muted)', marginBottom: 3 }}>
                    ↩ replying to <strong>{entry.username}</strong>: {entry.chatText}
                  </div>
                )}
                <div style={{ fontSize: 14, color: 'var(--text)' }}>{entry.coHostText}</div>
                {entry.activeGame && (
                  <div style={{ fontSize: 11, color: 'var(--text-muted)', marginTop: 4 }}>
                    🎮 {entry.activeGame}
                  </div>
                )}
                {entry.audioB64 && (
                  <button
                    className="btn btn-secondary btn-sm"
                    style={{ marginTop: 6, fontSize: 11 }}
                    onClick={() => playAudio(entry.audioB64)}
                    title="Replay this audio"
                  >
                    ▶ Replay
                  </button>
                )}
              </div>
            ))}
            {busy && (
              <div style={{ color: 'var(--text-muted)', fontSize: 13, fontStyle: 'italic' }}>
                {coHostName} is thinking…
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Manual speak */}
      <div className="card" style={{ marginTop: 16 }}>
        <div className="card-title">Say Something</div>
        <p className="setting-desc">
          Type what you want {coHostName} to say — perfect for when you can't speak aloud.
          The co-host will voice it in real-time.
        </p>
        <div style={{ display: 'flex', gap: 8 }}>
          <textarea
            className="text-input"
            rows={2}
            placeholder={`What should ${coHostName} say?`}
            value={speakText}
            onChange={e => setSpeakText(e.target.value)}
            onKeyDown={handleSpeakKeyDown}
            style={{ flex: 1, resize: 'vertical' }}
          />
          <button
            className="btn btn-primary"
            style={{ alignSelf: 'flex-end' }}
            disabled={speakBusy || !speakText.trim()}
            onClick={handleSpeak}
          >
            {speakBusy ? 'Speaking…' : `Speak as ${coHostName}`}
          </button>
        </div>
        <div style={{ fontSize: 11, color: 'var(--text-muted)', marginTop: 6 }}>
          Press Enter to speak (Shift+Enter for new line).
        </div>
      </div>
    </>
  )
}

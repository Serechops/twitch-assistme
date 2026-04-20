import { useState } from 'react'
import {
  SendAnnouncement,
  SendShoutout,
  CreateStreamMarker,
  SearchRaidTargets,
} from '../../wailsjs/go/main/App'

const ANNOUNCEMENT_COLORS = [
  { value: 'primary', label: 'Default' },
  { value: 'blue',    label: 'Blue' },
  { value: 'green',   label: 'Green' },
  { value: 'orange',  label: 'Orange' },
  { value: 'purple',  label: 'Purple' },
]

// ── Announcement Section ──────────────────────────────────────────────────────

function AnnouncementSection() {
  const [message, setMessage]   = useState('')
  const [color, setColor]       = useState('primary')
  const [busy, setBusy]         = useState(false)
  const [error, setError]       = useState('')
  const [success, setSuccess]   = useState(false)

  async function handleSend() {
    setError('')
    setSuccess(false)
    if (!message.trim()) { setError('Message is required.'); return }
    setBusy(true)
    try {
      await SendAnnouncement(message.trim(), color)
      setMessage('')
      setColor('primary')
      setSuccess(true)
      setTimeout(() => setSuccess(false), 3000)
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="card">
      <div className="card-title">Chat Announcement</div>
      <p className="setting-desc">
        Post a highlighted message in your chat. Viewers will see it styled by colour.
      </p>
      {error   && <div className="notice error">{error}</div>}
      {success && <div className="notice success">Announcement sent!</div>}
      <div className="settings-group">
        <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
          <div className="setting-label">Message <span className="text-muted">(max 500 chars)</span></div>
          <textarea
            className="text-input"
            rows={3}
            maxLength={500}
            placeholder="Hello chat! 👋"
            value={message}
            onChange={e => setMessage(e.target.value)}
            style={{ resize: 'vertical' }}
          />
          <div className="char-count" style={{ alignSelf: 'flex-end', fontSize: 12, color: 'var(--text-muted)' }}>
            {message.length}/500
          </div>
        </div>
        <div className="setting-row">
          <div>
            <div className="setting-label">Colour</div>
          </div>
          <select
            className="text-input"
            style={{ width: 'auto' }}
            value={color}
            onChange={e => setColor(e.target.value)}
          >
            {ANNOUNCEMENT_COLORS.map(c => (
              <option key={c.value} value={c.value}>{c.label}</option>
            ))}
          </select>
        </div>
      </div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 14 }}>
        <button className="btn btn-primary" onClick={handleSend} disabled={busy}>
          {busy ? 'Sending…' : 'Send Announcement'}
        </button>
      </div>
    </div>
  )
}

// ── Shoutout Section ─────────────────────────────────────────────────────────

function ShoutoutSection() {
  const [login, setLogin]         = useState('')
  const [suggestions, setSuggestions] = useState([])
  const [searchBusy, setSearchBusy]   = useState(false)
  const [busy, setBusy]           = useState(false)
  const [error, setError]         = useState('')
  const [success, setSuccess]     = useState('')

  let searchTimer = null
  function handleLoginChange(val) {
    setLogin(val)
    clearTimeout(searchTimer)
    setSuggestions([])
    if (val.trim().length < 2) return
    searchTimer = setTimeout(async () => {
      setSearchBusy(true)
      try {
        const results = await SearchRaidTargets(val.trim())
        setSuggestions((results || []).slice(0, 5))
      } catch {
        setSuggestions([])
      } finally {
        setSearchBusy(false)
      }
    }, 400)
  }

  async function handleSend() {
    setError('')
    setSuccess('')
    if (!login.trim()) { setError('Channel login is required.'); return }
    setBusy(true)
    try {
      await SendShoutout(login.trim())
      setLogin('')
      setSuggestions([])
      setSuccess(`Shoutout sent to ${login.trim()}!`)
      setTimeout(() => setSuccess(''), 4000)
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="card">
      <div className="card-title">Send Shoutout</div>
      <p className="setting-desc">
        Give a shoutout to another channel. Twitch will post a shoutout card in your chat.
      </p>
      {error   && <div className="notice error">{error}</div>}
      {success && <div className="notice success">{success}</div>}
      <div className="settings-group">
        <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6, position: 'relative' }}>
          <div className="setting-label">Channel login</div>
          <input
            className="text-input"
            type="text"
            placeholder="streamer_login"
            value={login}
            onChange={e => handleLoginChange(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleSend()}
          />
          {searchBusy && (
            <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>Searching…</div>
          )}
          {suggestions.length > 0 && (
            <div className="shoutout-suggestions">
              {suggestions.map(ch => (
                <button
                  key={ch.id}
                  className="shoutout-suggestion-item"
                  onClick={() => { setLogin(ch.login); setSuggestions([]) }}
                >
                  {ch.displayName}
                  {ch.gameName && <span className="text-muted"> · {ch.gameName}</span>}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 14 }}>
        <button className="btn btn-primary" onClick={handleSend} disabled={busy}>
          {busy ? 'Sending…' : 'Send Shoutout'}
        </button>
      </div>
    </div>
  )
}

// ── Stream Marker Section ─────────────────────────────────────────────────────

function StreamMarkerSection() {
  const [description, setDescription] = useState('')
  const [busy, setBusy]               = useState(false)
  const [error, setError]             = useState('')
  const [success, setSuccess]         = useState(false)

  async function handleCreate() {
    setError('')
    setSuccess(false)
    setBusy(true)
    try {
      await CreateStreamMarker(description.trim())
      setDescription('')
      setSuccess(true)
      setTimeout(() => setSuccess(false), 3000)
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="card">
      <div className="card-title">Stream Marker</div>
      <p className="setting-desc">
        Drop a marker at the current point in your VOD for easy editing later.
        Only works while you are live with VOD recording enabled.
      </p>
      {error   && <div className="notice error">{error}</div>}
      {success && <div className="notice success">Marker created!</div>}
      <div className="settings-group">
        <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
          <div className="setting-label">Description <span className="text-muted">(optional, max 140 chars)</span></div>
          <input
            className="text-input"
            type="text"
            placeholder="e.g. Clutch moment"
            maxLength={140}
            value={description}
            onChange={e => setDescription(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleCreate()}
          />
        </div>
      </div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 14 }}>
        <button className="btn btn-primary" onClick={handleCreate} disabled={busy}>
          {busy ? 'Creating…' : 'Create Marker'}
        </button>
      </div>
    </div>
  )
}

// ── Page ─────────────────────────────────────────────────────────────────────

export default function Tools() {
  return (
    <>
      <h1 className="page-title">Tools</h1>
      <AnnouncementSection />
      <ShoutoutSection />
      <StreamMarkerSection />
    </>
  )
}

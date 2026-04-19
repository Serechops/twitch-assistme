import { useEffect, useRef, useState } from 'react'
import { Login, Logout, ConnectEventSub, DisconnectEventSub, GetConnectionStatus, StartLogin, PollLogin, CreatePoll, EndPoll } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import useConnectionStatus from '../hooks/useConnectionStatus'

const MAX_MESSAGES = 100

export default function Dashboard({ user, setUser }) {
  const status = useConnectionStatus()
  const [messages, setMessages] = useState([])
  const [loginError, setLoginError] = useState('')
  const [busy, setBusy] = useState(false)
  const [deviceCode, setDeviceCode] = useState('')  // non-empty = waiting for device activation
  const feedRef = useRef(null)

  // Poll state
  const [pollTitle, setPollTitle] = useState('')
  const [pollChoices, setPollChoices] = useState(['', ''])
  const POLL_DURATIONS = [
    { label: '30s', seconds: 30 },
    { label: '1m',  seconds: 60 },
    { label: '2m',  seconds: 120 },
    { label: '5m',  seconds: 300 },
    { label: '10m', seconds: 600 },
    { label: '15m', seconds: 900 },
    { label: '30m', seconds: 1800 },
  ]
  const [pollDuration, setPollDuration] = useState(120)
  const [activePoll, setActivePoll] = useState(null)   // PollDTO or EventSub event
  const [pollBusy, setPollBusy] = useState(false)
  const [pollError, setPollError] = useState('')

  // Sync initial connection status
  useEffect(() => {
    GetConnectionStatus()
  }, [])

  // Subscribe to incoming chat messages
  useEffect(() => {
    const off = EventsOn('chat:message', evt => {
      setMessages(prev => {
        const next = [...prev, evt]
        return next.length > MAX_MESSAGES ? next.slice(next.length - MAX_MESSAGES) : next
      })
    })
    return () => off()
  }, [])

  // Subscribe to poll events from EventSub
  useEffect(() => {
    const offBegin = EventsOn('poll:begin', evt => {
      setActivePoll({ id: evt.id, title: evt.title, choices: evt.choices, endsAt: evt.ends_at, status: 'ACTIVE' })
    })
    const offProgress = EventsOn('poll:progress', evt => {
      setActivePoll(prev => prev ? { ...prev, choices: evt.choices } : prev)
    })
    const offEnd = EventsOn('poll:end', evt => {
      setActivePoll({ id: evt.id, title: evt.title, choices: evt.choices, endsAt: evt.ended_at, status: evt.status })
    })
    return () => { offBegin(); offProgress(); offEnd() }
  }, [])

  // Auto-scroll chat feed
  useEffect(() => {
    if (feedRef.current) {
      feedRef.current.scrollTop = feedRef.current.scrollHeight
    }
  }, [messages])

  async function handleLogin() {
    setBusy(true)
    setLoginError('')
    setDeviceCode('')
    try {
      const code = await StartLogin()
      if (code) {
        // Device Code flow — show the code, then poll in background
        setDeviceCode(code)
        setBusy(false)
        await PollLogin()
        setDeviceCode('')
      }
      // Auth Code flow returns empty string — browser already opened, done
    } catch (e) {
      setLoginError(String(e))
      setDeviceCode('')
    } finally {
      setBusy(false)
    }
  }

  async function handleLogout() {
    setBusy(true)
    try {
      await Logout()
    } finally {
      setBusy(false)
    }
  }

  async function handleConnect() {
    try {
      await ConnectEventSub()
    } catch (e) {
      console.error('ConnectEventSub:', e)
    }
  }

  function handleDisconnect() {
    DisconnectEventSub()
    setMessages([])
  }

  function addChoice() {
    if (pollChoices.length < 5) setPollChoices(prev => [...prev, ''])
  }

  function removeChoice(i) {
    if (pollChoices.length > 2) setPollChoices(prev => prev.filter((_, idx) => idx !== i))
  }

  function updateChoice(i, val) {
    setPollChoices(prev => prev.map((c, idx) => idx === i ? val : c))
  }

  async function handleCreatePoll() {
    setPollError('')
    const filledChoices = pollChoices.map(c => c.trim()).filter(Boolean)
    if (!pollTitle.trim()) { setPollError('Poll question is required.'); return }
    if (filledChoices.length < 2) { setPollError('At least 2 choices are required.'); return }
    setPollBusy(true)
    try {
      const poll = await CreatePoll(pollTitle.trim(), filledChoices, pollDuration)
      setActivePoll(poll)
      setPollTitle('')
      setPollChoices(['', ''])
      setPollDuration(120) // reset to 2 min default
    } catch (e) {
      setPollError(String(e))
    } finally {
      setPollBusy(false)
    }
  }

  async function handleEndPoll(showResults) {
    if (!activePoll?.id) return
    setPollBusy(true)
    try {
      const poll = await EndPoll(activePoll.id, showResults)
      setActivePoll(poll)
    } catch (e) {
      setPollError(String(e))
    } finally {
      setPollBusy(false)
    }
  }

  if (!user) {
    return (
      <div className="login-center">
        <h2>Welcome to Twitch AssistMe</h2>
        {deviceCode ? (
          <>
            <p>Your browser has been opened to <strong>twitch.tv/activate</strong>.</p>
            <p>Enter this code to connect your account:</p>
            <div className="device-code">{deviceCode}</div>
            <p className="login-hint">Waiting for authorization…</p>
          </>
        ) : (
          <>
            <p>Connect your Twitch account to start receiving chat notifications.</p>
            {loginError && <div className="notice error">{loginError}</div>}
            <button className="btn btn-primary" onClick={handleLogin} disabled={busy}>
              {busy ? 'Opening browser…' : 'Log in with Twitch'}
            </button>
          </>
        )}
      </div>
    )
  }

  return (
    <>
      <h1 className="page-title">Dashboard</h1>

      {/* User info */}
      <div className="card">
        <div className="card-title">Account</div>
        <div className="user-bar">
          {user.profileImageUrl
            ? <img className="user-bar-avatar" src={user.profileImageUrl} alt={user.displayName} />
            : <div className="user-bar-avatar user-bar-avatar--fallback">
                {user.displayName?.charAt(0).toUpperCase()}
              </div>
          }
          <div>
            <div className="user-bar-name">{user.displayName}</div>
            <div className="user-bar-login">@{user.login}</div>
          </div>
          <button className="btn btn-secondary btn-sm" style={{ marginLeft: 'auto' }}
                  onClick={handleLogout} disabled={busy}>
            Log out
          </button>
        </div>
      </div>

      {/* Connection */}
      <div className="card">
        <div className="card-title">Chat Notifications</div>
        <div className="connect-row">
          <span className={`status-badge ${status}`}>{status}</span>
          {status === 'disconnected' ? (
            <button className="btn btn-primary" onClick={handleConnect}>
              Connect
            </button>
          ) : (
            <button className="btn btn-danger btn-sm" onClick={handleDisconnect}>
              Disconnect
            </button>
          )}
        </div>
      </div>

      {/* Chat feed */}
      <div className="card">
        <div className="card-title">Live Chat</div>
        <div className="chat-feed" ref={feedRef}>
          {messages.length === 0
            ? <span className="chat-empty">No messages yet — connect to see chat.</span>
            : messages.map((m, i) => (
                <div className="chat-message" key={i}>
                  <span className="chat-chatter">{m.chatter_user_name}:</span>
                  <span className="chat-text">{m.message?.text}</span>
                </div>
              ))
          }
        </div>
      </div>

      {/* Active poll */}
      {activePoll && (
        <div className="card">
          <div className="card-title" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span>Active Poll</span>
            <span className={`poll-status-badge poll-status-${activePoll.status?.toLowerCase()}`}>
              {activePoll.status}
            </span>
          </div>
          <div className="poll-title">{activePoll.title}</div>
          <div className="poll-choices">
            {(activePoll.choices || []).map((c, i) => {
              const totalVotes = (activePoll.choices || []).reduce((sum, ch) => sum + (ch.votes || 0), 0)
              const pct = totalVotes > 0 ? Math.round(((c.votes || 0) / totalVotes) * 100) : 0
              return (
                <div className="poll-choice" key={i}>
                  <div className="poll-choice-header">
                    <span className="poll-choice-title">{c.title}</span>
                    <span className="poll-choice-votes">{c.votes || 0} votes ({pct}%)</span>
                  </div>
                  <div className="poll-bar-track">
                    <div className="poll-bar-fill" style={{ width: `${pct}%` }} />
                  </div>
                </div>
              )
            })}
          </div>
          {activePoll.status === 'ACTIVE' && (
            <div className="poll-actions">
              <button className="btn btn-primary btn-sm" onClick={() => handleEndPoll(true)} disabled={pollBusy}>
                End &amp; Show Results
              </button>
              <button className="btn btn-secondary btn-sm" onClick={() => handleEndPoll(false)} disabled={pollBusy}>
                End &amp; Archive
              </button>
            </div>
          )}
          {activePoll.status !== 'ACTIVE' && (
            <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 10 }}>
              <button className="btn btn-secondary btn-sm" onClick={() => setActivePoll(null)}>Clear Poll</button>
            </div>
          )}
        </div>
      )}

      {/* Poll creator */}
      <div className="card">
        <div className="card-title">Create Poll</div>
        {pollError && <div className="notice error">{pollError}</div>}
        <div className="settings-group">
          <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
            <div className="setting-label">Question</div>
            <input
              className="text-input"
              type="text"
              placeholder="Ask your viewers something…"
              maxLength={60}
              value={pollTitle}
              onChange={e => setPollTitle(e.target.value)}
            />
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div className="setting-label">Choices ({pollChoices.length}/5)</div>
            {pollChoices.map((c, i) => (
              <div key={i} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                <input
                  className="text-input"
                  type="text"
                  placeholder={`Choice ${i + 1}`}
                  maxLength={25}
                  value={c}
                  onChange={e => updateChoice(i, e.target.value)}
                />
                {pollChoices.length > 2 && (
                  <button className="btn btn-danger btn-sm" onClick={() => removeChoice(i)} title="Remove">✕</button>
                )}
              </div>
            ))}
            {pollChoices.length < 5 && (
              <button className="btn btn-secondary btn-sm" style={{ alignSelf: 'flex-start' }} onClick={addChoice}>
                + Add choice
              </button>
            )}
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div className="setting-label">Duration</div>
            <div className="poll-duration-presets">
              {POLL_DURATIONS.map(({ label, seconds }) => (
                <button
                  key={seconds}
                  className={`btn btn-sm poll-duration-btn${pollDuration === seconds ? ' poll-duration-btn--active' : ''}`}
                  onClick={() => setPollDuration(seconds)}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
        </div>
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 14 }}>
          <button className="btn btn-primary" onClick={handleCreatePoll} disabled={pollBusy || activePoll?.status === 'ACTIVE'}>
            {pollBusy ? 'Creating…' : activePoll?.status === 'ACTIVE' ? 'Poll already active' : 'Start Poll'}
          </button>
        </div>
      </div>
    </>
  )
}

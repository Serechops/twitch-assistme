import { useEffect, useRef, useState } from 'react'
import { Login, Logout, ConnectEventSub, DisconnectEventSub, GetConnectionStatus, StartLogin, PollLogin } from '../../wailsjs/go/main/App'
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
    </>
  )
}

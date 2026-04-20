import { Routes, Route, NavLink } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { GetUser } from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import Dashboard from './pages/Dashboard'
import Settings from './pages/Settings'
import Polls from './pages/Polls'
import Raids from './pages/Raids'
import StreamInfo from './pages/StreamInfo'
import Rewards from './pages/Rewards'
import Predictions from './pages/Predictions'
import Tools from './pages/Tools'
import useChatNotification from './hooks/useChatNotification'
import useVoiceCommand from './hooks/useVoiceCommand'
import VoiceCommandOverlay from './components/VoiceCommandOverlay'
import appLogo from './assets/images/twitch_assistme.png'

export default function App() {
  const [user, setUser] = useState(null)

  useChatNotification()
  const { voiceState, result, error, dismiss, stopAndSubmit, silenceProgress } = useVoiceCommand()

  useEffect(() => {
    GetUser().then(u => setUser(u))
    const off = EventsOn('auth:changed', u => setUser(u))
    return () => off()
  }, [])

  return (
    <div className="app-shell">
      <nav className="sidebar">
        <div className="sidebar-logo">
          <img src={appLogo} alt="Twitch AssistMe" className="logo-img" />
        </div>

        <div className="sidebar-links">
          <NavLink to="/" end className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
            Dashboard
          </NavLink>
          <NavLink to="/polls" className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
            Polls
          </NavLink>
          <NavLink to="/predictions" className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
            Predictions
          </NavLink>
          <NavLink to="/raids" className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
            Raids
          </NavLink>
          <NavLink to="/tools" className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
            Tools
          </NavLink>
          <NavLink to="/stream-info" className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
            Stream Info
          </NavLink>
          <NavLink to="/rewards" className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
            Rewards
          </NavLink>
          <NavLink to="/settings" className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
            Settings
          </NavLink>
        </div>

        {user && (
          <div className="sidebar-user">
            {user.profileImageUrl
              ? <img className="sidebar-user-avatar" src={user.profileImageUrl} alt={user.displayName} />
              : <div className="sidebar-user-avatar sidebar-user-avatar--fallback">
                  {user.displayName?.charAt(0).toUpperCase()}
                </div>
            }
            <div className="sidebar-user-name">@{user.login}</div>
          </div>
        )}

        <div className="sidebar-ai-hint" title="Press Ctrl+Shift+Space to give a voice command">
          🎙️ <span>AI Voice</span>
          <kbd>Ctrl+Shift+Space</kbd>
        </div>
      </nav>

      <main className="main-content">
        <Routes>
          <Route path="/" element={
            <Dashboard user={user} setUser={setUser} />
          } />
          <Route path="/polls" element={<Polls />} />
          <Route path="/predictions" element={<Predictions />} />
          <Route path="/raids" element={<Raids />} />
          <Route path="/tools" element={<Tools />} />
          <Route path="/stream-info" element={<StreamInfo />} />
          <Route path="/rewards" element={<Rewards />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </main>

      <VoiceCommandOverlay
        voiceState={voiceState}
        result={result}
        error={error}
        dismiss={dismiss}
        stopAndSubmit={stopAndSubmit}
        silenceProgress={silenceProgress}
      />
    </div>
  )
}


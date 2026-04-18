import { Routes, Route, NavLink } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { GetUser } from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import Dashboard from './pages/Dashboard'
import Settings from './pages/Settings'
import useChatNotification from './hooks/useChatNotification'

export default function App() {
  const [user, setUser] = useState(null)

  useChatNotification()

  useEffect(() => {
    GetUser().then(u => setUser(u))
    const off = EventsOn('auth:changed', u => setUser(u))
    return () => off()
  }, [])

  return (
    <div className="app-shell">
      <nav className="sidebar">
        <div className="sidebar-logo">
          <span className="logo-icon">🎮</span>
          <span className="logo-text">Streamer Tools</span>
        </div>

        <div className="sidebar-links">
          <NavLink to="/" end className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
            Dashboard
          </NavLink>
          <NavLink to="/settings" className={({ isActive }) => isActive ? 'nav-link active' : 'nav-link'}>
            Settings
          </NavLink>
        </div>

        {user && (
          <div className="sidebar-user">
            <div className="sidebar-user-name">@{user.login}</div>
          </div>
        )}
      </nav>

      <main className="main-content">
        <Routes>
          <Route path="/" element={<Dashboard user={user} setUser={setUser} />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </main>
    </div>
  )
}

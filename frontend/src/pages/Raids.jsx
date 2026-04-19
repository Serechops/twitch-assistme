import { useState, useCallback, useEffect } from 'react'
import {
  GetFollowedLiveChannels,
  GetSameCategoryChannels,
  SearchRaidTargets,
  StartRaid,
  CancelRaid,
} from '../../wailsjs/go/main/App'

// ─── Uptime formatter ────────────────────────────────────────────────────────

function formatUptime(startedAt) {
  if (!startedAt) return null
  const ms = Date.now() - new Date(startedAt).getTime()
  if (ms < 0) return null
  const h = Math.floor(ms / 3600000)
  const m = Math.floor((ms % 3600000) / 60000)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}

// ─── Viewer count badge ───────────────────────────────────────────────────────

function ViewerBadge({ count }) {
  const fmt = n =>
    n >= 1000 ? (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k' : String(n)
  return <span className="viewer-badge">{fmt(count)} viewers</span>
}

// ─── Single channel card ──────────────────────────────────────────────────────

function ChannelCard({ channel, onRaid, busy }) {
  const initials = channel.displayName?.charAt(0).toUpperCase() ?? '?'
  const uptime = formatUptime(channel.startedAt)

  return (
    <div className="raid-card">
      {channel.thumbnailURL && (
        <img
          className="raid-card-thumb"
          src={channel.thumbnailURL}
          alt={`${channel.displayName}'s stream`}
          onError={e => { e.currentTarget.style.display = 'none' }}
        />
      )}
      {channel.avatarURL
        ? <img className="raid-card-avatar raid-card-avatar--img" src={channel.avatarURL} alt={channel.displayName} />
        : <div className="raid-card-avatar">{initials}</div>
      }
      <div className="raid-card-info">
        <div className="raid-card-name">{channel.displayName}</div>
        <div className="raid-card-title" title={channel.title}>{channel.title || '—'}</div>
        <div className="raid-card-meta">
          {channel.gameName && <span className="raid-card-game">{channel.gameName}</span>}
          {channel.viewerCount > 0 && <ViewerBadge count={channel.viewerCount} />}
          {uptime && <span className="raid-card-uptime">{uptime}</span>}
        </div>
        {channel.tags?.length > 0 && (
          <div className="raid-card-tags">
            {channel.tags.slice(0, 4).map(tag => (
              <span key={tag} className="raid-tag">{tag}</span>
            ))}
          </div>
        )}
      </div>
      <button
        className="btn btn-accent raid-card-btn"
        onClick={() => onRaid(channel)}
        disabled={busy}
      >
        Raid
      </button>
    </div>
  )
}

// ─── Empty state ──────────────────────────────────────────────────────────────

function EmptyState({ message }) {
  return <p className="raids-empty">{message}</p>
}

// ─── Main page ────────────────────────────────────────────────────────────────

const TABS = [
  { id: 'followed', label: 'Following' },
  { id: 'category', label: 'Same Category' },
  { id: 'search',   label: 'Search' },
]

export default function Raids() {
  const [tab, setTab]           = useState('followed')
  const [channels, setChannels] = useState([])
  const [loading, setLoading]   = useState(false)
  const [loadError, setLoadError] = useState('')
  const [searchQ, setSearchQ]   = useState('')

  // Raid action state
  const [raidBusy, setRaidBusy]       = useState(false)
  const [raidTarget, setRaidTarget]   = useState(null)   // pending raid target
  const [raidError, setRaidError]     = useState('')
  const [raidSuccess, setRaidSuccess] = useState('')

  // ── Load helpers ─────────────────────────────────────────────────────────

  const loadFollowed = useCallback(async () => {
    setLoading(true); setLoadError('')
    try {
      const data = await GetFollowedLiveChannels()
      setChannels(data || [])
    } catch (e) {
      setLoadError(String(e))
    } finally {
      setLoading(false)
    }
  }, [])

  const loadCategory = useCallback(async () => {
    setLoading(true); setLoadError('')
    try {
      const data = await GetSameCategoryChannels()
      setChannels(data || [])
    } catch (e) {
      setLoadError(String(e))
    } finally {
      setLoading(false)
    }
  }, [])

  const doSearch = useCallback(async () => {
    if (!searchQ.trim()) return
    setLoading(true); setLoadError('')
    try {
      const data = await SearchRaidTargets(searchQ.trim())
      setChannels(data || [])
    } catch (e) {
      setLoadError(String(e))
    } finally {
      setLoading(false)
    }
  }, [searchQ])

  // ── Load followed channels on initial mount ───────────────────────────

  useEffect(() => { loadFollowed() }, [loadFollowed])

  // ── Tab change ───────────────────────────────────────────────────────────

  const switchTab = (id) => {
    setTab(id)
    setChannels([])
    setLoadError('')
    if (id === 'followed') loadFollowed()
    if (id === 'category') loadCategory()
  }

  // ── Raid actions ─────────────────────────────────────────────────────────

  const handleRaid = async (channel) => {
    setRaidBusy(true); setRaidError(''); setRaidSuccess('')
    try {
      await StartRaid(channel.id)
      setRaidTarget(channel)
      setRaidSuccess(
        `Raid initiated! Twitch will show a countdown in your chat. Click "Raid Now" to send your viewers to ${channel.displayName}.`
      )
    } catch (e) {
      setRaidError(String(e))
    } finally {
      setRaidBusy(false)
    }
  }

  const handleCancel = async () => {
    setRaidBusy(true); setRaidError('')
    try {
      await CancelRaid()
      setRaidTarget(null)
      setRaidSuccess('')
    } catch (e) {
      setRaidError(String(e))
    } finally {
      setRaidBusy(false)
    }
  }

  // ── Render ───────────────────────────────────────────────────────────────

  return (
    <div className="page">
      <h1 className="page-title">Raid</h1>

      {/* Pending raid banner */}
      {raidTarget && (
        <div className="raid-pending-banner">
          <span>
            Raid countdown active → <strong>{raidTarget.displayName}</strong>. Click
            &ldquo;Raid Now&rdquo; in your chat or wait 90 s.
          </span>
          <button
            className="btn btn-danger btn-sm"
            onClick={handleCancel}
            disabled={raidBusy}
          >
            Cancel Raid
          </button>
        </div>
      )}

      {/* Success / error messages */}
      {raidSuccess && !raidTarget && (
        <p className="raid-msg raid-msg--success">{raidSuccess}</p>
      )}
      {raidError && <p className="raid-msg raid-msg--error">{raidError}</p>}

      {/* Tabs */}
      <div className="tabs">
        {TABS.map(t => (
          <button
            key={t.id}
            className={`tab-btn${tab === t.id ? ' tab-btn--active' : ''}`}
            onClick={() => switchTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {/* Search input (search tab only) */}
      {tab === 'search' && (
        <div className="raid-search-row">
          <input
            className="raid-search-input"
            type="text"
            placeholder="Channel or category name…"
            value={searchQ}
            onChange={e => setSearchQ(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && doSearch()}
          />
          <button
            className="btn btn-accent"
            onClick={doSearch}
            disabled={loading || !searchQ.trim()}
          >
            Search
          </button>
        </div>
      )}

      {/* Channel list */}
      <div className="raid-list">
        {loading && <p className="raids-empty">Loading…</p>}
        {!loading && loadError && <p className="raids-empty raids-empty--error">{loadError}</p>}
        {!loading && !loadError && channels.length === 0 && tab !== 'search' && (
          <EmptyState message={
            tab === 'followed'
              ? 'None of the channels you follow are currently live.'
              : 'No other live channels found in your current category.'
          } />
        )}
        {!loading && !loadError && channels.length === 0 && tab === 'search' && searchQ && (
          <EmptyState message="No live channels found matching that search." />
        )}
        {!loading && channels.map(ch => (
          <ChannelCard
            key={ch.id}
            channel={ch}
            onRaid={handleRaid}
            busy={raidBusy}
          />
        ))}
      </div>
    </div>
  )
}

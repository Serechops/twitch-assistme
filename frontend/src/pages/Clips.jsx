import { useState, useEffect, useCallback } from 'react'
import { CreateClip, GetClips, GetMyChannelInfo, OpenURL, SuggestClipTitle } from '../../wailsjs/go/main/App'

// ── Helpers ──────────────────────────────────────────────────────────────────

function formatDuration(seconds) {
  const s = Math.round(seconds)
  const m = Math.floor(s / 60)
  const rem = s % 60
  return m > 0 ? `${m}m ${rem}s` : `${rem}s`
}

function formatDate(iso) {
  if (!iso) return ''
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}

// ── Clip Card ─────────────────────────────────────────────────────────────────

function ClipCard({ clip }) {
  const handleEdit = () => OpenURL(clip.editUrl)
  const handleView = () => OpenURL(clip.url)

  return (
    <div className="clip-card">
      <div className="clip-thumb-wrap" onClick={handleView} title="Open on Twitch">
        {clip.thumbnailUrl
          ? <img className="clip-thumb" src={clip.thumbnailUrl} alt={clip.title} />
          : <div className="clip-thumb clip-thumb--placeholder">No Preview</div>
        }
        <span className="clip-duration">{formatDuration(clip.duration)}</span>
        {clip.isFeatured && <span className="clip-badge">Featured</span>}
      </div>
      <div className="clip-info">
        <div className="clip-title" title={clip.title}>{clip.title || '(untitled)'}</div>
        <div className="clip-meta">
          <span>{clip.viewCount.toLocaleString()} views</span>
          <span>{formatDate(clip.createdAt)}</span>
        </div>
        <div className="clip-actions">
          <button className="btn btn-sm" onClick={handleView}>Watch</button>
          <button className="btn btn-sm btn-outline" onClick={handleEdit}>Edit on Twitch</button>
        </div>
      </div>
    </div>
  )
}

// ── Create Tab ────────────────────────────────────────────────────────────────

function CreateTab() {
  const [hasDelay, setHasDelay] = useState(false)
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState(null) // { id, editUrl }
  const [error, setError] = useState('')
  const [suggestBusy, setSuggestBusy] = useState(false)
  const [suggestedTitle, setSuggestedTitle] = useState('')
  const [channelInfo, setChannelInfo] = useState(null)

  useEffect(() => {
    GetMyChannelInfo().then(setChannelInfo).catch(() => {})
  }, [])

  const handleCreate = async () => {
    setError('')
    setResult(null)
    setSuggestedTitle('')
    setBusy(true)
    try {
      const clip = await CreateClip(hasDelay)
      setResult(clip)
    } catch (e) {
      setError(e?.toString() ?? 'Failed to create clip')
    } finally {
      setBusy(false)
    }
  }

  const handleSuggestTitle = async () => {
    setSuggestBusy(true)
    setSuggestedTitle('')
    try {
      const title = await SuggestClipTitle(
        channelInfo?.title ?? '',
        channelInfo?.gameName ?? ''
      )
      setSuggestedTitle(title)
    } catch (e) {
      setError(e?.toString() ?? 'AI title suggestion failed')
    } finally {
      setSuggestBusy(false)
    }
  }

  return (
    <div className="clips-create">
      <div className="clips-create-card">
        <h3>Create a Clip</h3>
        <p className="clips-hint">
          Clips are captured from your live stream at the moment you press the button.
          They may take up to 15 seconds to process before they appear in your clip library.
        </p>

        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={hasDelay}
            onChange={e => setHasDelay(e.target.checked)}
          />
          Add stream delay buffer
          <span className="label-hint">(accounts for viewer delay)</span>
        </label>

        <button
          className="btn btn-primary clips-create-btn"
          onClick={handleCreate}
          disabled={busy}
        >
          {busy ? 'Creating…' : '🎬 Clip Now'}
        </button>

        {error && <div className="notice error">{error}</div>}

        {result && (
          <div className="clip-created-result">
            <div className="notice success">Clip created! ID: <code>{result.id}</code></div>

            <div className="clip-created-actions">
              <button className="btn btn-outline" onClick={() => OpenURL(result.editUrl)}>
                ✏️ Edit on Twitch
              </button>
            </div>

            <div className="clip-ai-suggest">
              <p>Want an AI-generated title suggestion based on your current stream?</p>
              <button
                className="btn btn-sm"
                onClick={handleSuggestTitle}
                disabled={suggestBusy}
              >
                {suggestBusy ? 'Thinking…' : '✨ Suggest Title with AI'}
              </button>
              {suggestedTitle && (
                <div className="clip-suggested-title">
                  <strong>Suggested:</strong> {suggestedTitle}
                  <button
                    className="btn btn-sm btn-outline"
                    style={{ marginLeft: '0.5rem' }}
                    onClick={() => OpenURL(result.editUrl)}
                  >
                    Use this title →
                  </button>
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      <div className="clips-voice-hint">
        <span>💡 Tip: You can also say</span>
        <em>"clip that"</em>
        <span>using the AI Voice command (Ctrl+Shift+Space).</span>
      </div>
    </div>
  )
}

// ── My Clips Tab ──────────────────────────────────────────────────────────────

function MyClipsTab() {
  const [clips, setClips] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await GetClips(20)
      setClips(data ?? [])
    } catch (e) {
      setError(e?.toString() ?? 'Failed to load clips')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  if (loading) return <div className="loading-state">Loading clips…</div>
  if (error) return <div className="notice error">{error}</div>

  if (clips.length === 0) {
    return (
      <div className="empty-state">
        <p>No clips found. Create one from the Create tab while you&apos;re live!</p>
      </div>
    )
  }

  return (
    <div>
      <div className="clips-grid">
        {clips.map(clip => <ClipCard key={clip.id} clip={clip} />)}
      </div>
      <div style={{ textAlign: 'center', marginTop: '1rem' }}>
        <button className="btn btn-sm btn-outline" onClick={load}>Refresh</button>
      </div>
    </div>
  )
}

// ── Main Component ────────────────────────────────────────────────────────────

export default function Clips() {
  const [tab, setTab] = useState('create')

  return (
    <div className="page-container">
      <div className="page-header">
        <h2>Clips</h2>
      </div>

      <div className="tab-bar">
        <button
          className={`tab-btn${tab === 'create' ? ' active' : ''}`}
          onClick={() => setTab('create')}
        >
          Create
        </button>
        <button
          className={`tab-btn${tab === 'library' ? ' active' : ''}`}
          onClick={() => setTab('library')}
        >
          My Clips
        </button>
      </div>

      <div className="tab-content">
        {tab === 'create' ? <CreateTab /> : <MyClipsTab />}
      </div>
    </div>
  )
}

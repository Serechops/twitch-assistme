import { useEffect, useState, useRef, useCallback } from 'react'
import { GetMyChannelInfo, UpdateChannelInfo, SearchCategories } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

export default function StreamInfo() {
  const [title, setTitle] = useState('')
  const [gameID, setGameID] = useState('')
  const [gameName, setGameName] = useState('')
  const [gameBoxArtURL, setGameBoxArtURL] = useState('')
  const [language, setLanguage] = useState('')
  const [tags, setTags] = useState([])
  const [tagInput, setTagInput] = useState('')
  const [tagError, setTagError] = useState('')

  const [categoryQuery, setCategoryQuery] = useState('')
  const [categoryResults, setCategoryResults] = useState([])
  const [showDropdown, setShowDropdown] = useState(false)
  const categorySearchTimeout = useRef(null)
  const dropdownRef = useRef(null)

  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [loading, setLoading] = useState(true)

  const applyChannelInfo = useCallback(info => {
    setTitle(info.title)
    setGameID(info.gameID)
    setGameName(info.gameName)
    setGameBoxArtURL(info.boxArtURL || '')
    setLanguage(info.language || '')
    setTags(info.tags || [])
  }, [])

  useEffect(() => {
    GetMyChannelInfo()
      .then(applyChannelInfo)
      .catch(e => setError(String(e)))
      .finally(() => setLoading(false))
  }, [applyChannelInfo])

  // Re-fetch whenever the AI voice command changes stream info.
  useEffect(() => {
    const off = EventsOn('streaminfo:changed', () => {
      GetMyChannelInfo()
        .then(applyChannelInfo)
        .catch(() => {})
    })
    return () => off()
  }, [applyChannelInfo])

  useEffect(() => {
    if (!categoryQuery.trim()) {
      setCategoryResults([])
      setShowDropdown(false)
      return
    }
    clearTimeout(categorySearchTimeout.current)
    categorySearchTimeout.current = setTimeout(() => {
      SearchCategories(categoryQuery.trim())
        .then(results => {
          setCategoryResults(results || [])
          setShowDropdown(true)
        })
        .catch(() => {})
    }, 300)
    return () => clearTimeout(categorySearchTimeout.current)
  }, [categoryQuery])

  useEffect(() => {
    function handleClick(e) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target)) {
        setShowDropdown(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  function selectCategory(cat) {
    setGameID(cat.id)
    setGameName(cat.name)
    setGameBoxArtURL(cat.boxArtURL || '')
    setCategoryQuery('')
    setCategoryResults([])
    setShowDropdown(false)
  }

  function clearCategory() {
    setGameID('')
    setGameName('')
    setGameBoxArtURL('')
    setCategoryQuery('')
  }

  function addTag() {
    const t = tagInput.trim()
    setTagError('')
    if (!t) return
    if (t.length > 25) { setTagError('Max 25 characters per tag'); return }
    if (/[^a-zA-Z0-9]/.test(t)) { setTagError('Tags may only contain letters and numbers'); return }
    if (tags.length >= 10) { setTagError('Maximum 10 tags allowed'); return }
    if (tags.map(x => x.toLowerCase()).includes(t.toLowerCase())) {
      setTagError('Tag already added'); return
    }
    setTags(prev => [...prev, t])
    setTagInput('')
  }

  function removeTag(idx) {
    setTags(prev => prev.filter((_, i) => i !== idx))
  }

  async function handleSave() {
    if (!title.trim()) { setError('Title cannot be empty'); return }
    setSaving(true)
    setError('')
    try {
      await UpdateChannelInfo(title.trim(), gameID, language, tags)
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="stream-info-page">
        <p className="polls-empty">Loading channel info...</p>
      </div>
    )
  }

  const artSrc = gameBoxArtURL
    ? gameBoxArtURL.replace('{width}', '188').replace('{height}', '250')
    : null

  return (
    <div className="stream-info-page">
      <div className="si-page-header">
        <h1 className="page-title" style={{ margin: 0 }}>Stream Information</h1>
        <p className="si-page-subtitle">Manage your live channel details</p>
      </div>

      {error && <div className="notice error">{error}</div>}

      {/* Live preview hero */}
      {(gameName || title) && (
        <div className="si-hero">
          {artSrc && (
            <img className="si-hero-art" src={artSrc} alt={gameName} />
          )}
          <div className="si-hero-meta">
            {gameName && <div className="si-hero-game">{gameName}</div>}
            <div className="si-hero-title">{title || <span style={{ opacity: 0.4 }}>No title set</span>}</div>
            {tags.length > 0 && (
              <div className="si-hero-tags">
                {tags.map((t, i) => (
                  <span key={i} className="si-tag si-tag--hero">{t}</span>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      <div className="card si-card">

        {/* Stream Title */}
        <div className="si-section">
          <div className="si-section-label">Stream Title</div>
          <div className="si-section-body">
            <input
              className="text-input si-title-input"
              type="text"
              value={title}
              maxLength={140}
              onChange={e => setTitle(e.target.value)}
              placeholder="Enter your stream title…"
            />
            <span className="si-char-count">{title.length}/140</span>
          </div>
        </div>

        <div className="si-divider" />

        {/* Category / Game */}
        <div className="si-section">
          <div className="si-section-label">Category / Game</div>
          <div className="si-section-body">
            {gameName ? (
              <div className="si-selected-category">
                {gameBoxArtURL && (
                  <img
                    className="si-selected-art"
                    src={gameBoxArtURL.replace('{width}', '60').replace('{height}', '80')}
                    alt={gameName}
                    onError={e => { e.currentTarget.style.display = 'none' }}
                  />
                )}
                <div className="si-selected-details">
                  <span className="si-category-name">{gameName}</span>
                  {gameID && <span className="si-category-id">ID {gameID}</span>}
                </div>
                <button className="btn btn-secondary btn-sm si-change-btn" onClick={clearCategory}>Change</button>
              </div>
            ) : (
              <div className="si-category-search" ref={dropdownRef}>
                <input
                  className="text-input"
                  type="text"
                  value={categoryQuery}
                  onChange={e => setCategoryQuery(e.target.value)}
                  placeholder="Search for a game or category…"
                />
                {showDropdown && categoryResults.length > 0 && (
                  <ul className="si-category-dropdown">
                    {categoryResults.map(cat => (
                      <li
                        key={cat.id}
                        className="si-category-item"
                        onMouseDown={() => selectCategory(cat)}
                      >
                        <img
                          src={cat.boxArtURL.replace('{width}', '40').replace('{height}', '53')}
                          alt={cat.name}
                          className="si-category-art"
                          onError={e => { e.currentTarget.style.display = 'none' }}
                        />
                        <span>{cat.name}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </div>
        </div>

        <div className="si-divider" />

        {/* Language */}
        <div className="si-section si-section--inline">
          <div className="si-section-label">
            Language
            <span className="si-section-hint">ISO 639-1 code, e.g. "en"</span>
          </div>
          <div className="si-section-body">
            <input
              className="text-input si-lang-input"
              type="text"
              value={language}
              maxLength={5}
              onChange={e => setLanguage(e.target.value.toLowerCase())}
              placeholder="en"
            />
          </div>
        </div>

        <div className="si-divider" />

        {/* Tags */}
        <div className="si-section">
          <div className="si-section-label">
            Tags
            <span className="si-section-hint">{tags.length}/10 · letters &amp; numbers only · 25 chars max</span>
          </div>
          <div className="si-section-body">
            {tags.length > 0 && (
              <div className="si-tags-row">
                {tags.map((t, i) => (
                  <span key={i} className="si-tag">
                    {t}
                    <button className="si-tag-remove" onClick={() => removeTag(i)} title="Remove tag">✕</button>
                  </span>
                ))}
              </div>
            )}
            {tags.length < 10 && (
              <div className="si-tag-input-row">
                <input
                  className="text-input"
                  style={{ flex: 1 }}
                  type="text"
                  value={tagInput}
                  maxLength={25}
                  onChange={e => { setTagInput(e.target.value); setTagError('') }}
                  onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); addTag() } }}
                  placeholder="Add a tag…"
                />
                <button className="btn btn-secondary btn-sm" onClick={addTag}>Add</button>
              </div>
            )}
            {tagError && <p className="si-field-error">{tagError}</p>}
          </div>
        </div>

        <div className="si-divider" />

        <div className="si-actions">
          <button
            className={`btn btn-primary${saved ? ' btn--saved' : ''}`}
            onClick={handleSave}
            disabled={saving || saved}
          >
            {saving ? 'Saving…' : saved ? '✓ Saved!' : 'Save Changes'}
          </button>
        </div>

      </div>
    </div>
  )
}

import { useEffect, useState, useRef } from 'react'
import { GetMyChannelInfo, UpdateChannelInfo, SearchCategories } from '../../wailsjs/go/main/App'

export default function StreamInfo() {
  const [title, setTitle] = useState('')
  const [gameID, setGameID] = useState('')
  const [gameName, setGameName] = useState('')
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

  useEffect(() => {
    GetMyChannelInfo()
      .then(info => {
        setTitle(info.title)
        setGameID(info.gameID)
        setGameName(info.gameName)
        setLanguage(info.language || '')
        setTags(info.tags || [])
      })
      .catch(e => setError(String(e)))
      .finally(() => setLoading(false))
  }, [])

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
    setCategoryQuery('')
    setCategoryResults([])
    setShowDropdown(false)
  }

  function clearCategory() {
    setGameID('')
    setGameName('')
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

  return (
    <div className="stream-info-page">
      <h1 className="page-title">Stream Information</h1>

      {error && <div className="notice error">{error}</div>}

      <div className="card">
        <div className="settings-group">

          <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
            <div className="setting-label">Stream Title</div>
            <input
              className="text-input"
              type="text"
              value={title}
              maxLength={140}
              onChange={e => setTitle(e.target.value)}
              placeholder="Enter your stream title"
            />
            <span style={{ fontSize: 11, color: 'var(--text-muted)', alignSelf: 'flex-end' }}>
              {title.length}/140
            </span>
          </div>

          <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
            <div className="setting-label">Category / Game</div>
            {gameName ? (
              <div className="si-selected-category">
                <span className="si-category-name">{gameName}</span>
                <button className="btn btn-secondary btn-sm" onClick={clearCategory}>x Change</button>
              </div>
            ) : (
              <div className="si-category-search" ref={dropdownRef}>
                <input
                  className="text-input"
                  type="text"
                  value={categoryQuery}
                  onChange={e => setCategoryQuery(e.target.value)}
                  placeholder="Search for a game or category..."
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

          <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
            <div className="setting-label">
              Language <span style={{ fontWeight: 400, color: 'var(--text-muted)', fontSize: 12 }}>(ISO 639-1, e.g. "en")</span>
            </div>
            <input
              className="text-input"
              type="text"
              value={language}
              maxLength={5}
              style={{ width: 160 }}
              onChange={e => setLanguage(e.target.value.toLowerCase())}
              placeholder="en"
            />
          </div>

          <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 8 }}>
            <div className="setting-label">
              Tags <span style={{ fontWeight: 400, color: 'var(--text-muted)', fontSize: 12 }}>({tags.length}/10 - letters and numbers only - 25 chars max)</span>
            </div>
            {tags.length > 0 && (
              <div className="si-tags-row">
                {tags.map((t, i) => (
                  <span key={i} className="si-tag">
                    {t}
                    <button className="si-tag-remove" onClick={() => removeTag(i)}>x</button>
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
                  placeholder="Add tag..."
                />
                <button className="btn btn-secondary btn-sm" onClick={addTag}>Add</button>
              </div>
            )}
            {tagError && <p className="polls-empty" style={{ color: 'var(--red)', margin: 0 }}>{tagError}</p>}
          </div>

        </div>

        <div className="poll-actions">
          <button
            className={`btn btn-primary${saved ? ' btn--saved' : ''}`}
            onClick={handleSave}
            disabled={saving || saved}
          >
            {saving ? 'Saving...' : saved ? 'Saved!' : 'Save Changes'}
          </button>
        </div>
      </div>
    </div>
  )
}

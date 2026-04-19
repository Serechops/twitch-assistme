import { useEffect, useState, useCallback, useRef } from 'react'
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

  const [status, setStatus] = useState(null) // { type: 'error', msg }
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
      .catch(e => setStatus({ type: 'error', msg: String(e) }))
      .finally(() => setLoading(false))
  }, [])

  // Category search with debounce
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

  // Close dropdown when clicking outside
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
    if (!title.trim()) { setStatus({ type: 'error', msg: 'Title cannot be empty' }); return }
    setSaving(true)
    setStatus(null)
    try {
      await UpdateChannelInfo(title.trim(), gameID, language, tags)
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    } catch (e) {
      setStatus({ type: 'error', msg: String(e) })
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return <div className="stream-info-page"><p className="si-loading">Loading channel info…</p></div>
  }

  return (
    <div className="stream-info-page">
      <h2 className="page-title">Stream Information</h2>

      <div className="si-form">
        {/* Title */}
        <div className="si-field">
          <label className="si-label">Stream Title</label>
          <input
            className="si-input"
            type="text"
            value={title}
            maxLength={140}
            onChange={e => setTitle(e.target.value)}
            placeholder="Enter your stream title"
          />
          <span className="si-char-count">{title.length}/140</span>
        </div>

        {/* Category */}
        <div className="si-field">
          <label className="si-label">Category / Game</label>
          {gameName ? (
            <div className="si-selected-category">
              <span className="si-category-name">{gameName}</span>
              <button className="si-clear-btn" onClick={clearCategory}>✕ Change</button>
            </div>
          ) : (
            <div className="si-category-search" ref={dropdownRef}>
              <input
                className="si-input"
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

        {/* Language */}
        <div className="si-field">
          <label className="si-label">Language <span className="si-hint">(ISO 639-1, e.g. "en")</span></label>
          <input
            className="si-input si-input--sm"
            type="text"
            value={language}
            maxLength={5}
            onChange={e => setLanguage(e.target.value.toLowerCase())}
            placeholder="en"
          />
        </div>

        {/* Tags */}
        <div className="si-field">
          <label className="si-label">Tags <span className="si-hint">({tags.length}/10 · letters &amp; numbers only · 25 chars max)</span></label>
          <div className="si-tags-row">
            {tags.map((t, i) => (
              <span key={i} className="si-tag">
                {t}
                <button className="si-tag-remove" onClick={() => removeTag(i)}>✕</button>
              </span>
            ))}
          </div>
          {tags.length < 10 && (
            <div className="si-tag-input-row">
              <input
                className="si-input si-input--sm"
                type="text"
                value={tagInput}
                maxLength={25}
                onChange={e => { setTagInput(e.target.value); setTagError('') }}
                onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); addTag() } }}
                placeholder="Add tag…"
              />
              <button className="btn-secondary si-tag-add-btn" onClick={addTag}>Add</button>
            </div>
          )}
          {tagError && <p className="si-error">{tagError}</p>}
        </div>

        {/* Save */}
        {status && (
          <div className={`si-status si-status--${status.type}`}>{status.msg}</div>
        )}
        <button
          className={`btn-primary si-save-btn${saved ? ' btn--saved' : ''}`}
          onClick={handleSave}
          disabled={saving || saved}
        >
          {saving ? 'Saving…' : saved ? '✓ Saved!' : 'Save Changes'}
        </button>
      </div>
    </div>
  )
}

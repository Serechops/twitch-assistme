import { useEffect, useState, useCallback } from 'react'
import {
  GetCustomRewards,
  CreateCustomReward,
  UpdateCustomReward,
  DeleteCustomReward,
  GetPendingRedemptions,
  FulfillRedemption,
  CancelRedemption,
  ToggleCustomRewardPaused,
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

// ── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso) {
  if (!iso) return '—'
  try { return new Date(iso).toLocaleString() } catch { return iso }
}

function hexToRgb(hex) {
  const h = (hex || '#9147ff').replace('#', '')
  const r = parseInt(h.slice(0, 2), 16)
  const g = parseInt(h.slice(2, 4), 16)
  const b = parseInt(h.slice(4, 6), 16)
  return `${r},${g},${b}`
}

// ── Starter Templates ───────────────────────────────────────────────────────

const REWARD_TEMPLATES = [
  {
    emoji: '💧',
    title: 'Hydrate!',
    cost: 1,
    prompt: 'Remind your streamer to drink some water!',
    isEnabled: true,
    backgroundColor: '#00897b',
    isUserInputRequired: false,
    shouldRedemptionsSkipRequestQueue: true,
    maxPerStreamEnabled: true,
    maxPerStream: 5,
    maxPerUserEnabled: false, maxPerUser: 0,
    cooldownEnabled: false, cooldownSeconds: 0,
    tags: ['Auto-fulfill', 'Max/stream'],
    desc: 'Viewers spend a single point to ping you to drink water. Capped at 5 per stream so it stays fun.',
  },
  {
    emoji: '🎵',
    title: 'Pick My Song',
    cost: 500,
    prompt: 'Suggest a song for me to play! Format: Artist – Title',
    isEnabled: true,
    backgroundColor: '#1565c0',
    isUserInputRequired: true,
    shouldRedemptionsSkipRequestQueue: false,
    maxPerStreamEnabled: false, maxPerStream: 0,
    maxPerUserEnabled: true, maxPerUser: 1,
    cooldownEnabled: false, cooldownSeconds: 0,
    tags: ['Input required', 'Max per user'],
    desc: 'Viewers request a song. Capped to 1 per viewer so the queue stays manageable. You review before accepting.',
  },
  {
    emoji: '🔊',
    title: 'Sound Alert',
    cost: 100,
    prompt: 'Trigger a sound alert on stream!',
    isEnabled: true,
    backgroundColor: '#e65100',
    isUserInputRequired: false,
    shouldRedemptionsSkipRequestQueue: true,
    maxPerStreamEnabled: false, maxPerStream: 0,
    maxPerUserEnabled: false, maxPerUser: 0,
    cooldownEnabled: true, cooldownSeconds: 60,
    tags: ['Auto-fulfill', 'Cooldown 60s'],
    desc: 'Fires a sound FX on stream. 60-second global cooldown prevents spam.',
  },
  {
    emoji: '💬',
    title: 'TTS Message',
    cost: 300,
    prompt: 'Type a message and I\'ll read it out loud on stream!',
    isEnabled: true,
    backgroundColor: '#00695c',
    isUserInputRequired: true,
    shouldRedemptionsSkipRequestQueue: false,
    maxPerStreamEnabled: false, maxPerStream: 0,
    maxPerUserEnabled: false, maxPerUser: 0,
    cooldownEnabled: true, cooldownSeconds: 30,
    tags: ['Input required', 'Cooldown 30s'],
    desc: 'Viewer submits text for you to read aloud. You manually fulfill each one from the queue.',
  },
  {
    emoji: '📷',
    title: 'Change Camera Angle',
    cost: 250,
    prompt: 'Make me switch to a different camera angle for 30 seconds!',
    isEnabled: true,
    backgroundColor: '#ad1457',
    isUserInputRequired: false,
    shouldRedemptionsSkipRequestQueue: true,
    maxPerStreamEnabled: false, maxPerStream: 0,
    maxPerUserEnabled: false, maxPerUser: 0,
    cooldownEnabled: true, cooldownSeconds: 120,
    tags: ['Auto-fulfill', 'Cooldown 2 min'],
    desc: 'Auto-fulfills instantly. 2-minute cooldown keeps your stream layout stable.',
  },
  {
    emoji: '🎮',
    title: 'Challenge Accepted',
    cost: 1000,
    prompt: 'Dare me to do something within reason! I\'ll decide if I accept.',
    isEnabled: true,
    backgroundColor: '#c62828',
    isUserInputRequired: true,
    shouldRedemptionsSkipRequestQueue: false,
    maxPerStreamEnabled: true, maxPerStream: 3,
    maxPerUserEnabled: false, maxPerUser: 0,
    cooldownEnabled: false, cooldownSeconds: 0,
    tags: ['Input required', 'Max/stream'],
    desc: 'High-cost dare with a 3-per-stream cap. You review submissions and can cancel ones you decline.',
  },
  {
    emoji: '⏭️',
    title: 'Skip Current Song',
    cost: 200,
    prompt: 'Skip the current song in the queue!',
    isEnabled: true,
    backgroundColor: '#4527a0',
    isUserInputRequired: false,
    shouldRedemptionsSkipRequestQueue: true,
    maxPerStreamEnabled: false, maxPerStream: 0,
    maxPerUserEnabled: false, maxPerUser: 0,
    cooldownEnabled: true, cooldownSeconds: 90,
    tags: ['Auto-fulfill', 'Cooldown 90s'],
    desc: 'Lets chat vote with their points to skip a song. 90-second cooldown prevents rapid-fire skipping.',
  },
  {
    emoji: '🕹️',
    title: 'Choose My Next Game',
    cost: 5000,
    prompt: 'Name a game for me to play next! (Must be in my library)',
    isEnabled: true,
    backgroundColor: '#283593',
    isUserInputRequired: true,
    shouldRedemptionsSkipRequestQueue: false,
    maxPerStreamEnabled: true, maxPerStream: 1,
    maxPerUserEnabled: false, maxPerUser: 0,
    cooldownEnabled: false, cooldownSeconds: 0,
    tags: ['Input required', 'Max/stream: 1'],
    desc: 'Only redeemable once per stream — whoever gets it first chooses your next game.',
  },
  {
    emoji: '😂',
    title: 'Timeout Yourself',
    cost: 500,
    prompt: 'Put yourself in the timeout chair for 60 seconds!',
    isEnabled: true,
    backgroundColor: '#37474f',
    isUserInputRequired: false,
    shouldRedemptionsSkipRequestQueue: true,
    maxPerStreamEnabled: false, maxPerStream: 0,
    maxPerUserEnabled: true, maxPerUser: 2,
    cooldownEnabled: false, cooldownSeconds: 0,
    tags: ['Auto-fulfill', 'Max per user: 2'],
    desc: 'Comedy reward — viewer "times themselves out". Limit 2 per viewer per stream.',
  },
  {
    emoji: '🌈',
    title: 'Random Emote Mode',
    cost: 750,
    prompt: 'Activate emote-only mode in chat for 60 seconds!',
    isEnabled: true,
    backgroundColor: '#6a1b9a',
    isUserInputRequired: false,
    shouldRedemptionsSkipRequestQueue: true,
    maxPerStreamEnabled: false, maxPerStream: 0,
    maxPerUserEnabled: false, maxPerUser: 0,
    cooldownEnabled: true, cooldownSeconds: 300,
    tags: ['Auto-fulfill', 'Cooldown 5 min'],
    desc: 'Triggers a fun emote-only period. 5-minute cooldown keeps chat readable between activations.',
  },
]

// ── Template Gallery ──────────────────────────────────────────────────────────

function TemplateGallery({ onUse }) {
  return (
    <div className="rw-tpl-gallery">
      <p className="rw-tpl-intro">
        Click <strong>Use</strong> on any template to pre-fill the create form.
        You can adjust the settings before saving.
      </p>
      <div className="rw-tpl-grid">
        {REWARD_TEMPLATES.map((tpl, i) => {
          const rgb = hexToRgb(tpl.backgroundColor)
          return (
            <div
              key={i}
              className="rw-tpl-card"
              style={{ '--rw-accent': tpl.backgroundColor, '--rw-accent-rgb': rgb }}
            >
              <div className="rw-tpl-card-accent" />
              <div className="rw-tpl-card-body">
                <div className="rw-tpl-card-header">
                  <span className="rw-tpl-emoji">{tpl.emoji}</span>
                  <div className="rw-tpl-title-group">
                    <span className="rw-tpl-name">{tpl.title}</span>
                    <span className="rw-tpl-cost">🎁 {tpl.cost.toLocaleString()}</span>
                  </div>
                </div>
                <p className="rw-tpl-desc">{tpl.desc}</p>
                <div className="rw-tpl-tags">
                  {tpl.tags.map(tag => (
                    <span key={tag} className="rw-tpl-tag">{tag}</span>
                  ))}
                </div>
                <button
                  className="btn btn--sm rw-tpl-use-btn"
                  onClick={() => onUse(tpl)}
                >
                  Use Template
                </button>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ── Default form state ────────────────────────────────────────────────────────

const EMPTY_FORM = {
  title: '',
  cost: 100,
  prompt: '',
  isEnabled: true,
  backgroundColor: '#9147ff',
  isUserInputRequired: false,
  shouldRedemptionsSkipRequestQueue: false,
  maxPerStreamEnabled: false,
  maxPerStream: 0,
  maxPerUserEnabled: false,
  maxPerUser: 0,
  cooldownEnabled: false,
  cooldownSeconds: 0,
}

// ── Reward Form ───────────────────────────────────────────────────────────────

function RewardForm({ initial, onSave, onCancel }) {
  const [form, setForm] = useState(initial || EMPTY_FORM)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')

  const set = (key, val) => setForm(f => ({ ...f, [key]: val }))

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!form.title.trim()) { setError('Title is required.'); return }
    if (form.cost < 1) { setError('Cost must be at least 1.'); return }
    setSaving(true)
    setError('')
    try {
      await onSave(form)
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    } catch (err) {
      setError(String(err))
    } finally {
      setSaving(false)
    }
  }

  const isEdit = !!(initial && initial.id)

  return (
    <form className="rw-form" onSubmit={handleSubmit}>
      <h3 className="rw-form-title">{isEdit ? 'Edit Reward' : 'Create Reward'}</h3>

      <div className="rw-form-row">
        <label>
          Title <span className="rw-required">*</span>
          <input
            type="text"
            value={form.title}
            maxLength={45}
            onChange={e => set('title', e.target.value)}
            placeholder="Reward title (max 45 chars)"
            required
          />
          <span className="rw-char-count">{form.title.length}/45</span>
        </label>
      </div>

      <div className="rw-form-row rw-form-row--half">
        <label>
          Cost (channel points) <span className="rw-required">*</span>
          <input
            type="number"
            value={form.cost}
            min={1}
            onChange={e => set('cost', parseInt(e.target.value, 10) || 1)}
          />
        </label>

        <label>
          Background Color
          <div className="rw-color-row">
            <input
              type="color"
              value={form.backgroundColor || '#9147ff'}
              onChange={e => set('backgroundColor', e.target.value)}
              className="rw-color-picker"
            />
            <input
              type="text"
              value={form.backgroundColor || '#9147ff'}
              maxLength={7}
              onChange={e => set('backgroundColor', e.target.value)}
              className="rw-color-text"
            />
          </div>
        </label>
      </div>

      <div className="rw-form-row">
        <label>
          Prompt (optional — shown to viewer)
          <textarea
            value={form.prompt}
            maxLength={200}
            rows={2}
            onChange={e => set('prompt', e.target.value)}
            placeholder="Description viewers see when redeeming (max 200 chars)"
          />
          <span className="rw-char-count">{form.prompt.length}/200</span>
        </label>
      </div>

      <div className="rw-form-toggles">
        <label className="rw-toggle-label">
          <input
            type="checkbox"
            checked={form.isEnabled}
            onChange={e => set('isEnabled', e.target.checked)}
          />
          Enabled
        </label>
        <label className="rw-toggle-label">
          <input
            type="checkbox"
            checked={form.isUserInputRequired}
            onChange={e => set('isUserInputRequired', e.target.checked)}
          />
          Require user input
        </label>
        <label className="rw-toggle-label">
          <input
            type="checkbox"
            checked={form.shouldRedemptionsSkipRequestQueue}
            onChange={e => set('shouldRedemptionsSkipRequestQueue', e.target.checked)}
          />
          Auto-fulfill (skip queue)
        </label>
      </div>

      {/* Optional limits */}
      <div className="rw-form-limits">
        <div className="rw-limit-row">
          <label className="rw-toggle-label">
            <input
              type="checkbox"
              checked={form.maxPerStreamEnabled}
              onChange={e => set('maxPerStreamEnabled', e.target.checked)}
            />
            Max per stream
          </label>
          {form.maxPerStreamEnabled && (
            <input
              type="number"
              min={1}
              value={form.maxPerStream || 1}
              onChange={e => set('maxPerStream', parseInt(e.target.value, 10) || 1)}
              className="rw-limit-input"
            />
          )}
        </div>

        <div className="rw-limit-row">
          <label className="rw-toggle-label">
            <input
              type="checkbox"
              checked={form.maxPerUserEnabled}
              onChange={e => set('maxPerUserEnabled', e.target.checked)}
            />
            Max per user per stream
          </label>
          {form.maxPerUserEnabled && (
            <input
              type="number"
              min={1}
              value={form.maxPerUser || 1}
              onChange={e => set('maxPerUser', parseInt(e.target.value, 10) || 1)}
              className="rw-limit-input"
            />
          )}
        </div>

        <div className="rw-limit-row">
          <label className="rw-toggle-label">
            <input
              type="checkbox"
              checked={form.cooldownEnabled}
              onChange={e => set('cooldownEnabled', e.target.checked)}
            />
            Global cooldown
          </label>
          {form.cooldownEnabled && (
            <span className="rw-limit-with-unit">
              <input
                type="number"
                min={1}
                value={form.cooldownSeconds || 60}
                onChange={e => set('cooldownSeconds', parseInt(e.target.value, 10) || 60)}
                className="rw-limit-input"
              />
              <span className="rw-unit">sec</span>
            </span>
          )}
        </div>
      </div>

      {error && <p className="rw-error">{error}</p>}

      <div className="rw-form-actions">
        <button type="submit" className={`btn${saved ? ' btn--saved' : ''}`} disabled={saving}>
          {saving ? 'Saving…' : saved ? '✓ Saved!' : isEdit ? 'Save Changes' : 'Create Reward'}
        </button>
        <button type="button" className="btn btn--secondary" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  )
}

// ── Redemption Queue ──────────────────────────────────────────────────────────

function RedemptionQueue({ reward, onClose }) {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState({})

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await GetPendingRedemptions(reward.id)
      setItems(data || [])
    } catch (err) {
      setError(String(err))
    } finally {
      setLoading(false)
    }
  }, [reward.id])

  useEffect(() => { load() }, [load])

  const act = async (fn, redemptionId, action) => {
    setBusy(b => ({ ...b, [redemptionId + action]: true }))
    try {
      await fn(reward.id, redemptionId)
      setItems(prev => prev.filter(r => r.id !== redemptionId))
    } catch (err) {
      setError(String(err))
    } finally {
      setBusy(b => ({ ...b, [redemptionId + action]: false }))
    }
  }

  return (
    <div className="rw-queue">
      <div className="rw-queue-header">
        <h3>Queue — {reward.title}</h3>
        <button className="rw-queue-close" onClick={onClose} title="Close queue">✕</button>
      </div>

      {loading && <p className="rw-loading">Loading redemptions…</p>}
      {error && <p className="rw-error">{error}</p>}

      {!loading && items.length === 0 && !error && (
        <p className="rw-empty">No pending redemptions.</p>
      )}

      {items.map(r => (
        <div key={r.id} className="rw-redemption-item">
          <div className="rw-redemption-info">
            <span className="rw-redemption-user">{r.userName}</span>
            <span className="rw-redemption-time">{formatDate(r.redeemedAt)}</span>
            {r.userInput && (
              <span className="rw-redemption-input">"{r.userInput}"</span>
            )}
          </div>
          <div className="rw-redemption-actions">
            <button
              className="btn btn--fulfill"
              disabled={busy[r.id + 'f']}
              onClick={() => act(FulfillRedemption, r.id, 'f')}
            >
              ✓ Fulfill
            </button>
            <button
              className="btn btn--cancel-redemption"
              disabled={busy[r.id + 'c']}
              onClick={() => act(CancelRedemption, r.id, 'c')}
            >
              ✕ Cancel
            </button>
          </div>
        </div>
      ))}

      <div className="rw-queue-footer">
        <button className="btn btn--secondary btn--sm" onClick={load}>
          ↺ Refresh
        </button>
      </div>
    </div>
  )
}

// ── Reward Card ───────────────────────────────────────────────────────────────

function RewardCard({ reward, onEdit, onDelete, onViewQueue }) {
  const [toggling, setToggling] = useState(false)
  const [paused, setPaused] = useState(reward.isPaused)

  const handleTogglePause = async () => {
    setToggling(true)
    try {
      await ToggleCustomRewardPaused(reward.id, !paused)
      setPaused(p => !p)
    } catch (err) {
      // swallow; refresh will fix
    } finally {
      setToggling(false)
    }
  }

  const color = reward.backgroundColor || '#9147ff'
  const rgb = hexToRgb(color)

  return (
    <div
      className="rw-card"
      style={{ '--rw-accent': color, '--rw-accent-rgb': rgb }}
    >
      <div className="rw-card-accent" />
      <div className="rw-card-body">
        <div className="rw-card-header">
          <span className="rw-card-title">{reward.title}</span>
          <span className="rw-card-cost">🎁 {reward.cost.toLocaleString()}</span>
        </div>

        {reward.prompt && (
          <p className="rw-card-prompt">{reward.prompt}</p>
        )}

        <div className="rw-card-badges">
          {reward.isEnabled
            ? <span className="rw-badge rw-badge--enabled">Enabled</span>
            : <span className="rw-badge rw-badge--disabled">Disabled</span>}
          {paused && <span className="rw-badge rw-badge--paused">Paused</span>}
          {!reward.isInStock && <span className="rw-badge rw-badge--stock">Out of Stock</span>}
          {reward.isUserInputRequired && <span className="rw-badge rw-badge--input">Input Required</span>}
          {reward.shouldRedemptionsSkipRequestQueue && <span className="rw-badge rw-badge--auto">Auto-fulfill</span>}
        </div>

        <div className="rw-card-actions">
          <button
            className={`btn btn--sm btn--secondary${paused ? ' btn--active' : ''}`}
            onClick={handleTogglePause}
            disabled={toggling}
            title={paused ? 'Unpause reward' : 'Pause reward'}
          >
            {paused ? '▶ Unpause' : '⏸ Pause'}
          </button>
          <button className="btn btn--sm" onClick={() => onEdit(reward)}>
            Edit
          </button>
          {!reward.shouldRedemptionsSkipRequestQueue && (
            <button className="btn btn--sm btn--queue" onClick={() => onViewQueue(reward)}>
              Queue
            </button>
          )}
          <button
            className="btn btn--sm btn--danger"
            onClick={() => onDelete(reward.id)}
            title="Delete reward permanently"
          >
            Delete
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Main Page ─────────────────────────────────────────────────────────────────

export default function Rewards() {
  const [rewards, setRewards] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // form state: null = hidden, 'create' = new, object = editing existing
  const [formMode, setFormMode] = useState(null)
  const [queueReward, setQueueReward] = useState(null)
  const [showTemplates, setShowTemplates] = useState(false)

  const loadRewards = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await GetCustomRewards()
      setRewards(data || [])
    } catch (err) {
      setError(String(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { loadRewards() }, [loadRewards])

  // Re-fetch when the AI voice command creates or modifies a reward.
  useEffect(() => {
    const off = EventsOn('rewards:changed', loadRewards)
    return () => off()
  }, [loadRewards])

  const handleSave = async (form) => {
    if (formMode === 'create' || (formMode && !formMode.id)) {
      await CreateCustomReward(form)
    } else {
      await UpdateCustomReward(formMode.id, form)
    }
    setFormMode(null)
    await loadRewards()
  }

  const handleDelete = async (id) => {
    if (!window.confirm('Delete this reward permanently? This cannot be undone.')) return
    try {
      await DeleteCustomReward(id)
      setRewards(prev => prev.filter(r => r.id !== id))
    } catch (err) {
      setError(String(err))
    }
  }

  const openEdit = (reward) => {
    setQueueReward(null)
    setFormMode({
      id: reward.id,
      title: reward.title,
      cost: reward.cost,
      prompt: reward.prompt || '',
      isEnabled: reward.isEnabled,
      backgroundColor: reward.backgroundColor || '#9147ff',
      isUserInputRequired: reward.isUserInputRequired,
      shouldRedemptionsSkipRequestQueue: reward.shouldRedemptionsSkipRequestQueue,
      maxPerStreamEnabled: reward.maxPerStreamEnabled,
      maxPerStream: reward.maxPerStream || 0,
      maxPerUserEnabled: reward.maxPerUserEnabled,
      maxPerUser: reward.maxPerUser || 0,
      cooldownEnabled: reward.cooldownEnabled,
      cooldownSeconds: reward.cooldownSeconds || 0,
    })
  }

  return (
    <div className="rewards-page">
      <div className="rw-page-header">
        <h2>Channel Point Rewards</h2>
        <div className="rw-header-actions">
          <button
            className="btn"
            onClick={() => { setFormMode('create'); setQueueReward(null); setShowTemplates(false) }}
            disabled={formMode === 'create' && !showTemplates}
          >
            + Create Reward
          </button>
          <button
            className={`btn btn--sm${showTemplates ? ' btn--active' : ' btn--secondary'}`}
            onClick={() => { setShowTemplates(s => !s); setFormMode(null); setQueueReward(null) }}
          >
            {showTemplates ? '✕ Templates' : '⚡ Templates'}
          </button>
          <button className="btn btn--secondary btn--sm" onClick={loadRewards}>
            ↺ Refresh
          </button>
        </div>
      </div>

      {showTemplates && (
        <TemplateGallery
          onUse={tpl => {
            setShowTemplates(false)
            setQueueReward(null)
            const { emoji, tags, desc, ...formValues } = tpl
            setFormMode({ ...EMPTY_FORM, ...formValues })
          }}
        />
      )}

      {formMode !== null && !showTemplates && (
        <RewardForm
          initial={formMode === 'create' ? null : formMode}
          onSave={handleSave}
          onCancel={() => setFormMode(null)}
        />
      )}

      {queueReward && (
        <RedemptionQueue
          reward={queueReward}
          onClose={() => setQueueReward(null)}
        />
      )}

      {loading && <p className="rw-loading">Loading rewards…</p>}
      {error && <p className="rw-error">{error}</p>}

      {!loading && rewards.length === 0 && !error && (
        <div className="rw-empty-state">
          <p>No custom rewards found.</p>
          <p className="rw-empty-hint">
            Note: only rewards created by this app can be managed here.
          </p>
        </div>
      )}

      <div className="rw-cards">
        {rewards.map(r => (
          <RewardCard
            key={r.id}
            reward={r}
            onEdit={openEdit}
            onDelete={handleDelete}
            onViewQueue={rw => { setQueueReward(rw); setFormMode(null) }}
          />
        ))}
      </div>
    </div>
  )
}

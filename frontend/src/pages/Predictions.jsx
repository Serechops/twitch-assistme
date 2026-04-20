import { useEffect, useState, useCallback } from 'react'
import {
  CreatePrediction,
  EndPrediction,
  GetPredictions,
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

const PREDICTION_WINDOWS = [
  { label: '30s',  seconds: 30 },
  { label: '1m',   seconds: 60 },
  { label: '2m',   seconds: 120 },
  { label: '5m',   seconds: 300 },
  { label: '10m',  seconds: 600 },
  { label: '15m',  seconds: 900 },
  { label: '30m',  seconds: 1800 },
]

// Blue and Pink are the two Twitch-assigned colors; extra outcomes cycle through these
const OUTCOME_COLORS = ['BLUE', 'PINK', 'BLUE', 'PINK', 'BLUE', 'PINK', 'BLUE', 'PINK', 'BLUE', 'PINK']

function formatDate(isoStr) {
  if (!isoStr) return '—'
  return new Date(isoStr).toLocaleString()
}

function formatStatus(status) {
  if (!status) return ''
  return status.charAt(0) + status.slice(1).toLowerCase()
}

// ── Outcome Bars ─────────────────────────────────────────────────────────────

function OutcomeBars({ outcomes, winningOutcomeId }) {
  const totalPoints = (outcomes || []).reduce((s, o) => s + (o.channelPoints || 0), 0)
  return (
    <div className="poll-choices" style={{ marginBottom: 0 }}>
      {(outcomes || []).map((o, i) => {
        const pct = totalPoints > 0 ? Math.round(((o.channelPoints || 0) / totalPoints) * 100) : 0
        const isWinner = winningOutcomeId && o.id === winningOutcomeId
        return (
          <div className="poll-choice" key={o.id || i}>
            <div className="poll-choice-header">
              <span className="poll-choice-title">
                {isWinner && '🏆 '}{o.title}
                {o.color && (
                  <span
                    className="prediction-color-dot"
                    style={{ backgroundColor: o.color === 'BLUE' ? '#3a9cff' : '#ff5ea0' }}
                  />
                )}
              </span>
              <span className="poll-choice-votes">
                {(o.channelPoints || 0).toLocaleString()} pts · {o.users || 0} predictors ({pct}%)
              </span>
            </div>
            <div className="poll-bar-track">
              <div
                className="poll-bar-fill"
                style={{
                  width: `${pct}%`,
                  backgroundColor: o.color === 'PINK' ? 'var(--prediction-pink, #ff5ea0)' : undefined,
                }}
              />
            </div>
          </div>
        )
      })}
    </div>
  )
}

// ── Active prediction card ────────────────────────────────────────────────────

function ActivePredictionCard({ prediction, onEnd, busy }) {
  const isActive = prediction.status === 'ACTIVE'
  const isLocked = prediction.status === 'LOCKED'

  return (
    <div
      className="card"
      style={{ borderColor: isActive || isLocked ? 'var(--accent)' : undefined }}
    >
      <div
        className="card-title"
        style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}
      >
        <span>Active Prediction</span>
        <span className={`poll-status-badge poll-status-${prediction.status?.toLowerCase()}`}>
          {formatStatus(prediction.status)}
        </span>
      </div>
      <div className="poll-title">{prediction.title}</div>
      <OutcomeBars outcomes={prediction.outcomes} winningOutcomeId={prediction.winningOutcomeId} />

      {isActive && (
        <div className="poll-actions">
          <button
            className="btn btn-secondary btn-sm"
            onClick={() => onEnd('LOCKED', '')}
            disabled={busy}
          >
            Lock Predictions
          </button>
          <button
            className="btn btn-danger btn-sm"
            onClick={() => onEnd('CANCELED', '')}
            disabled={busy}
          >
            Cancel &amp; Refund
          </button>
        </div>
      )}

      {isLocked && (
        <ResolvePanel prediction={prediction} onEnd={onEnd} busy={busy} />
      )}
    </div>
  )
}

function ResolvePanel({ prediction, onEnd, busy }) {
  const [winnerID, setWinnerID] = useState('')
  return (
    <div style={{ marginTop: 12 }}>
      <div className="setting-label" style={{ marginBottom: 8 }}>Pick the winning outcome:</div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginBottom: 12 }}>
        {(prediction.outcomes || []).map(o => (
          <label key={o.id} style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
            <input
              type="radio"
              name="winner"
              value={o.id}
              checked={winnerID === o.id}
              onChange={() => setWinnerID(o.id)}
            />
            <span
              style={{
                display: 'inline-block',
                width: 10,
                height: 10,
                borderRadius: '50%',
                backgroundColor: o.color === 'PINK' ? '#ff5ea0' : '#3a9cff',
                flexShrink: 0,
              }}
            />
            {o.title}
          </label>
        ))}
      </div>
      <div className="poll-actions">
        <button
          className="btn btn-primary btn-sm"
          onClick={() => winnerID && onEnd('RESOLVED', winnerID)}
          disabled={busy || !winnerID}
        >
          Resolve
        </button>
        <button
          className="btn btn-danger btn-sm"
          onClick={() => onEnd('CANCELED', '')}
          disabled={busy}
        >
          Cancel &amp; Refund
        </button>
      </div>
    </div>
  )
}

// ── Create tab ───────────────────────────────────────────────────────────────

function CreateTab({ activePrediction, title, setTitle, outcomes, setOutcomes, window, setWindow, onCreate, busy, error }) {
  function addOutcome() {
    if (outcomes.length < 10) setOutcomes(prev => [...prev, ''])
  }
  function removeOutcome(i) {
    if (outcomes.length > 2) setOutcomes(prev => prev.filter((_, idx) => idx !== i))
  }
  function updateOutcome(i, val) {
    setOutcomes(prev => prev.map((o, idx) => idx === i ? val : o))
  }

  const hasActive = activePrediction && (activePrediction.status === 'ACTIVE' || activePrediction.status === 'LOCKED')

  return (
    <div>
      {error && <div className="notice error">{error}</div>}
      <div className="settings-group">
        <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
          <div className="setting-label">Question</div>
          <input
            className="text-input"
            type="text"
            placeholder="Will I win this match?"
            maxLength={45}
            value={title}
            onChange={e => setTitle(e.target.value)}
          />
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <div className="setting-label">Outcomes ({outcomes.length}/10)</div>
          {outcomes.map((o, i) => (
            <div key={i} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <span
                style={{
                  display: 'inline-block',
                  width: 10,
                  height: 10,
                  borderRadius: '50%',
                  backgroundColor: OUTCOME_COLORS[i] === 'PINK' ? '#ff5ea0' : '#3a9cff',
                  flexShrink: 0,
                }}
              />
              <input
                className="text-input"
                type="text"
                placeholder={`Outcome ${i + 1}`}
                maxLength={25}
                value={o}
                onChange={e => updateOutcome(i, e.target.value)}
              />
              {outcomes.length > 2 && (
                <button className="btn btn-danger btn-sm" onClick={() => removeOutcome(i)} title="Remove">✕</button>
              )}
            </div>
          ))}
          {outcomes.length < 10 && (
            <button
              className="btn btn-secondary btn-sm"
              style={{ alignSelf: 'flex-start' }}
              onClick={addOutcome}
            >
              + Add outcome
            </button>
          )}
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <div className="setting-label">Prediction window</div>
          <div className="poll-duration-presets">
            {PREDICTION_WINDOWS.map(({ label, seconds }) => (
              <button
                key={seconds}
                className={`btn btn-sm poll-duration-btn${window === seconds ? ' poll-duration-btn--active' : ''}`}
                onClick={() => setWindow(seconds)}
              >
                {label}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 14 }}>
        <button
          className="btn btn-primary"
          onClick={onCreate}
          disabled={busy || hasActive}
        >
          {busy
            ? 'Creating…'
            : hasActive
              ? 'Prediction already active'
              : 'Start Prediction'}
        </button>
      </div>
    </div>
  )
}

// ── History tab ──────────────────────────────────────────────────────────────

function HistoryTab() {
  const [predictions, setPredictions] = useState([])
  const [loading, setLoading] = useState(true)
  const [expanded, setExpanded] = useState(null)

  useEffect(() => {
    GetPredictions()
      .then(rows => setPredictions((rows || []).filter(p => p.status !== 'ACTIVE' && p.status !== 'LOCKED')))
      .catch(() => setPredictions([]))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <p className="polls-empty">Loading…</p>
  if (predictions.length === 0)
    return <p className="polls-empty">No past predictions yet.</p>

  return (
    <div className="polls-list">
      {predictions.map(p => (
        <div className="polls-item" key={p.id}>
          <div
            className="polls-item-header"
            onClick={() => setExpanded(prev => prev === p.id ? null : p.id)}
          >
            <div className="polls-item-left">
              <span className={`poll-status-badge poll-status-${p.status?.toLowerCase()}`}>
                {formatStatus(p.status)}
              </span>
              <span className="polls-item-title">{p.title}</span>
            </div>
            <div className="polls-item-right">
              <span className="polls-item-date">{formatDate(p.createdAt)}</span>
              <span className="polls-chevron">{expanded === p.id ? '▲' : '▼'}</span>
            </div>
          </div>
          {expanded === p.id && (
            <div className="polls-item-body">
              <OutcomeBars outcomes={p.outcomes} winningOutcomeId={p.winningOutcomeId} />
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

// ── Page ─────────────────────────────────────────────────────────────────────

export default function Predictions() {
  const [tab, setTab] = useState('create')

  const [activePrediction, setActivePrediction] = useState(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  // Create form state
  const [title, setTitle] = useState('')
  const [outcomes, setOutcomes] = useState(['', ''])
  const [window, setWindow] = useState(120)

  // Check for any currently active/locked prediction on mount
  const fetchActivePrediction = useCallback(() => {
    GetPredictions()
      .then(preds => {
        const active = (preds || []).find(p => p.status === 'ACTIVE' || p.status === 'LOCKED')
        if (active) setActivePrediction(active)
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    fetchActivePrediction()
  }, [fetchActivePrediction])

  // Subscribe to EventSub prediction events
  useEffect(() => {
    const offBegin = EventsOn('prediction:begin', evt => {
      setActivePrediction({
        id: evt.id,
        title: evt.title,
        outcomes: evt.outcomes,
        status: 'ACTIVE',
        winningOutcomeId: '',
      })
    })
    const offProgress = EventsOn('prediction:progress', evt => {
      setActivePrediction(prev => prev ? { ...prev, outcomes: evt.outcomes } : prev)
    })
    const offLock = EventsOn('prediction:lock', evt => {
      setActivePrediction(prev => prev
        ? { ...prev, status: 'LOCKED', outcomes: evt.outcomes }
        : { id: evt.id, title: evt.title, outcomes: evt.outcomes, status: 'LOCKED', winningOutcomeId: '' })
    })
    const offEnd = EventsOn('prediction:end', evt => {
      setActivePrediction({
        id: evt.id,
        title: evt.title,
        outcomes: evt.outcomes,
        status: evt.status,
        winningOutcomeId: evt.winning_outcome_id || '',
      })
    })
    return () => { offBegin(); offProgress(); offLock(); offEnd() }
  }, [])

  async function handleCreate() {
    setError('')
    const filled = outcomes.map(o => o.trim()).filter(Boolean)
    if (!title.trim()) { setError('Prediction question is required.'); return }
    if (filled.length < 2) { setError('At least 2 outcomes are required.'); return }
    setBusy(true)
    try {
      const p = await CreatePrediction(title.trim(), filled, window)
      setActivePrediction(p)
      setTitle('')
      setOutcomes(['', ''])
      setWindow(120)
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  async function handleEnd(status, winningOutcomeID) {
    if (!activePrediction?.id) return
    setBusy(true)
    setError('')
    try {
      const p = await EndPrediction(activePrediction.id, status, winningOutcomeID)
      setActivePrediction(p)
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <h1 className="page-title">Predictions</h1>

      {error && <div className="notice error" style={{ marginBottom: 12 }}>{error}</div>}

      {activePrediction && (activePrediction.status === 'ACTIVE' || activePrediction.status === 'LOCKED') && (
        <ActivePredictionCard
          prediction={activePrediction}
          busy={busy}
          onEnd={handleEnd}
        />
      )}

      <div className="tabs">
        <button
          className={`tab-btn${tab === 'create' ? ' tab-btn--active' : ''}`}
          onClick={() => setTab('create')}
        >
          Create
        </button>
        <button
          className={`tab-btn${tab === 'history' ? ' tab-btn--active' : ''}`}
          onClick={() => setTab('history')}
        >
          History
        </button>
      </div>

      <div className="tab-content">
        {tab === 'create' && (
          <CreateTab
            activePrediction={activePrediction}
            title={title}
            setTitle={setTitle}
            outcomes={outcomes}
            setOutcomes={setOutcomes}
            window={window}
            setWindow={setWindow}
            onCreate={handleCreate}
            busy={busy}
            error={null}
          />
        )}
        {tab === 'history' && <HistoryTab />}
      </div>
    </>
  )
}

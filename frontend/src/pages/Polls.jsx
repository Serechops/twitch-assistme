import { useEffect, useState } from 'react'
import {
  CreatePoll,
  EndPoll,
  GetPollArchive,
  GetPollTemplates,
  SavePollTemplate,
  DeletePollTemplate,
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

const POLL_DURATIONS = [
  { label: '30s', seconds: 30 },
  { label: '1m',  seconds: 60 },
  { label: '2m',  seconds: 120 },
  { label: '5m',  seconds: 300 },
  { label: '10m', seconds: 600 },
  { label: '15m', seconds: 900 },
  { label: '30m', seconds: 1800 },
]

function formatDate(unixSec) {
  if (!unixSec) return '—'
  return new Date(unixSec * 1000).toLocaleString()
}

// ── Shared vote bars ─────────────────────────────────────────────────────────

function PollVoteBars({ choices }) {
  const totalVotes = (choices || []).reduce((s, c) => s + (c.votes || 0), 0)
  return (
    <div className="poll-choices" style={{ marginBottom: 0 }}>
      {(choices || []).map((c, i) => {
        const pct = totalVotes > 0 ? Math.round(((c.votes || 0) / totalVotes) * 100) : 0
        return (
          <div className="poll-choice" key={i}>
            <div className="poll-choice-header">
              <span className="poll-choice-title">{c.title}</span>
              <span className="poll-choice-votes">{c.votes || 0} votes ({pct}%)</span>
            </div>
            <div className="poll-bar-track">
              <div className="poll-bar-fill" style={{ width: `${pct}%` }} />
            </div>
          </div>
        )
      })}
    </div>
  )
}

// ── Active poll card ─────────────────────────────────────────────────────────

function ActivePollCard({ poll, onEnd, onClear, busy }) {
  return (
    <div className="card" style={{ borderColor: poll.status === 'ACTIVE' ? 'var(--accent)' : undefined }}>
      <div className="card-title" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <span>Active Poll</span>
        <span className={`poll-status-badge poll-status-${poll.status?.toLowerCase()}`}>
          {poll.status}
        </span>
      </div>
      <div className="poll-title">{poll.title}</div>
      <PollVoteBars choices={poll.choices} />
      {poll.status === 'ACTIVE' && (
        <div className="poll-actions">
          <button className="btn btn-primary btn-sm" onClick={() => onEnd(true)} disabled={busy}>
            End &amp; Show Results
          </button>
          <button className="btn btn-secondary btn-sm" onClick={() => onEnd(false)} disabled={busy}>
            End &amp; Archive
          </button>
        </div>
      )}
      {poll.status !== 'ACTIVE' && (
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 10 }}>
          <button className="btn btn-secondary btn-sm" onClick={onClear}>Clear</button>
        </div>
      )}
    </div>
  )
}

// ── Create tab ───────────────────────────────────────────────────────────────

function CreateTab({ activePoll, pollTitle, setPollTitle, pollChoices, setPollChoices, pollDuration, setPollDuration, onCreatePoll, busy, error }) {
  function addChoice() {
    if (pollChoices.length < 5) setPollChoices(prev => [...prev, ''])
  }
  function removeChoice(i) {
    if (pollChoices.length > 2) setPollChoices(prev => prev.filter((_, idx) => idx !== i))
  }
  function updateChoice(i, val) {
    setPollChoices(prev => prev.map((c, idx) => idx === i ? val : c))
  }

  return (
    <div>
      {error && <div className="notice error">{error}</div>}
      <div className="settings-group">
        <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
          <div className="setting-label">Question</div>
          <input
            className="text-input"
            type="text"
            placeholder="Ask your viewers something…"
            maxLength={60}
            value={pollTitle}
            onChange={e => setPollTitle(e.target.value)}
          />
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <div className="setting-label">Choices ({pollChoices.length}/5)</div>
          {pollChoices.map((c, i) => (
            <div key={i} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <input
                className="text-input"
                type="text"
                placeholder={`Choice ${i + 1}`}
                maxLength={25}
                value={c}
                onChange={e => updateChoice(i, e.target.value)}
              />
              {pollChoices.length > 2 && (
                <button className="btn btn-danger btn-sm" onClick={() => removeChoice(i)} title="Remove">✕</button>
              )}
            </div>
          ))}
          {pollChoices.length < 5 && (
            <button className="btn btn-secondary btn-sm" style={{ alignSelf: 'flex-start' }} onClick={addChoice}>
              + Add choice
            </button>
          )}
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <div className="setting-label">Duration</div>
          <div className="poll-duration-presets">
            {POLL_DURATIONS.map(({ label, seconds }) => (
              <button
                key={seconds}
                className={`btn btn-sm poll-duration-btn${pollDuration === seconds ? ' poll-duration-btn--active' : ''}`}
                onClick={() => setPollDuration(seconds)}
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
          onClick={onCreatePoll}
          disabled={busy || activePoll?.status === 'ACTIVE'}
        >
          {busy ? 'Creating…' : activePoll?.status === 'ACTIVE' ? 'Poll already active' : 'Start Poll'}
        </button>
      </div>
    </div>
  )
}

// ── History tab ──────────────────────────────────────────────────────────────

function HistoryTab() {
  const [archive, setArchive] = useState([])
  const [loading, setLoading] = useState(true)
  const [expanded, setExpanded] = useState(null)

  useEffect(() => {
    GetPollArchive()
      .then(rows => setArchive(rows || []))
      .catch(() => setArchive([]))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <p className="polls-empty">Loading…</p>
  if (archive.length === 0)
    return <p className="polls-empty">No polls archived yet. Polls are saved automatically when they end.</p>

  return (
    <div className="polls-list">
      {archive.map(poll => (
        <div className="polls-item" key={poll.id}>
          <div
            className="polls-item-header"
            onClick={() => setExpanded(prev => prev === poll.id ? null : poll.id)}
          >
            <div className="polls-item-left">
              <span className={`poll-status-badge poll-status-${poll.status?.toLowerCase()}`}>
                {poll.status}
              </span>
              <span className="polls-item-title">{poll.title}</span>
            </div>
            <div className="polls-item-right">
              <span className="polls-item-date">{formatDate(poll.createdAt)}</span>
              <span className="polls-chevron">{expanded === poll.id ? '▲' : '▼'}</span>
            </div>
          </div>
          {expanded === poll.id && (
            <div className="polls-item-body">
              <PollVoteBars choices={poll.choices} />
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

// ── Templates tab ────────────────────────────────────────────────────────────

function TemplatesTab({ onUseTemplate }) {
  const [templates, setTemplates] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const [formName, setFormName] = useState('')
  const [formTitle, setFormTitle] = useState('')
  const [formChoices, setFormChoices] = useState(['', ''])
  const [formDuration, setFormDuration] = useState(120)
  const [showForm, setShowForm] = useState(false)

  function loadTemplates() {
    GetPollTemplates()
      .then(rows => setTemplates(rows || []))
      .catch(() => setTemplates([]))
      .finally(() => setLoading(false))
  }
  useEffect(() => { loadTemplates() }, [])

  function addFormChoice() {
    if (formChoices.length < 5) setFormChoices(p => [...p, ''])
  }
  function removeFormChoice(i) {
    if (formChoices.length > 2) setFormChoices(p => p.filter((_, idx) => idx !== i))
  }
  function updateFormChoice(i, val) {
    setFormChoices(p => p.map((c, idx) => idx === i ? val : c))
  }

  async function handleSaveTemplate() {
    setError('')
    const name = formName.trim()
    const title = formTitle.trim()
    const choices = formChoices.map(c => c.trim()).filter(Boolean)
    if (!name)  { setError('Template name is required.'); return }
    if (!title) { setError('Poll question is required.'); return }
    if (choices.length < 2) { setError('At least 2 choices are required.'); return }
    setBusy(true)
    try {
      await SavePollTemplate(name, title, choices, formDuration)
      setFormName(''); setFormTitle(''); setFormChoices(['', '']); setFormDuration(120)
      setShowForm(false)
      loadTemplates()
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  async function handleDelete(id) {
    try {
      await DeletePollTemplate(id)
      setTemplates(prev => prev.filter(t => t.id !== id))
    } catch (e) {
      setError(String(e))
    }
  }

  if (loading) return <p className="polls-empty">Loading…</p>

  return (
    <div>
      {error && <div className="notice error">{error}</div>}

      {templates.length === 0 && !showForm && (
        <p className="polls-empty">No templates saved yet. Create one to reuse polls quickly.</p>
      )}

      {templates.length > 0 && (
        <div className="polls-list" style={{ marginBottom: 16 }}>
          {templates.map(t => (
            <div className="polls-item" key={t.id}>
              <div className="polls-item-header">
                <div className="polls-item-left">
                  <span className="template-name">{t.name}</span>
                  <span className="polls-item-title" style={{ color: 'var(--text-muted)', fontSize: 13 }}>
                    {t.title}
                  </span>
                </div>
                <div className="polls-item-right" style={{ gap: 6 }}>
                  <span className="polls-item-date">
                    {POLL_DURATIONS.find(d => d.seconds === t.duration)?.label ?? `${t.duration}s`}
                  </span>
                  <button className="btn btn-primary btn-sm" onClick={() => onUseTemplate(t)}>
                    Use
                  </button>
                  <button className="btn btn-danger btn-sm" onClick={() => handleDelete(t.id)}>
                    Delete
                  </button>
                </div>
              </div>
              <div className="polls-template-choices">
                {(t.choices || []).map((c, i) => (
                  <span className="template-choice-pill" key={i}>{c}</span>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {showForm ? (
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">New Template</div>
          <div className="settings-group">
            <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
              <div className="setting-label">Template Name</div>
              <input className="text-input" type="text" placeholder="e.g. Favourite Game Poll"
                maxLength={60} value={formName} onChange={e => setFormName(e.target.value)} />
            </div>
            <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
              <div className="setting-label">Question</div>
              <input className="text-input" type="text" placeholder="Ask your viewers something…"
                maxLength={60} value={formTitle} onChange={e => setFormTitle(e.target.value)} />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              <div className="setting-label">Choices ({formChoices.length}/5)</div>
              {formChoices.map((c, i) => (
                <div key={i} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <input className="text-input" type="text" placeholder={`Choice ${i + 1}`}
                    maxLength={25} value={c} onChange={e => updateFormChoice(i, e.target.value)} />
                  {formChoices.length > 2 && (
                    <button className="btn btn-danger btn-sm" onClick={() => removeFormChoice(i)}>✕</button>
                  )}
                </div>
              ))}
              {formChoices.length < 5 && (
                <button className="btn btn-secondary btn-sm" style={{ alignSelf: 'flex-start' }} onClick={addFormChoice}>
                  + Add choice
                </button>
              )}
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              <div className="setting-label">Default Duration</div>
              <div className="poll-duration-presets">
                {POLL_DURATIONS.map(({ label, seconds }) => (
                  <button key={seconds}
                    className={`btn btn-sm poll-duration-btn${formDuration === seconds ? ' poll-duration-btn--active' : ''}`}
                    onClick={() => setFormDuration(seconds)}>
                    {label}
                  </button>
                ))}
              </div>
            </div>
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 14 }}>
            <button className="btn btn-secondary" onClick={() => { setShowForm(false); setError('') }}>
              Cancel
            </button>
            <button className="btn btn-primary" onClick={handleSaveTemplate} disabled={busy}>
              {busy ? 'Saving…' : 'Save Template'}
            </button>
          </div>
        </div>
      ) : (
        <button className="btn btn-secondary" onClick={() => setShowForm(true)}>
          + New Template
        </button>
      )}
    </div>
  )
}

// ── Page ─────────────────────────────────────────────────────────────────────

export default function Polls() {
  const [tab, setTab] = useState('create')

  // Active poll (driven by EventSub events)
  const [activePoll, setActivePoll] = useState(null)
  const [pollBusy, setPollBusy] = useState(false)
  const [pollError, setPollError] = useState('')

  // Poll creator form state (lifted so "Use template" can fill it)
  const [pollTitle, setPollTitle] = useState('')
  const [pollChoices, setPollChoices] = useState(['', ''])
  const [pollDuration, setPollDuration] = useState(120)

  // Subscribe to EventSub poll events
  useEffect(() => {
    const offBegin = EventsOn('poll:begin', evt => {
      setActivePoll({ id: evt.id, title: evt.title, choices: evt.choices, endsAt: evt.ends_at, status: 'ACTIVE' })
    })
    const offProgress = EventsOn('poll:progress', evt => {
      setActivePoll(prev => prev ? { ...prev, choices: evt.choices } : prev)
    })
    const offEnd = EventsOn('poll:end', evt => {
      setActivePoll({ id: evt.id, title: evt.title, choices: evt.choices, endsAt: evt.ended_at, status: evt.status })
    })
    return () => { offBegin(); offProgress(); offEnd() }
  }, [])

  async function handleCreatePoll() {
    setPollError('')
    const filledChoices = pollChoices.map(c => c.trim()).filter(Boolean)
    if (!pollTitle.trim()) { setPollError('Poll question is required.'); return }
    if (filledChoices.length < 2) { setPollError('At least 2 choices are required.'); return }
    setPollBusy(true)
    try {
      const poll = await CreatePoll(pollTitle.trim(), filledChoices, pollDuration)
      setActivePoll(poll)
      setPollTitle('')
      setPollChoices(['', ''])
      setPollDuration(120)
    } catch (e) {
      setPollError(String(e))
    } finally {
      setPollBusy(false)
    }
  }

  async function handleEndPoll(showResults) {
    if (!activePoll?.id) return
    setPollBusy(true)
    try {
      const poll = await EndPoll(activePoll.id, showResults)
      setActivePoll(poll)
    } catch (e) {
      setPollError(String(e))
    } finally {
      setPollBusy(false)
    }
  }

  // "Use" fills the creator form and switches to the Create tab
  function handleUseTemplate(template) {
    setPollTitle(template.title || '')
    setPollChoices(
      (template.choices || []).length >= 2
        ? template.choices.map(c => c || '')
        : ['', '']
    )
    setPollDuration(template.duration || 120)
    setTab('create')
  }

  return (
    <>
      <h1 className="page-title">Polls</h1>

      {activePoll && (
        <ActivePollCard
          poll={activePoll}
          busy={pollBusy}
          onEnd={handleEndPoll}
          onClear={() => setActivePoll(null)}
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
        <button
          className={`tab-btn${tab === 'templates' ? ' tab-btn--active' : ''}`}
          onClick={() => setTab('templates')}
        >
          Templates
        </button>
      </div>

      <div className="card">
        {tab === 'create' && (
          <CreateTab
            activePoll={activePoll}
            pollTitle={pollTitle}
            setPollTitle={setPollTitle}
            pollChoices={pollChoices}
            setPollChoices={setPollChoices}
            pollDuration={pollDuration}
            setPollDuration={setPollDuration}
            onCreatePoll={handleCreatePoll}
            busy={pollBusy}
            error={pollError}
          />
        )}
        {tab === 'history' && <HistoryTab />}
        {tab === 'templates' && <TemplatesTab onUseTemplate={handleUseTemplate} />}
      </div>
    </>
  )
}

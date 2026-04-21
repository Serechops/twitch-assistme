import { useEffect, useState } from 'react'
import { GetSettings, SaveSettings, SaveCustomSound, ClearCustomSound, GetSoundDataBase64, TestSound, GetUser } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import twitchBanner from '../assets/images/twitch_banner.jpg'

export default function Settings() {
  const [s, setS] = useState(null)
  const [user, setUser] = useState(null)
  const [notice, setNotice] = useState(null)
  const [customFileName, setCustomFileName] = useState('')
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [showKey, setShowKey] = useState(false)

  useEffect(() => {
    GetSettings().then(settings => setS(settings))
    GetUser().then(u => setUser(u))
    const off = EventsOn('auth:changed', u => setUser(u))
    return () => off()
  }, [])

  function showNotice(type, msg) {
    setNotice({ type, msg })
    setTimeout(() => setNotice(null), 3000)
  }

  async function handleSave() {
    setBusy(true)
    try {
      await SaveSettings(s)
      window.dispatchEvent(new CustomEvent('settings:changed'))
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    } catch (e) {
      showNotice('error', String(e))
    } finally {
      setBusy(false)
    }
  }

  async function handleFileChange(e) {
    const file = e.target.files[0]
    if (!file) return

    const allowed = ['audio/mpeg', 'audio/wav', 'audio/ogg', 'audio/mp4', 'audio/x-m4a']
    if (!allowed.includes(file.type) && !file.name.match(/\.(mp3|wav|ogg|m4a)$/i)) {
      showNotice('error', 'Unsupported file type. Use MP3, WAV, OGG, or M4A.')
      return
    }

    setBusy(true)
    try {
      const buffer = await file.arrayBuffer()
      const bytes = new Uint8Array(buffer)
      let binary = ''
      for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i])
      const b64 = btoa(binary)
      await SaveCustomSound(b64, file.name)
      setCustomFileName(file.name)
      window.dispatchEvent(new CustomEvent('settings:changed'))
      showNotice('success', 'Custom sound saved.')
    } catch (err) {
      showNotice('error', String(err))
    } finally {
      setBusy(false)
    }
  }

  async function handleClearSound() {
    setBusy(true)
    try {
      await ClearCustomSound()
      setCustomFileName('')
      setS(prev => ({ ...prev, soundPath: '' }))
      window.dispatchEvent(new CustomEvent('settings:changed'))
      showNotice('success', 'Reverted to default chime.')
    } catch (err) {
      showNotice('error', String(err))
    } finally {
      setBusy(false)
    }
  }

  if (!s) return <div style={{ color: 'var(--text-muted)', padding: 24 }}>Loading settings…</div>

  return (
    <>
      <h1 className="page-title">Settings</h1>

      {notice && <div className={`notice ${notice.type}`}>{notice.msg}</div>}

      {/* Account card */}
      {user && (
        <div className="account-card">
          <div
            className="account-card-banner"
            style={{ backgroundImage: `url(${twitchBanner})` }}
          />
          <div className="account-card-body">
            {user.profileImageUrl
              ? <img className="account-card-avatar" src={user.profileImageUrl} alt={user.displayName} />
              : <div className="account-card-avatar account-card-avatar--fallback">
                  {user.displayName?.charAt(0).toUpperCase()}
                </div>
            }
            <div className="account-card-info">
              <div className="account-card-name">{user.displayName}</div>
              <div className="account-card-login">@{user.login}</div>
            </div>
          </div>
        </div>
      )}

      {/* Notification Sound */}
      <div className="card">
        <div className="card-title">Notification Sound</div>
        <div className="settings-group">

          <div className="setting-row">
            <div>
              <div className="setting-label">Enable sound</div>
              <div className="setting-description">Play a chime when a new chat message arrives.</div>
            </div>
            <label className="toggle-wrapper">
              <input type="checkbox" checked={s.soundEnabled}
                onChange={e => setS({ ...s, soundEnabled: e.target.checked })} />
              <span className="toggle-track" />
            </label>
          </div>

          <div className="setting-row">
            <div>
              <div className="setting-label">Volume</div>
              <div className="setting-description">Adjust notification volume.</div>
            </div>
            <div className="range-row">
              <input type="range" min={0} max={1} step={0.05}
                value={s.soundVolume}
                onChange={e => setS({ ...s, soundVolume: parseFloat(e.target.value) })} />
              <span className="range-val">{Math.round(s.soundVolume * 100)}%</span>
            </div>
          </div>

          <div className="setting-row">
            <div>
              <div className="setting-label">Custom sound</div>
              <div className="setting-description">Upload your own audio file (MP3, WAV, OGG, M4A). Leave empty to use the default chime.</div>
            </div>
            <div className="file-upload-row">
              {customFileName
                ? <span className="file-name-label">{customFileName}</span>
                : s.soundPath
                  ? <span className="file-name-label">Custom sound active</span>
                  : <span className="file-name-label">Default chime</span>
              }
              <label className="btn btn-secondary btn-sm" style={{ cursor: 'pointer' }}>
                Browse…
                <input type="file" accept=".mp3,.wav,.ogg,.m4a,audio/*"
                  style={{ display: 'none' }} onChange={handleFileChange} />
              </label>
            </div>
          </div>

          <div style={{ display: 'flex', gap: 10 }}>
            <button className="btn btn-secondary btn-sm"
              onClick={() => window.dispatchEvent(new CustomEvent('sound:test', { detail: { volume: s.soundVolume } }))}>
              Test sound
            </button>
            {(customFileName || s.soundPath) && (
              <button className="btn btn-danger btn-sm" onClick={handleClearSound} disabled={busy}>
                Use default
              </button>
            )}
          </div>

        </div>
      </div>

      {/* Chat Filters */}
      <div className="card">
        <div className="card-title">Chat Filters</div>
        <div className="settings-group">

          <div className="setting-row">
            <div>
              <div className="setting-label">Ignore own messages</div>
              <div className="setting-description">Don't trigger a notification when you send a chat message.</div>
            </div>
            <label className="toggle-wrapper">
              <input type="checkbox" checked={s.ignoreOwn}
                onChange={e => setS({ ...s, ignoreOwn: e.target.checked })} />
              <span className="toggle-track" />
            </label>
          </div>

          <div className="setting-row">
            <div>
              <div className="setting-label">Notification cooldown (ms)</div>
              <div className="setting-description">Minimum time between consecutive notification sounds. 0 = no cooldown.</div>
            </div>
            <input className="number-input" type="number" min={0} step={100}
              value={s.cooldownMs}
              onChange={e => setS({ ...s, cooldownMs: parseInt(e.target.value, 10) || 0 })} />
          </div>

        </div>
      </div>

      {/* AI Voice Commands */}
      <div className="card">
        <div className="card-title">AI Voice Commands</div>
        <div className="settings-group">
          <div className="setting-row" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
            <div className="setting-label">OpenAI API Key</div>
            <div className="setting-description">
              Required for voice commands. Get yours at{' '}
              <a href="https://platform.openai.com/api-keys" target="_blank" rel="noreferrer"
                style={{ color: 'var(--accent)' }}>
                platform.openai.com/api-keys
              </a>.
            </div>
            <div style={{ display: 'flex', gap: 8, width: '100%' }}>
              <input
                className="text-input"
                type={showKey ? 'text' : 'password'}
                value={s.openAIApiKey || ''}
                onChange={e => setS({ ...s, openAIApiKey: e.target.value })}
                placeholder="sk-proj-…"
                autoComplete="off"
                spellCheck={false}
                style={{ flex: 1, fontFamily: showKey ? 'monospace' : undefined }}
              />
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => setShowKey(v => !v)}
                type="button"
                title={showKey ? 'Hide key' : 'Show key'}
              >
                {showKey ? 'Hide' : 'Show'}
              </button>
            </div>
            {s.openAIApiKey
              ? <span style={{ fontSize: 12, color: 'var(--text-success, #4caf50)' }}>&#x2713; API key set — voice commands enabled</span>
              : <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>No key set — voice commands unavailable</span>
            }
          </div>

          <div className="setting-row">
            <div>
              <div className="setting-label">Game Guide voice feedback</div>
              <div className="setting-description">
                Automatically read Game Guide answers aloud using AI text-to-speech.
                Each answer incurs an additional API cost (~$0.015 per 1,000 characters with <code>gpt-4o-mini-tts</code>).
              </div>
            </div>
            <label className="toggle-wrapper">
              <input type="checkbox" checked={!!s.voiceFeedback}
                onChange={e => setS({ ...s, voiceFeedback: e.target.checked })} />
              <span className="toggle-track" />
            </label>
          </div>
        </div>
      </div>

      <div className="save-row">
        <button
          className={`btn btn-primary${saved ? ' btn--saved' : ''}`}
          onClick={handleSave}
          disabled={busy || saved}
        >
          {busy ? 'Saving…' : saved ? '✓ Saved!' : 'Save settings'}
        </button>
      </div>
    </>
  )
}

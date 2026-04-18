import { useEffect, useState } from 'react'
import { GetSettings, SaveSettings, SaveCustomSound, GetSoundDataBase64, TestSound } from '../../wailsjs/go/main/App'

export default function Settings() {
  const [s, setS] = useState(null)
  const [notice, setNotice] = useState(null)
  const [customFileName, setCustomFileName] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    GetSettings().then(settings => setS(settings))
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
      showNotice('success', 'Settings saved.')
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

  if (!s) return <div style={{ color: 'var(--text-muted)', padding: 24 }}>Loading settings…</div>

  return (
    <>
      <h1 className="page-title">Settings</h1>

      {notice && <div className={`notice ${notice.type}`}>{notice.msg}</div>}

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
            <button className="btn btn-secondary btn-sm" onClick={() => TestSound()} disabled={!s.soundEnabled}>
              Test sound
            </button>
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

      <div className="save-row">
        <button className="btn btn-primary" onClick={handleSave} disabled={busy}>
          {busy ? 'Saving…' : 'Save settings'}
        </button>
      </div>
    </>
  )
}

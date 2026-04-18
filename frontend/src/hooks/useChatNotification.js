import { useEffect, useRef } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { GetSettings, GetSoundDataBase64 } from '../../wailsjs/go/main/App'

// Default chime — a simple 440 Hz beep generated via Web Audio API as a fallback.
function playDefaultChime(volume = 1) {
  try {
    const ctx = new (window.AudioContext || window.webkitAudioContext)()
    const osc = ctx.createOscillator()
    const gain = ctx.createGain()
    osc.connect(gain)
    gain.connect(ctx.destination)
    osc.type = 'sine'
    osc.frequency.setValueAtTime(880, ctx.currentTime)
    gain.gain.setValueAtTime(volume * 0.4, ctx.currentTime)
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.35)
    osc.start(ctx.currentTime)
    osc.stop(ctx.currentTime + 0.35)
  } catch (e) {
    // AudioContext not available (no user gesture etc.) — ignore
  }
}

export default function useChatNotification() {
  const settingsRef = useRef({ soundEnabled: true, soundVolume: 1, soundPath: '' })
  const customAudioRef = useRef(null) // cached AudioBuffer for custom sound
  const audioCtxRef = useRef(null)
  const lastPlayedRef = useRef(0)
  const cooldownRef = useRef(0)

  // Load settings once and refresh on settings page save
  useEffect(() => {
    async function loadSettings() {
      try {
        const s = await GetSettings()
        settingsRef.current = s
        cooldownRef.current = s.cooldownMs || 0

        if (s.soundPath) {
          await loadCustomSound()
        } else {
          customAudioRef.current = null
        }
      } catch (_) {}
    }

    async function loadCustomSound() {
      try {
        const b64 = await GetSoundDataBase64()
        if (!b64) { customAudioRef.current = null; return }

        const binary = atob(b64)
        const bytes = new Uint8Array(binary.length)
        for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)

        if (!audioCtxRef.current) audioCtxRef.current = new (window.AudioContext || window.webkitAudioContext)()
        const buffer = await audioCtxRef.current.decodeAudioData(bytes.buffer)
        customAudioRef.current = buffer
      } catch (_) {
        customAudioRef.current = null
      }
    }

    loadSettings()

    window.addEventListener('settings:changed', loadSettings)
    return () => window.removeEventListener('settings:changed', loadSettings)
  }, [])

  useEffect(() => {
    const off = EventsOn('chat:message', () => {
      const { soundEnabled, soundVolume } = settingsRef.current
      if (!soundEnabled) return

      const now = Date.now()
      if (now - lastPlayedRef.current < cooldownRef.current) return
      lastPlayedRef.current = now

      if (customAudioRef.current && audioCtxRef.current) {
        try {
          const source = audioCtxRef.current.createBufferSource()
          source.buffer = customAudioRef.current
          const gain = audioCtxRef.current.createGain()
          gain.gain.value = soundVolume
          source.connect(gain)
          gain.connect(audioCtxRef.current.destination)
          source.start()
        } catch (_) {
          playDefaultChime(soundVolume)
        }
      } else {
        playDefaultChime(soundVolume)
      }
    })
    return () => off()
  }, [])
}

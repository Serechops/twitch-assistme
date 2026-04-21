import { useEffect, useRef, useCallback, useState } from 'react'
import { ProcessVoiceCommand, SpeakAnswer, GetSettings } from '../../wailsjs/go/main/App'

// States the voice command system can be in.
export const VoiceState = {
  IDLE: 'idle',
  RECORDING: 'recording',
  PROCESSING: 'processing',
  RESULT: 'result',
  ERROR: 'error',
}

// Silence detection config
const SILENCE_THRESHOLD = 0.015  // RMS below this = silence (0–1 scale)
const SILENCE_DELAY_MS  = 1500   // ms of continuous silence before auto-submit
const MAX_RECORD_MS     = 30000  // hard cap to avoid runaway recordings

/**
 * useVoiceCommand — press Ctrl+Shift+Space to start recording.
 * Auto-submits after SILENCE_DELAY_MS of silence detected via Web Audio API.
 * Manual stop still works via the same shortcut or the Stop button.
 *
 * Returns: { voiceState, result, error, isSupported, dismiss, stopAndSubmit, silenceProgress }
 * silenceProgress: 0–1 indicating how far through the silence countdown we are.
 */
export default function useVoiceCommand() {
  const [voiceState, setVoiceState]       = useState(VoiceState.IDLE)
  const [result, setResult]               = useState(null)
  const [error, setError]                 = useState(null)
  const [silenceProgress, setSilenceProgress] = useState(0) // 0–1

  const mediaRecorderRef  = useRef(null)
  const chunksRef         = useRef([])
  const streamRef         = useRef(null)
  const audioCtxRef       = useRef(null)
  const analyserRef       = useRef(null)
  const rafRef            = useRef(null)   // requestAnimationFrame handle
  const silenceStartRef   = useRef(null)   // timestamp when silence began
  const submittedRef      = useRef(false)  // guard against double-submit
  const maxTimerRef       = useRef(null)

  const isSupported =
    typeof navigator !== 'undefined' &&
    typeof navigator.mediaDevices?.getUserMedia === 'function' &&
    typeof MediaRecorder !== 'undefined'

  const dismiss = useCallback(() => {
    setVoiceState(VoiceState.IDLE)
    setResult(null)
    setError(null)
    setSilenceProgress(0)
  }, [])

  // Tears down the Web Audio silence detector.
  const stopSilenceDetector = useCallback(() => {
    if (rafRef.current) {
      cancelAnimationFrame(rafRef.current)
      rafRef.current = null
    }
    if (audioCtxRef.current) {
      audioCtxRef.current.close().catch(() => {})
      audioCtxRef.current = null
      analyserRef.current = null
    }
    if (maxTimerRef.current) {
      clearTimeout(maxTimerRef.current)
      maxTimerRef.current = null
    }
    silenceStartRef.current = null
    submittedRef.current = false
    setSilenceProgress(0)
  }, [])

  const stopAndSubmit = useCallback(() => {
    const recorder = mediaRecorderRef.current
    if (!recorder || recorder.state === 'inactive') return
    stopSilenceDetector()
    recorder.stop()
    streamRef.current?.getTracks().forEach(t => t.stop())
  }, [stopSilenceDetector])

  // Starts the Web Audio analyser loop that watches for silence.
  const startSilenceDetector = useCallback((stream) => {
    try {
      const ctx      = new AudioContext()
      const source   = ctx.createMediaStreamSource(stream)
      const analyser = ctx.createAnalyser()
      analyser.fftSize = 512
      source.connect(analyser)

      audioCtxRef.current  = ctx
      analyserRef.current  = analyser
      silenceStartRef.current = null
      submittedRef.current = false

      const buf = new Float32Array(analyser.fftSize)

      const tick = () => {
        if (!analyserRef.current) return
        analyser.getFloatTimeDomainData(buf)

        // Root-mean-square of the waveform.
        let sum = 0
        for (let i = 0; i < buf.length; i++) sum += buf[i] * buf[i]
        const rms = Math.sqrt(sum / buf.length)

        const now = performance.now()
        if (rms < SILENCE_THRESHOLD) {
          // Silence — start or continue the countdown.
          if (silenceStartRef.current === null) {
            silenceStartRef.current = now
          }
          const elapsed = now - silenceStartRef.current
          setSilenceProgress(Math.min(elapsed / SILENCE_DELAY_MS, 1))

          if (elapsed >= SILENCE_DELAY_MS && !submittedRef.current) {
            submittedRef.current = true
            stopAndSubmit()
            return // don't schedule another frame
          }
        } else {
          // Speech detected — reset the silence countdown.
          silenceStartRef.current = null
          setSilenceProgress(0)
        }

        rafRef.current = requestAnimationFrame(tick)
      }

      rafRef.current = requestAnimationFrame(tick)
    } catch {
      // Web Audio not available — silence detection simply won't run;
      // the user can still stop manually.
    }
  }, [stopAndSubmit])

  const startRecording = useCallback(async () => {
    if (!isSupported) {
      setError('Microphone access is not supported in this environment.')
      setVoiceState(VoiceState.ERROR)
      return
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      streamRef.current = stream
      chunksRef.current = []

      // Prefer WebM/Opus (Whisper-compatible); fall back to whatever is supported.
      const mimeType = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
        ? 'audio/webm;codecs=opus'
        : MediaRecorder.isTypeSupported('audio/webm')
        ? 'audio/webm'
        : ''

      const recorder = new MediaRecorder(stream, mimeType ? { mimeType } : undefined)
      mediaRecorderRef.current = recorder

      recorder.ondataavailable = e => {
        if (e.data.size > 0) chunksRef.current.push(e.data)
      }

      recorder.onstop = async () => {
        stopSilenceDetector()
        setVoiceState(VoiceState.PROCESSING)
        try {
          const blob        = new Blob(chunksRef.current, { type: recorder.mimeType || 'audio/webm' })
          const arrayBuffer = await blob.arrayBuffer()
          const bytes       = new Uint8Array(arrayBuffer)

          // Convert to base64 for the Wails bridge.
          let binary = ''
          for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i])
          const base64 = btoa(binary)

          const commandResult = await ProcessVoiceCommand(base64)
          setResult(commandResult)
          setVoiceState(VoiceState.RESULT)

          // Play TTS for game guide answers if voice feedback is enabled.
          if (commandResult?.message) {
            try {
              const settings = await GetSettings()
              if (settings?.voiceFeedback) {
                const b64   = await SpeakAnswer(commandResult.message)
                const bytes = Uint8Array.from(atob(b64), c => c.charCodeAt(0))
                const blob  = new Blob([bytes], { type: 'audio/mpeg' })
                const url   = URL.createObjectURL(blob)
                const audio = new Audio(url)
                audio.onended = () => URL.revokeObjectURL(url)
                audio.onerror = () => URL.revokeObjectURL(url)
                await audio.play()
              }
            } catch (ttsErr) {
              console.error('TTS playback error:', ttsErr)
            }
          }
        } catch (err) {
          setError(err?.message ?? String(err))
          setVoiceState(VoiceState.ERROR)
        }
      }

      recorder.start()
      setVoiceState(VoiceState.RECORDING)

      // Start silence detection immediately.
      startSilenceDetector(stream)

      // Hard cap — auto-submit after MAX_RECORD_MS regardless.
      maxTimerRef.current = setTimeout(() => {
        if (!submittedRef.current) stopAndSubmit()
      }, MAX_RECORD_MS)
    } catch (err) {
      setError(err?.message ?? 'Microphone access denied.')
      setVoiceState(VoiceState.ERROR)
    }
  }, [isSupported, startSilenceDetector, stopAndSubmit, stopSilenceDetector])

  // Toggle: Ctrl+Shift+Space starts / stops recording.
  useEffect(() => {
    const handleKeyDown = e => {
      if (e.code === 'Space' && e.ctrlKey && e.shiftKey && !e.repeat) {
        e.preventDefault()
        if (voiceState === VoiceState.IDLE || voiceState === VoiceState.RESULT || voiceState === VoiceState.ERROR) {
          startRecording()
        } else if (voiceState === VoiceState.RECORDING) {
          stopAndSubmit()
        }
      }
      // Escape dismisses the overlay.
      if (e.code === 'Escape') {
        if (voiceState === VoiceState.RECORDING) {
          stopSilenceDetector()
          mediaRecorderRef.current?.stop()
          streamRef.current?.getTracks().forEach(t => t.stop())
        }
        dismiss()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [voiceState, startRecording, stopAndSubmit, stopSilenceDetector, dismiss])

  // Cleanup on unmount.
  useEffect(() => {
    return () => {
      stopSilenceDetector()
      mediaRecorderRef.current?.stop()
      streamRef.current?.getTracks().forEach(t => t.stop())
    }
  }, [stopSilenceDetector])

  return { voiceState, result, error, isSupported, dismiss, stopAndSubmit, silenceProgress }
}

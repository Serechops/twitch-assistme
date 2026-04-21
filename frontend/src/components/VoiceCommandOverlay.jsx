import { VoiceState } from '../hooks/useVoiceCommand'
import ReactMarkdown from 'react-markdown'
import './VoiceCommandOverlay.css'

const ACTION_LABELS = {
  create_poll: 'Poll created',
  start_raid: 'Raid started',
  cancel_raid: 'Raid cancelled',
  update_stream_title: 'Title updated',
  update_stream_game: 'Game/category updated',
  create_channel_point_reward: 'Reward created',
}

// SVG arc that fills clockwise as silenceProgress goes 0 → 1.
function SilenceArc({ progress }) {
  const r  = 34
  const cx = 40
  const cy = 40
  const circumference = 2 * Math.PI * r
  const offset = circumference * (1 - progress)
  return (
    <svg className="vc-silence-arc" viewBox="0 0 80 80" aria-hidden="true">
      <circle cx={cx} cy={cy} r={r} className="vc-silence-arc-track" />
      <circle
        cx={cx} cy={cy} r={r}
        className="vc-silence-arc-fill"
        strokeDasharray={circumference}
        strokeDashoffset={offset}
        style={{ transition: progress === 0 ? 'none' : 'stroke-dashoffset 0.05s linear' }}
      />
    </svg>
  )
}

export default function VoiceCommandOverlay({ voiceState, result, error, dismiss, stopAndSubmit, silenceProgress }) {
  if (voiceState === VoiceState.IDLE) return null

  return (
    <div className="vc-overlay" role="dialog" aria-live="polite" onClick={e => e.target === e.currentTarget && dismiss()}>
      <div className="vc-card">
        {voiceState === VoiceState.RECORDING && (
          <>
            <div className="vc-mic-wrapper">
              {silenceProgress > 0
                ? <SilenceArc progress={silenceProgress} />
                : <div className="vc-pulse-ring" />}
              <div className="vc-mic-icon">🎙️</div>
            </div>
            <p className="vc-label">Listening&hellip;</p>
            <p className="vc-hint">
              {silenceProgress > 0
                ? 'Sending when quiet\u2026'
                : <span>Press <kbd>Ctrl+Shift+Space</kbd> to send early</span>}
            </p>
            <button className="vc-stop-btn" onClick={stopAndSubmit}>Stop &amp; Send</button>
          </>
        )}

        {voiceState === VoiceState.PROCESSING && (
          <>
            <div className="vc-spinner" />
            <p className="vc-label">Processing command&hellip;</p>
          </>
        )}

        {voiceState === VoiceState.RESULT && result && (
          <>
            <div className="vc-icon-success">&#x2705;</div>
            {result.transcript && (
              <p className="vc-transcript">&ldquo;{result.transcript}&rdquo;</p>
            )}
            {result.actions && result.actions.length > 0 && (
              <div className="vc-actions">
                {result.actions.map((a, i) => (
                  <span key={i} className="vc-action-badge">
                    {ACTION_LABELS[a] ?? a}
                  </span>
                ))}
              </div>
            )}
            {result.message && (
              <div className="vc-message">
                <ReactMarkdown
                  components={{
                    a: ({ href, children }) => (
                      <a href={href} target="_blank" rel="noreferrer" style={{ color: 'var(--accent)' }}>{children}</a>
                    ),
                    p: ({ children }) => <span>{children}</span>,
                  }}
                >{result.message}</ReactMarkdown>
              </div>
            )}
            <button className="vc-dismiss-btn" onClick={dismiss}>Dismiss</button>
          </>
        )}

        {voiceState === VoiceState.ERROR && (
          <>
            <div className="vc-icon-error">&#x26A0;&#xFE0F;</div>
            <p className="vc-label">Command failed</p>
            <p className="vc-error-msg">{error}</p>
            <button className="vc-dismiss-btn" onClick={dismiss}>Dismiss</button>
          </>
        )}

      </div>
    </div>
  )
}

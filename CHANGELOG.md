# Changelog

All notable changes to **Twitch AssistMe** are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [Unreleased]

---

## [1.1.0] – 2026-04-20

### Added
- **Predictions page** (`/predictions`) — full lifecycle management for channel predictions
  - Create tab: 2–10 outcomes, flexible duration picker
  - Active prediction card: lock, resolve (winner picker), or cancel in-flight
  - History tab: past predictions with outcomes and point totals
  - Live updates via `channel.prediction.begin/progress/lock/end` EventSub subscriptions
- **Tools page** (`/tools`) — broadcaster utility panel
  - Announcement sender with color picker (blue / green / orange / purple / primary)
  - Shoutout with live channel search suggestions
  - Stream Marker creator with description field
- **Clips page** (`/clips`) — clip creation and management
  - Create tab: clip now button, optional delay toggle, AI-generated title suggestion after creation
  - My Clips tab: 16:9 thumbnail grid with view count, duration, date, Watch + Edit on Twitch links
  - "Clip that" / "make a clip" voice commands trigger clip creation hands-free
- **Dashboard stats cards** — Creator Goals progress bars and Hype Train level/progress card (shown only when active)
- **AI Voice Command system** — global, always-on voice assistant
  - Global hotkey: Win32 `RegisterHotKey` (keyboard combos) + `WH_MOUSE_LL` hook (mouse buttons 3/4/5); fires silently without stealing window focus
  - System tray icon: closing the window hides to tray; right-click menu exposes "Show Window" and "Quit"
  - `VoiceCommandOverlay` component: floating modal shown during recording, processing, and result display
  - Voice commands: create clip, start/end poll, start/cancel raid, and more
- **Game Guide** (in Tools page) — in-game AI assistant
  - Voice trigger: prefix a voice command with "game guide" to route it to the game guide AI
  - Game-aware web search: question rewritten to embed the active game name for accurate results
  - Powered by `gpt-4.1-mini` via the OpenAI Responses API with forced `web_search_preview`
  - Refusal detector: clears session and retries if the model skips the web search
- **Text-to-Speech** — AI answers read aloud
  - `SpeakAnswer` backend method streams MP3 audio from OpenAI TTS and returns it as base64
  - Auto-plays after voice commands and game guide responses when "Voice Feedback" is enabled in Settings
  - `cleanTextForTTS` pre-processor strips Markdown (links, code blocks, headings, bold/italic, bullets), bare URLs, and HTML tags before synthesis so only natural prose is spoken
- **Markdown rendering** — `react-markdown@8` renders assistant replies in both the VoiceCommandOverlay and Game Guide chat bubbles
- **HotkeyRecorder** in Settings — visual keyboard/mouse binding UI; records combos live and persists `voice_hotkey` to the database
- **VirusTotal integration** in release script — scans the built `.exe` before publishing; SHA-256 cache avoids redundant uploads; appends scan summary and full report link to GitHub release notes; gracefully skips when `VT_API_KEY` is not set; ≤2 detections treated as clean (ML false-positive threshold)

### Changed
- Closing the main window now minimizes to the system tray instead of exiting (`HideWindowOnClose: true`)
- Global hotkey trigger no longer brings the window to the front — recording starts silently in the background
- Game guide upgraded from `gpt-4o` to `gpt-4.1-mini`; `tool_choice` pinned to `web_search_preview` to eliminate training-data-only responses
- Release script uploads the `.exe` directly (previously zipped)

### Fixed
- Game guide no longer silently passes "unknown game" to the AI — returns a clear error when stream category is not set
- Stale `previous_response_id` from a failed/refused turn no longer poisons subsequent game guide requests
- Post-response guard returns a retryable error if `web_search_call` is absent in the model output

### New OAuth Scopes (re-authentication required)
- `channel:manage:predictions`
- `moderator:manage:announcements`
- `moderator:manage:shoutouts`
- `channel:read:goals`
- `channel:read:hype_train`
- `clips:edit`

---

## [1.0.0] – 2026-04-19

### Added
- **Stream Info page** (`/stream-info`) — view and edit live channel title, category, broadcaster language, and tags
  - Debounced category search with box-art dropdown (GET /helix/search/categories)
  - Tag manager with alphanumeric validation, 25-char limit, max 10 tags, no duplicates
  - Loads current values on mount via GET /helix/channels
  - Updates channel via PATCH /helix/channels (`channel:manage:broadcast` scope)
- **Save button feedback** — save buttons in Stream Info and Settings turn green and show "✓ Saved!" for 2.5 s on success instead of a status message; errors still surface as inline text
- **Raids page** (`/raids`) with enriched raid cards — avatar, live stream preview thumbnail, category tags, and formatted uptime
- **Polls page** (`/polls`) with archive and reusable templates
  - Poll templates can be saved and applied to new polls in one click
  - Archive shows all past polls with status badges (completed / archived / terminated)
- **Live poll viewer on Dashboard** — active poll card with real-time vote bars and an End Poll button
  - Poll progress updates via `channel.poll.progress` EventSub subscription
- **Poll creator on Dashboard** — title, 2–5 choices, duration preset buttons (1 / 2 / 5 / 10 min)
- **Settings** — static Twitch banner image (`twitch_banner.jpg`) used as profile banner
- **Device Code Grant Flow** login replacing PKCE, with improved login UX and progress feedback
- **Session persistence** — token and broadcaster ID stored locally; app auto-connects on restart
- Scope `channel:manage:polls` for poll management
- Scope `channel:manage:raids` for raid management
- Scope `channel:manage:broadcast` for channel info updates

### Changed
- Dashboard stripped of poll UI; polls moved to dedicated Polls page
- Poll duration input replaced with preset buttons (1 / 2 / 5 / 10 min)
- Account card banner height increased from 96 px to 160 px in Settings
- Re-auth forced automatically when the stored token is missing required scopes

### Fixed
- DCF one-time-use refresh token handled correctly
- Silent re-auth on logout and app restart
- Twitch client ID updated to correct app registration
- `.env` loading works from both the exe directory and cwd (for `wails dev` compatibility)

---

## [0.1.0] – 2026-04-18

### Added
- Initial project scaffold (Wails v2 + React + Go)
- PKCE OAuth flow (later replaced by Device Code Grant)
- Profile images, custom notification sounds, and dual OAuth flow
- Settings change event propagation for live chat notification updates
- App icon and logo rebrand throughout

---

[Unreleased]: https://github.com/your-org/twitch-assistme/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/your-org/twitch-assistme/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/your-org/twitch-assistme/compare/v0.1.0...v1.0.0
[0.1.0]: https://github.com/your-org/twitch-assistme/releases/tag/v0.1.0

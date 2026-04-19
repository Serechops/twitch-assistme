# Changelog

All notable changes to **Twitch AssistMe** are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [Unreleased]

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

[Unreleased]: https://github.com/your-org/twitch-assistme/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/your-org/twitch-assistme/compare/v0.1.0...v1.0.0
[0.1.0]: https://github.com/your-org/twitch-assistme/releases/tag/v0.1.0

# Twitch AssistMe

A lightweight Windows desktop app for Twitch streamers. Get audible notifications for every new chat message — so you never miss a message while focused on your stream.

Built with [Wails v2](https://wails.io), Go, React, and SQLite. No Electron, no browser tab, no OBS plugin required.

---

## Features

- **Chat notifications** — plays a sound every time a new message arrives in your Twitch chat via EventSub WebSocket
- **Custom notification sound** — upload your own MP3, WAV, OGG, or M4A file, or revert to the built-in chime at any time
- **Volume control** — per-notification volume slider with a live test button that previews the exact volume before saving
- **Chat filters** — optionally ignore your own messages; configurable cooldown to prevent rapid-fire sound spam
- **Live chat feed** — scrolling message feed in the dashboard with chatter names
- **Account card** — displays your Twitch profile picture and channel banner on the Settings page
- **Persistent auth** — tokens are stored locally and refreshed automatically; login is required only once (or once per 30 days with the Device Code flow)

---

## Tech Stack

| Layer | Technology |
|---|---|
| Desktop framework | Wails v2 (Go + WebView2) |
| Backend | Go 1.25, Windows/amd64 |
| Frontend | React 18, Vite 3, plain CSS |
| Database | SQLite via `modernc.org/sqlite` (pure Go, no CGO) |
| Twitch realtime | EventSub WebSocket (`gorilla/websocket`) |
| Twitch auth | Authorization Code flow (confidential) or Device Code Grant Flow (public) |

---

## Prerequisites

- Windows 10/11 (WebView2 is pre-installed on Windows 11; Windows 10 may prompt to install it)
- [Go 1.21+](https://go.dev/dl/)
- [Node.js 18+](https://nodejs.org/)
- [Wails CLI](https://wails.io/docs/gettingstarted/installation): `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

---

## Twitch App Setup

1. Go to [https://dev.twitch.tv/console/apps](https://dev.twitch.tv/console/apps) and create a new application.
2. Set the **OAuth Redirect URL** to `http://localhost:3333`.
3. For the best login experience (single browser click, no activation code):
   - Set the **Client type** to **Confidential**.
   - Copy the **Client Secret**.
   - In `app.go`, fill in the constants:
     ```go
     const twitchClientID     = "your_client_id"
     const twitchClientSecret = "your_client_secret"
     ```
4. For an open-source / public build (no secret in source):
   - Leave `twitchClientSecret = ""`.
   - The app automatically falls back to the **Device Code Grant Flow** — a one-time browser activation step is required on each login (tokens last 30 days).

---

## Development

```bash
wails dev
```

Hot-reloads the React frontend on save. The Go backend recompiles automatically. A browser dev server is also available at `http://localhost:34115`.

---

## Building

```bash
wails build
```

Produces a single `.exe` in `build/bin/`. No installer or runtime dependencies required on the target machine beyond WebView2.

---

## Project Structure

```
.
├── app.go                      # Wails app struct — all backend methods exposed to frontend
├── main.go                     # Entry point, Wails config
├── internal/
│   ├── auth/
│   │   ├── oauth.go            # Device Code Grant Flow + token refresh + validate
│   │   └── authcode.go         # Authorization Code flow (state, PKCE-free, localhost redirect)
│   ├── db/
│   │   └── db.go               # SQLite — auth tokens, settings
│   └── twitch/
│       ├── client.go           # Helix REST (GetCurrentUser)
│       └── eventsub.go         # EventSub WebSocket client with auto-reconnect
└── frontend/
    └── src/
        ├── App.jsx             # Shell — sidebar, routing, auth state
        ├── pages/
        │   ├── Dashboard.jsx   # Chat feed, connect/disconnect, account card
        │   └── Settings.jsx    # Sound, filters, account, save
        ├── hooks/
        │   ├── useChatNotification.js   # Web Audio API — plays sound on chat:message
        │   └── useConnectionStatus.js  # EventSub status listener
        └── styles/
            └── main.css        # Dark theme, all component styles
```

---

## Data Storage

Settings and tokens are stored in `%APPDATA%\TwitchStreamerTools\`:

- `twitch_tools.db` — SQLite database (auth tokens, settings)
- `sounds/custom_notification.*` — custom notification sound file (if uploaded)

No data is sent anywhere other than the Twitch API.

---

## License

MIT

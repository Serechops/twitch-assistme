package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"ai-ssistme/internal/auth"
	"ai-ssistme/internal/db"
	twitch "ai-ssistme/internal/twitch"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// twitchClientID is the public Twitch application client ID.
// Safe to embed in distributed binaries — it identifies the app, not a user.
const twitchClientID = "qclbf55wgzujy2rnsqqc88dv3re3yp"

// twitchClientSecret is optional. When set, enables the smoother Authorization
// Code flow. When empty (default), the app uses Device Code Grant Flow which
// works out of the box with no setup required for end users.
// Set via TWITCH_AISSISTME_SECRET_KEY in a .env file (never distribute this).
var twitchClientSecret = os.Getenv("TWITCH_AISSISTME_SECRET_KEY")

const (
	twitchRedirectURI = "http://localhost:3333"
	twitchScopes      = "user:read:chat"
)

// App is the main application struct bound to the Wails frontend.
type App struct {
	ctx      context.Context
	database *db.DB

	eventSubClient *twitch.EventSubClient
	eventSubCancel context.CancelFunc
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	d, err := db.Open()
	if err != nil {
		runtime.LogErrorf(ctx, "Failed to open database: %v", err)
		return
	}
	a.database = d
}

// domReady is called once the frontend DOM is ready to receive events.
// It restores the previous session (refreshing the token if needed) and
// auto-connects EventSub so the user never has to log in or click Connect again.
func (a *App) domReady(ctx context.Context) {
	if a.database == nil {
		return
	}

	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.UserID == "" {
		return // no saved session — login screen is already shown
	}

	// If the access token has expired (or is within 60 s of expiry), refresh it.
	if time.Now().Unix() >= row.ExpiresAt-60 {
		if row.RefreshToken == "" {
			// Cannot refresh — force re-login.
			_ = a.database.ClearAuth()
			runtime.EventsEmit(a.ctx, "auth:changed", nil)
			return
		}
		tokens, rerr := auth.RefreshAccessToken(twitchClientID, twitchClientSecret, row.RefreshToken)
		if rerr != nil {
			runtime.LogWarningf(a.ctx, "Startup token refresh failed: %v", rerr)
			_ = a.database.ClearAuth()
			runtime.EventsEmit(a.ctx, "auth:changed", nil)
			return
		}
		row.AccessToken = tokens.AccessToken
		row.RefreshToken = tokens.RefreshToken
		row.ExpiresAt = time.Now().Unix() + int64(tokens.ExpiresIn)
		_ = a.database.SaveAuth(*row)
	}

	// Emit auth:changed so the frontend is in sync with the persisted session.
	runtime.EventsEmit(a.ctx, "auth:changed", &UserInfo{
		ID:              row.UserID,
		Login:           row.UserLogin,
		DisplayName:     row.UserDisplayName,
		ProfileImageURL: row.ProfileImageURL,
		OfflineImageURL: row.OfflineImageURL,
	})

	// Auto-reconnect EventSub — user should never have to click Connect manually.
	if err := a.ConnectEventSub(); err != nil {
		runtime.LogWarningf(a.ctx, "Auto-connect EventSub on startup failed: %v", err)
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.eventSubCancel != nil {
		a.eventSubCancel()
	}
	if a.database != nil {
		a.database.Close()
	}
}

// ─── Auth ────────────────────────────────────────────────────────────────────

// UserInfo is the data returned to the frontend about the authenticated user.
type UserInfo struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"displayName"`
	ProfileImageURL string `json:"profileImageUrl"`
	OfflineImageURL string `json:"offlineImageUrl"`
}

// Login starts the appropriate OAuth flow:
//   - If twitchClientSecret is set, uses Authorization Code flow (browser authorize + localhost redirect).
//   - Otherwise, falls back to Device Code Grant Flow (browser activation page).
func (a *App) Login() error {
	if twitchClientSecret != "" {
		return a.loginAuthCode()
	}
	return a.loginDeviceFlow()
}

func (a *App) loginAuthCode() error {
	state, err := auth.GenerateState()
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	authURL := auth.BuildAuthURL(twitchClientID, twitchRedirectURI, state, twitchScopes)
	runtime.BrowserOpenURL(a.ctx, authURL)

	code, err := auth.ListenForCallback(a.ctx, state, "3333")
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	tokens, err := auth.ExchangeCode(twitchClientID, twitchClientSecret, twitchRedirectURI, code)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	return a.finishLogin(tokens)
}

// DeviceLoginState holds pending device flow state between StartLogin and polling.
type DeviceLoginState struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	Interval        int
}

var pendingDeviceLogin *DeviceLoginState

// StartLogin begins the auth flow:
//   - Auth Code flow: opens browser immediately, returns empty string (no code needed).
//   - Device Code flow: returns the user code to display, opens browser to activation page.
//
// After calling StartLogin, call PollLogin to wait for completion.
func (a *App) StartLogin() (string, error) {
	if twitchClientSecret != "" {
		return "", a.loginAuthCode()
	}

	dfr, err := auth.StartDeviceFlow(twitchClientID, twitchScopes)
	if err != nil {
		return "", fmt.Errorf("login: %w", err)
	}

	pendingDeviceLogin = &DeviceLoginState{
		DeviceCode:      dfr.DeviceCode,
		UserCode:        dfr.UserCode,
		VerificationURI: dfr.VerificationURI,
		Interval:        dfr.Interval,
	}

	runtime.BrowserOpenURL(a.ctx, dfr.VerificationURI)
	return dfr.UserCode, nil
}

// PollLogin waits for the pending device flow to complete (blocking).
// Only call this after StartLogin returned a user code.
func (a *App) PollLogin() error {
	if pendingDeviceLogin == nil {
		return fmt.Errorf("no pending login")
	}
	p := pendingDeviceLogin
	pendingDeviceLogin = nil

	tokens, err := auth.PollForToken(a.ctx, twitchClientID, p.DeviceCode, p.Interval)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	return a.finishLogin(tokens)
}

func (a *App) loginDeviceFlow() error {
	dfr, err := auth.StartDeviceFlow(twitchClientID, twitchScopes)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	runtime.BrowserOpenURL(a.ctx, dfr.VerificationURI)

	tokens, err := auth.PollForToken(a.ctx, twitchClientID, dfr.DeviceCode, dfr.Interval)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	return a.finishLogin(tokens)
}

func (a *App) finishLogin(tokens *auth.TokenResponse) error {
	user, err := twitch.GetCurrentUser(twitchClientID, tokens.AccessToken)
	if err != nil {
		return fmt.Errorf("login: get user: %w", err)
	}

	row := db.AuthRow{
		AccessToken:     tokens.AccessToken,
		RefreshToken:    tokens.RefreshToken,
		ExpiresAt:       time.Now().Unix() + int64(tokens.ExpiresIn),
		UserID:          user.ID,
		UserLogin:       user.Login,
		UserDisplayName: user.DisplayName,
		ProfileImageURL: user.ProfileImageURL,
		OfflineImageURL: user.OfflineImageURL,
		CreatedAt:       time.Now().Unix(),
	}
	if err := a.database.SaveAuth(row); err != nil {
		return fmt.Errorf("login: save auth: %w", err)
	}

	runtime.EventsEmit(a.ctx, "auth:changed", a.buildUserInfo(row, user.ProfileImageURL, user.OfflineImageURL))

	// Auto-connect EventSub immediately after login.
	if err := a.ConnectEventSub(); err != nil {
		runtime.LogWarningf(a.ctx, "Auto-connect EventSub after login failed: %v", err)
	}
	return nil
}

// Logout clears auth, disconnects EventSub, and emits auth:changed.
func (a *App) Logout() error {
	if a.eventSubCancel != nil {
		a.eventSubCancel()
		a.eventSubCancel = nil
		a.eventSubClient = nil
	}
	if err := a.database.ClearAuth(); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "auth:changed", nil)
	runtime.EventsEmit(a.ctx, "eventsub:status", twitch.StatusDisconnected)
	return nil
}

// GetUser returns the stored user info, or nil if not authenticated.
func (a *App) GetUser() *UserInfo {
	if a.database == nil {
		return nil
	}
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.UserID == "" {
		return nil
	}
	return &UserInfo{
		ID:              row.UserID,
		Login:           row.UserLogin,
		DisplayName:     row.UserDisplayName,
		ProfileImageURL: row.ProfileImageURL,
		OfflineImageURL: row.OfflineImageURL,
	}
}

// IsAuthenticated returns true if a valid auth token is stored.
func (a *App) IsAuthenticated() bool {
	return a.GetUser() != nil
}

func (a *App) buildUserInfo(row db.AuthRow, profileImageURL, offlineImageURL string) *UserInfo {
	return &UserInfo{
		ID:              row.UserID,
		Login:           row.UserLogin,
		DisplayName:     row.UserDisplayName,
		ProfileImageURL: profileImageURL,
		OfflineImageURL: offlineImageURL,
	}
}

// ─── EventSub ────────────────────────────────────────────────────────────────

// ConnectEventSub starts the Twitch EventSub WebSocket connection.
func (a *App) ConnectEventSub() error {
	if a.eventSubCancel != nil {
		// Already connected — disconnect first.
		a.eventSubCancel()
		a.eventSubCancel = nil
	}

	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return fmt.Errorf("not authenticated")
	}

	// Reload filter settings from DB.
	ignoreOwn, _ := a.database.GetSetting("chat_filter_ignore_own")
	cooldownStr, _ := a.database.GetSetting("chat_filter_cooldown_ms")
	cooldownMs, _ := strconv.ParseInt(cooldownStr, 10, 64)

	client := twitch.NewEventSubClient(twitchClientID, row.AccessToken, row.UserID, row.UserID)
	client.IgnoreOwnMessages = ignoreOwn == "true"
	client.CooldownMs = cooldownMs

	client.OnStatus = func(status string) {
		runtime.EventsEmit(a.ctx, "eventsub:status", status)
	}
	client.OnChatMessage = func(evt twitch.ChatMessageEvent) {
		runtime.EventsEmit(a.ctx, "chat:message", evt)
	}

	ctx, cancel := context.WithCancel(a.ctx)
	a.eventSubClient = client
	a.eventSubCancel = cancel

	go client.Connect(ctx)
	return nil
}

// DisconnectEventSub stops the EventSub connection.
func (a *App) DisconnectEventSub() {
	if a.eventSubCancel != nil {
		a.eventSubCancel()
		a.eventSubCancel = nil
		a.eventSubClient = nil
		runtime.EventsEmit(a.ctx, "eventsub:status", twitch.StatusDisconnected)
	}
}

// GetConnectionStatus returns the current EventSub connection status.
func (a *App) GetConnectionStatus() string {
	if a.eventSubClient == nil {
		return twitch.StatusDisconnected
	}
	return a.eventSubClient.GetStatus()
}

// ─── Settings ────────────────────────────────────────────────────────────────

// SettingsDTO is the data transferred to/from the frontend for settings.
type SettingsDTO struct {
	SoundEnabled bool    `json:"soundEnabled"`
	SoundPath    string  `json:"soundPath"`
	SoundVolume  float64 `json:"soundVolume"`
	IgnoreOwn    bool    `json:"ignoreOwn"`
	CooldownMs   int64   `json:"cooldownMs"`
}

// GetSettings loads current settings from the database.
func (a *App) GetSettings() SettingsDTO {
	getBool := func(key string) bool {
		v, _ := a.database.GetSetting(key)
		return v == "true"
	}
	getFloat := func(key string) float64 {
		v, _ := a.database.GetSetting(key)
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	getInt64 := func(key string) int64 {
		v, _ := a.database.GetSetting(key)
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	getString := func(key string) string {
		v, _ := a.database.GetSetting(key)
		return v
	}

	return SettingsDTO{
		SoundEnabled: getBool("chat_sound_enabled"),
		SoundPath:    getString("chat_sound_path"),
		SoundVolume:  getFloat("chat_sound_volume"),
		IgnoreOwn:    getBool("chat_filter_ignore_own"),
		CooldownMs:   getInt64("chat_filter_cooldown_ms"),
	}
}

// SaveSettings persists settings and applies filter changes to the live client.
func (a *App) SaveSettings(s SettingsDTO) error {
	boolStr := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}

	saves := map[string]string{
		"chat_sound_enabled":      boolStr(s.SoundEnabled),
		"chat_sound_volume":       strconv.FormatFloat(s.SoundVolume, 'f', 2, 64),
		"chat_filter_ignore_own":  boolStr(s.IgnoreOwn),
		"chat_filter_cooldown_ms": strconv.FormatInt(s.CooldownMs, 10),
	}
	for k, v := range saves {
		if err := a.database.SaveSetting(k, v); err != nil {
			return err
		}
	}

	// Apply filter changes to running client without reconnecting.
	if a.eventSubClient != nil {
		a.eventSubClient.IgnoreOwnMessages = s.IgnoreOwn
		a.eventSubClient.CooldownMs = s.CooldownMs
	}
	return nil
}

// SaveCustomSound decodes a base64-encoded audio file and stores it in the app data dir.
func (a *App) SaveCustomSound(base64Data, filename string) error {
	decoded, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("save sound: decode: %w", err)
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	soundDir := filepath.Join(configDir, "TwitchStreamerTools", "sounds")
	if err := os.MkdirAll(soundDir, 0700); err != nil {
		return err
	}

	// Only allow audio file extensions.
	ext := filepath.Ext(filename)
	allowed := map[string]bool{".mp3": true, ".wav": true, ".ogg": true, ".m4a": true}
	if !allowed[ext] {
		return fmt.Errorf("save sound: unsupported file type %q", ext)
	}

	savePath := filepath.Join(soundDir, "custom_notification"+ext)
	if err := os.WriteFile(savePath, decoded, 0600); err != nil {
		return fmt.Errorf("save sound: write: %w", err)
	}

	return a.database.SaveSetting("chat_sound_path", savePath)
}

// ClearCustomSound removes the custom notification sound and reverts to the default.
func (a *App) ClearCustomSound() error {
	path, _ := a.database.GetSetting("chat_sound_path")
	if path != "" {
		os.Remove(path) // best-effort; ignore error if already gone
	}
	return a.database.SaveSetting("chat_sound_path", "")
}

// GetSoundDataBase64 returns the custom notification sound as a base64 string,
// or an empty string if no custom sound is configured.
func (a *App) GetSoundDataBase64() string {
	path, _ := a.database.GetSetting("chat_sound_path")
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

// TestSound emits a fake chat:message event so the frontend plays the notification sound.
func (a *App) TestSound() {
	runtime.EventsEmit(a.ctx, "chat:message", twitch.ChatMessageEvent{
		ChatterUserName:     "TestUser",
		BroadcasterUserName: "You",
	})
}

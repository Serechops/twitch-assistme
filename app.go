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

	// Attempt silent token refresh on startup.
	a.tryRefreshToken()
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

func (a *App) loginDeviceFlow() error {
	dfr, err := auth.StartDeviceFlow(twitchClientID, twitchScopes)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	// VerificationURI includes the device code, so the user just clicks Authorize.
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

func (a *App) tryRefreshToken() {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.RefreshToken == "" {
		return
	}

	// Only refresh if within 30 minutes of expiry or already expired.
	if time.Now().Unix() < row.ExpiresAt-1800 {
		return
	}

	tokens, err := auth.RefreshAccessToken(twitchClientID, twitchClientSecret, row.RefreshToken)
	if err != nil {
		runtime.LogWarningf(a.ctx, "Token refresh failed: %v", err)
		return
	}

	row.AccessToken = tokens.AccessToken
	row.RefreshToken = tokens.RefreshToken
	row.ExpiresAt = time.Now().Unix() + int64(tokens.ExpiresIn)
	if err := a.database.SaveAuth(*row); err != nil {
		runtime.LogWarningf(a.ctx, "Save refreshed token failed: %v", err)
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

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

// App is the main application struct bound to the Wails frontend.
type App struct {
	ctx      context.Context
	database *db.DB

	clientID     string
	clientSecret string

	eventSubClient *twitch.EventSubClient
	eventSubCancel context.CancelFunc
}

// NewApp creates the App with Twitch credentials loaded from env.
func NewApp() *App {
	return &App{
		clientID:     os.Getenv("TWITCH_AISSISTME_CLIENT_ID"),
		clientSecret: os.Getenv("TWITCH_AISSISTME_SECRET_KEY"),
	}
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
}

// Login starts the PKCE OAuth flow, opens the browser, waits for callback,
// fetches user info, and persists the token to the database.
func (a *App) Login() error {
	verifier, err := auth.GenerateCodeVerifier()
	if err != nil {
		return fmt.Errorf("login: generate verifier: %w", err)
	}

	challenge := auth.CodeChallenge(verifier)

	state, err := auth.GenerateState()
	if err != nil {
		return fmt.Errorf("login: generate state: %w", err)
	}

	authURL := auth.BuildAuthURL(a.clientID, state, challenge)
	runtime.BrowserOpenURL(a.ctx, authURL)

	code, err := auth.ListenForCallback(a.ctx, state)
	if err != nil {
		return fmt.Errorf("login: callback: %w", err)
	}

	tokens, err := auth.ExchangeCode(a.clientID, a.clientSecret, code, verifier)
	if err != nil {
		return fmt.Errorf("login: exchange code: %w", err)
	}

	user, err := twitch.GetCurrentUser(a.clientID, tokens.AccessToken)
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
		CreatedAt:       time.Now().Unix(),
	}
	if err := a.database.SaveAuth(row); err != nil {
		return fmt.Errorf("login: save auth: %w", err)
	}

	runtime.EventsEmit(a.ctx, "auth:changed", a.buildUserInfo(row, user.ProfileImageURL))
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
		ID:          row.UserID,
		Login:       row.UserLogin,
		DisplayName: row.UserDisplayName,
	}
}

// IsAuthenticated returns true if a valid auth token is stored.
func (a *App) IsAuthenticated() bool {
	return a.GetUser() != nil
}

func (a *App) buildUserInfo(row db.AuthRow, profileImageURL string) *UserInfo {
	return &UserInfo{
		ID:              row.UserID,
		Login:           row.UserLogin,
		DisplayName:     row.UserDisplayName,
		ProfileImageURL: profileImageURL,
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

	tokens, err := auth.RefreshAccessToken(a.clientID, a.clientSecret, row.RefreshToken)
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

	client := twitch.NewEventSubClient(a.clientID, row.AccessToken, row.UserID, row.UserID)
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

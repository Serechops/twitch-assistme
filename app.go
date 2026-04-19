package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ai-ssistme/internal/auth"
	"ai-ssistme/internal/db"
	twitch "ai-ssistme/internal/twitch"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// twitchClientID is the public Twitch application client ID.
// Safe to embed in distributed binaries — it identifies the app, not a user.
// twitchClientSecret enables Authorization Code flow (confidential client).
// Both are defined in secrets.go (gitignored — never committed).

const (
	twitchRedirectURI = "http://localhost:3333"
	twitchScopes      = "user:read:chat channel:manage:polls"
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

// hasRequiredScopes returns true if all space-separated required scopes are
// present in the token's actual scope list.
func hasRequiredScopes(tokenScopes []string, required string) bool {
	have := make(map[string]bool, len(tokenScopes))
	for _, s := range tokenScopes {
		have[s] = true
	}
	for _, s := range strings.Fields(required) {
		if !have[s] {
			return false
		}
	}
	return true
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
//
// NOTE: domReady only restores sessions where the user was still logged in
// (UserID != ""). If the user explicitly logged out, we show the login screen
// and let StartLogin handle silent re-authentication — this avoids a race
// condition where both domReady and StartLogin consume the one-time-use
// DCF refresh token simultaneously.
func (a *App) domReady(ctx context.Context) {
	if a.database == nil {
		return
	}

	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.UserID == "" {
		return // no active session — show login screen
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
		// DCF refresh tokens are one-time use — Twitch returns a new one each time.
		// Preserve the existing token if the response somehow omits it.
		if tokens.RefreshToken != "" {
			row.RefreshToken = tokens.RefreshToken
		}
		row.ExpiresAt = time.Now().Unix() + int64(tokens.ExpiresIn)
		_ = a.database.SaveAuth(*row)
	}

	// Verify the token actually has all required scopes.
	// A refresh token cannot grant new scopes — if any are missing the user
	// must re-authorize interactively.
	if val, verr := twitch.ValidateToken(row.AccessToken); verr != nil || !hasRequiredScopes(val.Scopes, twitchScopes) {
		_ = a.database.ClearAuth()
		runtime.EventsEmit(a.ctx, "auth:changed", nil)
		return
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
	// Try silent re-authentication first using any stored refresh token.
	// This avoids Device Code Flow when the user has already authorized the app.
	if a.database != nil {
		if row, err := a.database.GetAuth(); err == nil && row != nil && row.RefreshToken != "" {
			tokens, rerr := auth.RefreshAccessToken(twitchClientID, twitchClientSecret, row.RefreshToken)
			if rerr == nil {
				// Verify the refreshed token has all required scopes before accepting it.
				// Refresh tokens cannot grant new scopes added since initial authorization.
				if val, verr := twitch.ValidateToken(tokens.AccessToken); verr == nil && hasRequiredScopes(val.Scopes, twitchScopes) {
					return "", a.finishLogin(tokens)
				}
				runtime.LogWarningf(a.ctx, "Silent re-auth succeeded but token lacks required scopes — forcing re-authorization")
			} else {
				runtime.LogWarningf(a.ctx, "Silent re-auth failed (%v) — falling back to interactive flow", rerr)
			}
			_ = a.database.ClearAuth()
		}
	}

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

	// DCF refresh tokens are one-time use. If the token exchange response omits
	// the new refresh token (shouldn't happen, but be defensive), preserve the
	// existing one so the next login can still silently re-authenticate.
	refreshToken := tokens.RefreshToken
	if refreshToken == "" {
		if existing, gerr := a.database.GetAuth(); gerr == nil && existing != nil {
			refreshToken = existing.RefreshToken
		}
	}

	row := db.AuthRow{
		AccessToken:     tokens.AccessToken,
		RefreshToken:    refreshToken,
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

// Logout disconnects EventSub and clears user/session data.
// The refresh token is intentionally preserved so the next login can silently
// re-authenticate without requiring Device Code Flow again.
func (a *App) Logout() error {
	if a.eventSubCancel != nil {
		a.eventSubCancel()
		a.eventSubCancel = nil
		a.eventSubClient = nil
	}
	if err := a.database.ClearAuthKeepRefresh(); err != nil {
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
	client.OnPollBegin = func(evt twitch.PollEvent) {
		runtime.EventsEmit(a.ctx, "poll:begin", evt)
	}
	client.OnPollEnd = func(evt twitch.PollEvent) {
		runtime.EventsEmit(a.ctx, "poll:end", evt)
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

// ─── Polls ───────────────────────────────────────────────────────────────────

// PollChoiceDTO mirrors twitch.PollChoice for the frontend.
type PollChoiceDTO struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Votes              int    `json:"votes"`
	ChannelPointsVotes int    `json:"channelPointsVotes"`
}

// PollDTO mirrors twitch.Poll for the frontend.
type PollDTO struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Choices   []PollChoiceDTO `json:"choices"`
	Status    string          `json:"status"`
	Duration  int             `json:"duration"`
	StartedAt string          `json:"startedAt"`
	EndedAt   string          `json:"endedAt"`
}

func pollToDTO(p twitch.Poll) PollDTO {
	choices := make([]PollChoiceDTO, len(p.Choices))
	for i, c := range p.Choices {
		choices[i] = PollChoiceDTO{
			ID:                 c.ID,
			Title:              c.Title,
			Votes:              c.Votes,
			ChannelPointsVotes: c.ChannelPointsVotes,
		}
	}
	return PollDTO{
		ID:        p.ID,
		Title:     p.Title,
		Choices:   choices,
		Status:    p.Status,
		Duration:  p.Duration,
		StartedAt: p.StartedAt,
		EndedAt:   p.EndedAt,
	}
}

// CreatePoll creates a new channel poll. duration is in seconds (15–1800).
func (a *App) CreatePoll(title string, choices []string, duration int) (*PollDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	poll, err := twitch.CreatePoll(twitchClientID, row.AccessToken, row.UserID, title, choices, duration)
	if err != nil {
		return nil, err
	}
	dto := pollToDTO(*poll)
	return &dto, nil
}

// EndPoll ends the active poll. Pass showResults=true to display results in chat.
func (a *App) EndPoll(pollID string, showResults bool) (*PollDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	status := "TERMINATED"
	if !showResults {
		status = "ARCHIVED"
	}
	poll, err := twitch.EndPoll(twitchClientID, row.AccessToken, row.UserID, pollID, status)
	if err != nil {
		return nil, err
	}
	dto := pollToDTO(*poll)
	return &dto, nil
}

// GetPolls returns recent polls for the authenticated broadcaster.
func (a *App) GetPolls() ([]PollDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	polls, err := twitch.GetPolls(twitchClientID, row.AccessToken, row.UserID)
	if err != nil {
		return nil, err
	}
	dtos := make([]PollDTO, len(polls))
	for i, p := range polls {
		dtos[i] = pollToDTO(p)
	}
	return dtos, nil
}

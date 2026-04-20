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

	"ai-ssistme/internal/ai"
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
	twitchScopes      = "user:read:chat channel:manage:polls channel:manage:raids user:read:follows channel:manage:broadcast channel:manage:redemptions channel:manage:predictions moderator:manage:announcements moderator:manage:shoutouts channel:read:goals channel:read:hype_train clips:edit"
)

// App is the main application struct bound to the Wails frontend.
type App struct {
	ctx      context.Context
	database *db.DB

	eventSubClient *twitch.EventSubClient
	eventSubCancel context.CancelFunc

	aiProcessor *ai.Processor
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
	client.OnPollProgress = func(evt twitch.PollEvent) {
		runtime.EventsEmit(a.ctx, "poll:progress", evt)
	}
	client.OnPollEnd = func(evt twitch.PollEvent) {
		runtime.EventsEmit(a.ctx, "poll:end", evt)
		// Auto-archive the completed poll in the local DB.
		if a.database != nil {
			choices := make([]db.ArchivedPollChoice, len(evt.Choices))
			for i, c := range evt.Choices {
				choices[i] = db.ArchivedPollChoice{ID: c.ID, Title: c.Title, Votes: c.Votes}
			}
			_ = a.database.SavePollArchive(db.ArchivedPoll{
				PollID:    evt.ID,
				Title:     evt.Title,
				Status:    evt.Status,
				Choices:   choices,
				StartedAt: evt.StartedAt,
				EndedAt:   evt.EndedAt,
				CreatedAt: time.Now().Unix(),
			})
		}
	}
	client.OnPredictionBegin = func(evt twitch.PredictionEvent) {
		runtime.EventsEmit(a.ctx, "prediction:begin", evt)
	}
	client.OnPredictionProgress = func(evt twitch.PredictionEvent) {
		runtime.EventsEmit(a.ctx, "prediction:progress", evt)
	}
	client.OnPredictionLock = func(evt twitch.PredictionEvent) {
		runtime.EventsEmit(a.ctx, "prediction:lock", evt)
	}
	client.OnPredictionEnd = func(evt twitch.PredictionEvent) {
		runtime.EventsEmit(a.ctx, "prediction:end", evt)
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
	OpenAIAPIKey string  `json:"openAIApiKey"`
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
		OpenAIAPIKey: getString("openai_api_key"),
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
		"openai_api_key":          s.OpenAIAPIKey,
	}

	// Read old key before saving so we can detect a change.
	oldKey, _ := a.database.GetSetting("openai_api_key")

	for k, v := range saves {
		if err := a.database.SaveSetting(k, v); err != nil {
			return err
		}
	}

	// If the API key changed, discard the cached processor so it re-initialises
	// with the new key on the next voice command.
	if oldKey != s.OpenAIAPIKey {
		a.aiProcessor = nil
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

// ─── Poll Archive & Templates ─────────────────────────────────────────────────

// ArchivedPollDTO is a past poll sent to the frontend.
type ArchivedPollDTO struct {
	ID        int64           `json:"id"`
	PollID    string          `json:"pollId"`
	Title     string          `json:"title"`
	Status    string          `json:"status"`
	Duration  int             `json:"duration"`
	Choices   []PollChoiceDTO `json:"choices"`
	StartedAt string          `json:"startedAt"`
	EndedAt   string          `json:"endedAt"`
	CreatedAt int64           `json:"createdAt"`
}

// PollTemplateDTO is a reusable poll template sent to the frontend.
type PollTemplateDTO struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Title     string   `json:"title"`
	Choices   []string `json:"choices"`
	Duration  int      `json:"duration"`
	CreatedAt int64    `json:"createdAt"`
}

// GetPollArchive returns all locally archived polls, newest first.
func (a *App) GetPollArchive() ([]ArchivedPollDTO, error) {
	rows, err := a.database.GetPollArchive()
	if err != nil {
		return nil, err
	}
	out := make([]ArchivedPollDTO, len(rows))
	for i, r := range rows {
		choices := make([]PollChoiceDTO, len(r.Choices))
		for j, c := range r.Choices {
			choices[j] = PollChoiceDTO{ID: c.ID, Title: c.Title, Votes: c.Votes}
		}
		out[i] = ArchivedPollDTO{
			ID:        r.ID,
			PollID:    r.PollID,
			Title:     r.Title,
			Status:    r.Status,
			Duration:  r.Duration,
			Choices:   choices,
			StartedAt: r.StartedAt,
			EndedAt:   r.EndedAt,
			CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

// GetPollTemplates returns all saved poll templates.
func (a *App) GetPollTemplates() ([]PollTemplateDTO, error) {
	rows, err := a.database.GetPollTemplates()
	if err != nil {
		return nil, err
	}
	out := make([]PollTemplateDTO, len(rows))
	for i, t := range rows {
		out[i] = PollTemplateDTO{
			ID:        t.ID,
			Name:      t.Name,
			Title:     t.Title,
			Choices:   t.Choices,
			Duration:  t.Duration,
			CreatedAt: t.CreatedAt,
		}
	}
	return out, nil
}

// SavePollTemplate saves a new reusable poll template.
func (a *App) SavePollTemplate(name string, title string, choices []string, duration int) error {
	if name == "" {
		return fmt.Errorf("template name is required")
	}
	if title == "" {
		return fmt.Errorf("poll question is required")
	}
	if len(choices) < 2 {
		return fmt.Errorf("at least 2 choices are required")
	}
	return a.database.SavePollTemplate(db.PollTemplate{
		Name:      name,
		Title:     title,
		Choices:   choices,
		Duration:  duration,
		CreatedAt: time.Now().Unix(),
	})
}

// DeletePollTemplate deletes a saved poll template by ID.
func (a *App) DeletePollTemplate(id int64) error {
	return a.database.DeletePollTemplate(id)
}

// ─── Raids ────────────────────────────────────────────────────────────────────

// RaidTargetDTO is a live channel that can be raided.
type RaidTargetDTO struct {
	ID           string   `json:"id"`
	Login        string   `json:"login"`
	DisplayName  string   `json:"displayName"`
	GameName     string   `json:"gameName"`
	Title        string   `json:"title"`
	ViewerCount  int      `json:"viewerCount"`
	StartedAt    string   `json:"startedAt"`
	ThumbnailURL string   `json:"thumbnailURL"`
	AvatarURL    string   `json:"avatarURL"`
	Tags         []string `json:"tags"`
}

func streamToRaidTarget(s twitch.LiveStream) RaidTargetDTO {
	thumb := strings.NewReplacer("{width}", "320", "{height}", "180").Replace(s.ThumbnailURL)
	tags := s.Tags
	if tags == nil {
		tags = []string{}
	}
	return RaidTargetDTO{
		ID:           s.UserID,
		Login:        s.UserLogin,
		DisplayName:  s.UserName,
		GameName:     s.GameName,
		Title:        s.Title,
		ViewerCount:  s.ViewerCount,
		StartedAt:    s.StartedAt,
		ThumbnailURL: thumb,
		Tags:         tags,
	}
}

// GetFollowedLiveChannels returns live streams from channels the authenticated user follows.
func (a *App) GetFollowedLiveChannels() ([]RaidTargetDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	streams, err := twitch.GetFollowedStreams(twitchClientID, row.AccessToken, row.UserID, 100)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(streams))
	for _, s := range streams {
		if s.UserID != row.UserID {
			ids = append(ids, s.UserID)
		}
	}
	avatars, _ := twitch.GetUserProfileImages(twitchClientID, row.AccessToken, ids)
	out := make([]RaidTargetDTO, 0, len(ids))
	for _, s := range streams {
		if s.UserID == row.UserID {
			continue
		}
		dto := streamToRaidTarget(s)
		dto.AvatarURL = avatars[s.UserID]
		out = append(out, dto)
	}
	return out, nil
}

// GetSameCategoryChannels returns live channels streaming in the same game/category
// as the authenticated broadcaster. Results are ordered by viewer count (Twitch default).
func (a *App) GetSameCategoryChannels() ([]RaidTargetDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	info, err := twitch.GetChannelInfo(twitchClientID, row.AccessToken, row.UserID)
	if err != nil {
		return nil, err
	}
	if info.GameID == "" {
		return nil, fmt.Errorf("your channel has no category set")
	}
	streams, err := twitch.GetStreamsByCategory(twitchClientID, row.AccessToken, info.GameID, 50)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(streams))
	for _, s := range streams {
		if s.UserID != row.UserID {
			ids = append(ids, s.UserID)
		}
	}
	avatars, _ := twitch.GetUserProfileImages(twitchClientID, row.AccessToken, ids)
	out := make([]RaidTargetDTO, 0, len(ids))
	for _, s := range streams {
		if s.UserID == row.UserID {
			continue
		}
		dto := streamToRaidTarget(s)
		dto.AvatarURL = avatars[s.UserID]
		out = append(out, dto)
	}
	return out, nil
}

// SearchRaidTargets searches for live channels matching a query string.
func (a *App) SearchRaidTargets(query string) ([]RaidTargetDTO, error) {
	if query == "" {
		return []RaidTargetDTO{}, nil
	}
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	streams, err := twitch.SearchLiveChannels(twitchClientID, row.AccessToken, query, 20)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(streams))
	for _, s := range streams {
		if s.UserID != row.UserID {
			ids = append(ids, s.UserID)
		}
	}
	avatars, _ := twitch.GetUserProfileImages(twitchClientID, row.AccessToken, ids)
	out := make([]RaidTargetDTO, 0, len(ids))
	for _, s := range streams {
		if s.UserID == row.UserID {
			continue
		}
		dto := streamToRaidTarget(s)
		dto.AvatarURL = avatars[s.UserID]
		out = append(out, dto)
	}
	return out, nil
}

// StartRaid initiates a raid to the specified broadcaster ID.
func (a *App) StartRaid(toBroadcasterID string) error {
	if toBroadcasterID == "" {
		return fmt.Errorf("target broadcaster ID is required")
	}
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return fmt.Errorf("not authenticated")
	}
	if toBroadcasterID == row.UserID {
		return fmt.Errorf("cannot raid your own channel")
	}
	return twitch.StartRaid(twitchClientID, row.AccessToken, row.UserID, toBroadcasterID)
}

// CancelRaid cancels the broadcaster's pending raid.
func (a *App) CancelRaid() error {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return fmt.Errorf("not authenticated")
	}
	return twitch.CancelRaid(twitchClientID, row.AccessToken, row.UserID)
}

// ChannelInfoDTO is the frontend-facing channel info structure.
type ChannelInfoDTO struct {
	Title    string   `json:"title"`
	GameID   string   `json:"gameID"`
	GameName string   `json:"gameName"`
	Language string   `json:"language"`
	Tags     []string `json:"tags"`
}

// CategoryDTO is a game/category search result.
type CategoryDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BoxArtURL string `json:"boxArtURL"`
}

// GetMyChannelInfo fetches the authenticated broadcaster's current channel information.
func (a *App) GetMyChannelInfo() (*ChannelInfoDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	info, err := twitch.GetChannelInfo(twitchClientID, row.AccessToken, row.UserID)
	if err != nil {
		return nil, err
	}
	tags := info.Tags
	if tags == nil {
		tags = []string{}
	}
	return &ChannelInfoDTO{
		Title:    info.Title,
		GameID:   info.GameID,
		GameName: info.GameName,
		Language: info.BroadcasterLanguage,
		Tags:     tags,
	}, nil
}

// UpdateChannelInfo updates the broadcaster's channel title, game, language, and tags.
func (a *App) UpdateChannelInfo(title, gameID, language string, tags []string) error {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return fmt.Errorf("not authenticated")
	}
	return twitch.ModifyChannel(twitchClientID, row.AccessToken, row.UserID, title, gameID, language, tags)
}

// SearchCategories searches for Twitch categories/games matching the query.
func (a *App) SearchCategories(query string) ([]CategoryDTO, error) {
	if query == "" {
		return []CategoryDTO{}, nil
	}
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	results, err := twitch.SearchCategories(twitchClientID, row.AccessToken, query, 10)
	if err != nil {
		return nil, err
	}
	out := make([]CategoryDTO, 0, len(results))
	for _, r := range results {
		out = append(out, CategoryDTO{
			ID:        r.ID,
			Name:      r.Name,
			BoxArtURL: r.BoxArtURL,
		})
	}
	return out, nil
}

// ─── Custom Rewards ───────────────────────────────────────────────────────────

// CustomRewardDTO is the frontend-facing custom reward structure.
type CustomRewardDTO struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Prompt          string `json:"prompt"`
	Cost            int    `json:"cost"`
	BackgroundColor string `json:"backgroundColor"`
	IsEnabled       bool   `json:"isEnabled"`
	IsPaused        bool   `json:"isPaused"`
	IsInStock       bool   `json:"isInStock"`

	IsUserInputRequired               bool `json:"isUserInputRequired"`
	ShouldRedemptionsSkipRequestQueue bool `json:"shouldRedemptionsSkipRequestQueue"`

	MaxPerStreamEnabled bool `json:"maxPerStreamEnabled"`
	MaxPerStream        int  `json:"maxPerStream"`
	MaxPerUserEnabled   bool `json:"maxPerUserEnabled"`
	MaxPerUser          int  `json:"maxPerUser"`
	CooldownEnabled     bool `json:"cooldownEnabled"`
	CooldownSeconds     int  `json:"cooldownSeconds"`
}

// RedemptionDTO is a viewer's redemption of a custom reward.
type RedemptionDTO struct {
	ID          string `json:"id"`
	UserID      string `json:"userId"`
	UserLogin   string `json:"userLogin"`
	UserName    string `json:"userName"`
	UserInput   string `json:"userInput"`
	Status      string `json:"status"`
	RedeemedAt  string `json:"redeemedAt"`
	RewardID    string `json:"rewardId"`
	RewardTitle string `json:"rewardTitle"`
	RewardCost  int    `json:"rewardCost"`
}

// CreateRewardInput is the data sent from the frontend to create a reward.
type CreateRewardInput struct {
	Title                             string `json:"title"`
	Cost                              int    `json:"cost"`
	Prompt                            string `json:"prompt"`
	IsEnabled                         bool   `json:"isEnabled"`
	BackgroundColor                   string `json:"backgroundColor"`
	IsUserInputRequired               bool   `json:"isUserInputRequired"`
	ShouldRedemptionsSkipRequestQueue bool   `json:"shouldRedemptionsSkipRequestQueue"`
	MaxPerStreamEnabled               bool   `json:"maxPerStreamEnabled"`
	MaxPerStream                      int    `json:"maxPerStream"`
	MaxPerUserEnabled                 bool   `json:"maxPerUserEnabled"`
	MaxPerUser                        int    `json:"maxPerUser"`
	CooldownEnabled                   bool   `json:"cooldownEnabled"`
	CooldownSeconds                   int    `json:"cooldownSeconds"`
}

func rewardToDTO(r twitch.CustomReward) CustomRewardDTO {
	return CustomRewardDTO{
		ID:                                r.ID,
		Title:                             r.Title,
		Prompt:                            r.Prompt,
		Cost:                              r.Cost,
		BackgroundColor:                   r.BackgroundColor,
		IsEnabled:                         r.IsEnabled,
		IsPaused:                          r.IsPaused,
		IsInStock:                         r.IsInStock,
		IsUserInputRequired:               r.IsUserInputRequired,
		ShouldRedemptionsSkipRequestQueue: r.ShouldRedemptionsSkipRequestQueue,
		MaxPerStreamEnabled:               r.MaxPerStreamSetting.IsEnabled,
		MaxPerStream:                      r.MaxPerStreamSetting.MaxPerStream,
		MaxPerUserEnabled:                 r.MaxPerUserPerStreamSetting.IsEnabled,
		MaxPerUser:                        r.MaxPerUserPerStreamSetting.MaxPerUserPerStream,
		CooldownEnabled:                   r.GlobalCooldownSetting.IsEnabled,
		CooldownSeconds:                   r.GlobalCooldownSetting.GlobalCooldownSeconds,
	}
}

// GetCustomRewards returns all manageable custom rewards for the authenticated broadcaster.
func (a *App) GetCustomRewards() ([]CustomRewardDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	rewards, err := twitch.GetCustomRewards(twitchClientID, row.AccessToken, row.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]CustomRewardDTO, len(rewards))
	for i, r := range rewards {
		out[i] = rewardToDTO(r)
	}
	return out, nil
}

// CreateCustomReward creates a new channel point custom reward.
func (a *App) CreateCustomReward(input CreateRewardInput) (*CustomRewardDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	reward, err := twitch.CreateCustomReward(twitchClientID, row.AccessToken, row.UserID, twitch.CreateCustomRewardInput{
		Title:                             input.Title,
		Cost:                              input.Cost,
		Prompt:                            input.Prompt,
		IsEnabled:                         input.IsEnabled,
		BackgroundColor:                   input.BackgroundColor,
		IsUserInputRequired:               input.IsUserInputRequired,
		ShouldRedemptionsSkipRequestQueue: input.ShouldRedemptionsSkipRequestQueue,
		IsMaxPerStreamEnabled:             input.MaxPerStreamEnabled,
		MaxPerStream:                      input.MaxPerStream,
		IsMaxPerUserPerStreamEnabled:      input.MaxPerUserEnabled,
		MaxPerUserPerStream:               input.MaxPerUser,
		IsGlobalCooldownEnabled:           input.CooldownEnabled,
		GlobalCooldownSeconds:             input.CooldownSeconds,
	})
	if err != nil {
		return nil, err
	}
	dto := rewardToDTO(*reward)
	return &dto, nil
}

// UpdateCustomReward updates an existing custom reward by ID.
func (a *App) UpdateCustomReward(rewardID string, input CreateRewardInput) (*CustomRewardDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	ptrBool := func(b bool) *bool { return &b }
	ptrInt := func(i int) *int { return &i }
	ptrStr := func(s string) *string { return &s }
	reward, err := twitch.UpdateCustomReward(twitchClientID, row.AccessToken, row.UserID, rewardID, twitch.UpdateCustomRewardInput{
		Title:                             ptrStr(input.Title),
		Cost:                              ptrInt(input.Cost),
		Prompt:                            ptrStr(input.Prompt),
		IsEnabled:                         ptrBool(input.IsEnabled),
		BackgroundColor:                   ptrStr(input.BackgroundColor),
		IsUserInputRequired:               ptrBool(input.IsUserInputRequired),
		ShouldRedemptionsSkipRequestQueue: ptrBool(input.ShouldRedemptionsSkipRequestQueue),
		IsMaxPerStreamEnabled:             ptrBool(input.MaxPerStreamEnabled),
		MaxPerStream:                      ptrInt(input.MaxPerStream),
		IsMaxPerUserPerStreamEnabled:      ptrBool(input.MaxPerUserEnabled),
		MaxPerUserPerStream:               ptrInt(input.MaxPerUser),
		IsGlobalCooldownEnabled:           ptrBool(input.CooldownEnabled),
		GlobalCooldownSeconds:             ptrInt(input.CooldownSeconds),
	})
	if err != nil {
		return nil, err
	}
	dto := rewardToDTO(*reward)
	return &dto, nil
}

// ToggleCustomRewardPaused pauses or unpauses a reward without touching other settings.
func (a *App) ToggleCustomRewardPaused(rewardID string, paused bool) error {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return fmt.Errorf("not authenticated")
	}
	_, err = twitch.UpdateCustomReward(twitchClientID, row.AccessToken, row.UserID, rewardID, twitch.UpdateCustomRewardInput{
		IsPaused: func(b bool) *bool { return &b }(paused),
	})
	return err
}

// DeleteCustomReward permanently removes a custom reward created by this app.
func (a *App) DeleteCustomReward(rewardID string) error {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return fmt.Errorf("not authenticated")
	}
	return twitch.DeleteCustomReward(twitchClientID, row.AccessToken, row.UserID, rewardID)
}

// GetPendingRedemptions returns UNFULFILLED redemptions for the given reward.
func (a *App) GetPendingRedemptions(rewardID string) ([]RedemptionDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	redemptions, err := twitch.GetRedemptions(twitchClientID, row.AccessToken, row.UserID, rewardID, "UNFULFILLED")
	if err != nil {
		return nil, err
	}
	out := make([]RedemptionDTO, len(redemptions))
	for i, r := range redemptions {
		out[i] = RedemptionDTO{
			ID:          r.ID,
			UserID:      r.UserID,
			UserLogin:   r.UserLogin,
			UserName:    r.UserName,
			UserInput:   r.UserInput,
			Status:      r.Status,
			RedeemedAt:  r.RedeemedAt,
			RewardID:    r.Reward.ID,
			RewardTitle: r.Reward.Title,
			RewardCost:  r.Reward.Cost,
		}
	}
	return out, nil
}

// FulfillRedemption marks a redemption as FULFILLED.
func (a *App) FulfillRedemption(rewardID, redemptionID string) error {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return fmt.Errorf("not authenticated")
	}
	return twitch.UpdateRedemptionStatus(twitchClientID, row.AccessToken, row.UserID, rewardID, redemptionID, "FULFILLED")
}

// CancelRedemption marks a redemption as CANCELED (refunds the viewer's points).
func (a *App) CancelRedemption(rewardID, redemptionID string) error {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return fmt.Errorf("not authenticated")
	}
	return twitch.UpdateRedemptionStatus(twitchClientID, row.AccessToken, row.UserID, rewardID, redemptionID, "CANCELED")
}

// ─── AI Voice Commands ────────────────────────────────────────────────────────

// AICommandResultDTO is the result of an AI voice command returned to the frontend.
type AICommandResultDTO struct {
	Transcript string   `json:"transcript"`
	Message    string   `json:"message"`
	Actions    []string `json:"actions"`
}

// getAIProcessor lazily constructs the AI processor, wiring Twitch action handlers.
func (a *App) getAIProcessor() (*ai.Processor, error) {
	apiKey, _ := a.database.GetSetting("openai_api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("no OpenAI API key configured — add yours in Settings → AI Voice Commands")
	}
	if a.aiProcessor != nil {
		return a.aiProcessor, nil
	}
	a.aiProcessor = ai.NewProcessor(apiKey, ai.ActionHandlers{
		CreatePoll: func(title string, choices []string, durationSecs int) error {
			_, err := a.CreatePoll(title, choices, durationSecs)
			return err
		},
		StartRaidByLogin: func(channelLogin string) error {
			row, err := a.database.GetAuth()
			if err != nil || row == nil || row.AccessToken == "" {
				return fmt.Errorf("not authenticated")
			}
			user, err := twitch.GetUserByLogin(twitchClientID, row.AccessToken, channelLogin)
			if err != nil {
				return err
			}
			return a.StartRaid(user.ID)
		},
		CancelRaid: func() error {
			return a.CancelRaid()
		},
		EndActivePoll: func() error {
			polls, err := a.GetPolls()
			if err != nil {
				return err
			}
			for _, p := range polls {
				if p.Status == "ACTIVE" {
					_, err := a.EndPoll(p.ID, true)
					return err
				}
			}
			return fmt.Errorf("no active poll to end")
		},
		UpdateStreamTitle: func(title string) error {
			return a.UpdateChannelInfo(title, "", "", nil)
		},
		UpdateStreamGame: func(gameName string) error {
			categories, err := a.SearchCategories(gameName)
			if err != nil {
				return err
			}
			if len(categories) == 0 {
				return fmt.Errorf("no category found matching '%s'", gameName)
			}
			return a.UpdateChannelInfo("", categories[0].ID, "", nil)
		},
		CreateChannelReward: func(title string, cost int, prompt string) error {
			_, err := a.CreateCustomReward(CreateRewardInput{
				Title:     title,
				Cost:      cost,
				Prompt:    prompt,
				IsEnabled: true,
			})
			return err
		},
		PauseReward: func(title string) error {
			rewards, err := a.GetCustomRewards()
			if err != nil {
				return err
			}
			titleLower := strings.ToLower(title)
			for _, r := range rewards {
				if strings.EqualFold(r.Title, title) {
					return a.ToggleCustomRewardPaused(r.ID, true)
				}
			}
			for _, r := range rewards {
				if strings.Contains(strings.ToLower(r.Title), titleLower) {
					return a.ToggleCustomRewardPaused(r.ID, true)
				}
			}
			return fmt.Errorf("no reward found matching '%s'", title)
		},
		ResumeReward: func(title string) error {
			rewards, err := a.GetCustomRewards()
			if err != nil {
				return err
			}
			titleLower := strings.ToLower(title)
			for _, r := range rewards {
				if strings.EqualFold(r.Title, title) {
					return a.ToggleCustomRewardPaused(r.ID, false)
				}
			}
			for _, r := range rewards {
				if strings.Contains(strings.ToLower(r.Title), titleLower) {
					return a.ToggleCustomRewardPaused(r.ID, false)
				}
			}
			return fmt.Errorf("no reward found matching '%s'", title)
		},
		CreateClip: func() error {
			_, err := a.CreateClip(false)
			return err
		},
	})
	return a.aiProcessor, nil
}

// ProcessVoiceCommand accepts a base64-encoded WebM/Opus audio recording from the
// frontend, transcribes it with OpenAI Whisper, and executes any Twitch management
// commands via GPT-4o function calling.
func (a *App) ProcessVoiceCommand(audioBase64 string) (*AICommandResultDTO, error) {
	if !a.IsAuthenticated() {
		return nil, fmt.Errorf("not authenticated — please log in first")
	}

	processor, err := a.getAIProcessor()
	if err != nil {
		return nil, err
	}

	audioBytes, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid audio data: %w", err)
	}
	if len(audioBytes) == 0 {
		return nil, fmt.Errorf("audio data is empty")
	}

	result, err := processor.ProcessAudio(a.ctx, audioBytes)
	if err != nil {
		return nil, err
	}

	// Notify pages that depend on channel info to re-fetch if the AI touched stream info.
	for _, action := range result.Actions {
		switch action {
		case "update_stream_title", "update_stream_game":
			runtime.EventsEmit(a.ctx, "streaminfo:changed")
		case "create_poll", "end_poll":
			runtime.EventsEmit(a.ctx, "polls:changed")
		case "create_channel_point_reward", "pause_reward", "resume_reward":
			runtime.EventsEmit(a.ctx, "rewards:changed")
		}
	}

	return &AICommandResultDTO{
		Transcript: result.Transcript,
		Message:    result.Message,
		Actions:    result.Actions,
	}, nil
}

// ─── Predictions ─────────────────────────────────────────────────────────────

// PredictionOutcomeDTO mirrors a prediction outcome for the frontend.
type PredictionOutcomeDTO struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Color         string `json:"color"`
	Users         int    `json:"users"`
	ChannelPoints int    `json:"channelPoints"`
}

// PredictionDTO mirrors twitch.Prediction for the frontend.
type PredictionDTO struct {
	ID               string                 `json:"id"`
	Title            string                 `json:"title"`
	WinningOutcomeID string                 `json:"winningOutcomeId"`
	Outcomes         []PredictionOutcomeDTO `json:"outcomes"`
	PredictionWindow int                    `json:"predictionWindow"`
	Status           string                 `json:"status"`
	CreatedAt        string                 `json:"createdAt"`
	EndedAt          string                 `json:"endedAt"`
	LockedAt         string                 `json:"lockedAt"`
}

func predictionToDTO(p twitch.Prediction) PredictionDTO {
	outcomes := make([]PredictionOutcomeDTO, len(p.Outcomes))
	for i, o := range p.Outcomes {
		outcomes[i] = PredictionOutcomeDTO{
			ID:            o.ID,
			Title:         o.Title,
			Color:         o.Color,
			Users:         o.Users,
			ChannelPoints: o.ChannelPoints,
		}
	}
	return PredictionDTO{
		ID:               p.ID,
		Title:            p.Title,
		WinningOutcomeID: p.WinningOutcomeID,
		Outcomes:         outcomes,
		PredictionWindow: p.PredictionWindow,
		Status:           p.Status,
		CreatedAt:        p.CreatedAt,
		EndedAt:          p.EndedAt,
		LockedAt:         p.LockedAt,
	}
}

// GetPredictions returns recent predictions for the authenticated broadcaster.
func (a *App) GetPredictions() ([]PredictionDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	predictions, err := twitch.GetPredictions(twitchClientID, row.AccessToken, row.UserID)
	if err != nil {
		return nil, err
	}
	dtos := make([]PredictionDTO, len(predictions))
	for i, p := range predictions {
		dtos[i] = predictionToDTO(p)
	}
	return dtos, nil
}

// CreatePrediction creates a new channel prediction.
// outcomes must have 2–10 items; window is in seconds (30–1800).
func (a *App) CreatePrediction(title string, outcomes []string, window int) (*PredictionDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	p, err := twitch.CreatePrediction(twitchClientID, row.AccessToken, row.UserID, title, outcomes, window)
	if err != nil {
		return nil, err
	}
	dto := predictionToDTO(*p)
	return &dto, nil
}

// EndPrediction resolves, cancels, or locks a prediction.
// Pass status "RESOLVED" with a winningOutcomeID, "CANCELED", or "LOCKED".
func (a *App) EndPrediction(predictionID, status, winningOutcomeID string) (*PredictionDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	p, err := twitch.EndPrediction(twitchClientID, row.AccessToken, row.UserID, predictionID, status, winningOutcomeID)
	if err != nil {
		return nil, err
	}
	dto := predictionToDTO(*p)
	return &dto, nil
}

// ─── Tools (Announcements, Shoutouts, Stream Markers) ────────────────────────

// SendAnnouncement sends a highlighted announcement in the broadcaster's chat.
// color must be one of: blue, green, orange, purple, primary (empty defaults to primary).
func (a *App) SendAnnouncement(message, color string) error {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return fmt.Errorf("not authenticated")
	}
	return twitch.SendAnnouncement(twitchClientID, row.AccessToken, row.UserID, row.UserID, message, color)
}

// SendShoutout sends a shoutout to a channel identified by login name.
func (a *App) SendShoutout(targetLogin string) error {
	if targetLogin == "" {
		return fmt.Errorf("target channel login is required")
	}
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return fmt.Errorf("not authenticated")
	}
	target, err := twitch.GetUserByLogin(twitchClientID, row.AccessToken, targetLogin)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	return twitch.SendShoutout(twitchClientID, row.AccessToken, row.UserID, target.ID, row.UserID)
}

// CreateStreamMarker creates a marker at the current position in the live VOD.
// description is optional (≤140 chars). Requires the stream to be live with VOD enabled.
func (a *App) CreateStreamMarker(description string) error {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return fmt.Errorf("not authenticated")
	}
	return twitch.CreateStreamMarker(twitchClientID, row.AccessToken, row.UserID, description)
}

// ─── Dashboard Stats ──────────────────────────────────────────────────────────

// CreatorGoalDTO is the frontend-facing creator goal structure.
type CreatorGoalDTO struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Description   string `json:"description"`
	CurrentAmount int    `json:"currentAmount"`
	TargetAmount  int    `json:"targetAmount"`
	CreatedAt     string `json:"createdAt"`
}

// GetCreatorGoals returns active creator goals for the authenticated broadcaster.
func (a *App) GetCreatorGoals() ([]CreatorGoalDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	goals, err := twitch.GetCreatorGoals(twitchClientID, row.AccessToken, row.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]CreatorGoalDTO, len(goals))
	for i, g := range goals {
		out[i] = CreatorGoalDTO{
			ID:            g.ID,
			Type:          g.Type,
			Description:   g.Description,
			CurrentAmount: g.CurrentAmount,
			TargetAmount:  g.TargetAmount,
			CreatedAt:     g.CreatedAt,
		}
	}
	return out, nil
}

// HypeTrainEventDTO is the frontend-facing hype train event structure.
type HypeTrainEventDTO struct {
	Level     int    `json:"level"`
	Total     int    `json:"total"`
	Goal      int    `json:"goal"`
	StartedAt string `json:"startedAt"`
	ExpiresAt string `json:"expiresAt"`
}

// GetHypeTrainEvents returns the most recent hype train event for the broadcaster.
// Returns an empty slice if no hype train is active or has recently occurred.
func (a *App) GetHypeTrainEvents() ([]HypeTrainEventDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	events, err := twitch.GetHypeTrainEvents(twitchClientID, row.AccessToken, row.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]HypeTrainEventDTO, len(events))
	for i, e := range events {
		out[i] = HypeTrainEventDTO{
			Level:     e.EventData.Level,
			Total:     e.EventData.Total,
			Goal:      e.EventData.Goal,
			StartedAt: e.EventData.StartedAt,
			ExpiresAt: e.EventData.ExpiresAt,
		}
	}
	return out, nil
}

// ── Clips ────────────────────────────────────────────────────────────────────

// ClipCreatedDTO is returned after a clip is created.
type ClipCreatedDTO struct {
	ID      string `json:"id"`
	EditURL string `json:"editUrl"`
}

// ClipDTO represents a Twitch clip for display.
type ClipDTO struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"`
	EditURL      string  `json:"editUrl"`
	Title        string  `json:"title"`
	CreatorName  string  `json:"creatorName"`
	ThumbnailURL string  `json:"thumbnailUrl"`
	ViewCount    int     `json:"viewCount"`
	Duration     float64 `json:"duration"`
	CreatedAt    string  `json:"createdAt"`
	IsFeatured   bool    `json:"isFeatured"`
}

// CreateClip creates a clip from the broadcaster's live stream.
// hasDelay adds a brief buffer to account for stream delay.
func (a *App) CreateClip(hasDelay bool) (*ClipCreatedDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	created, err := twitch.CreateClip(twitchClientID, row.AccessToken, row.UserID, hasDelay)
	if err != nil {
		return nil, err
	}
	return &ClipCreatedDTO{ID: created.ID, EditURL: created.EditURL}, nil
}

// GetClips returns the broadcaster's most recent clips (up to first, max 100).
func (a *App) GetClips(first int) ([]ClipDTO, error) {
	row, err := a.database.GetAuth()
	if err != nil || row == nil || row.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	clips, err := twitch.GetClips(twitchClientID, row.AccessToken, row.UserID, first)
	if err != nil {
		return nil, err
	}
	out := make([]ClipDTO, len(clips))
	for i, c := range clips {
		out[i] = ClipDTO{
			ID:           c.ID,
			URL:          c.URL,
			EditURL:      c.EditURL,
			Title:        c.Title,
			CreatorName:  c.CreatorName,
			ThumbnailURL: c.ThumbnailURL,
			ViewCount:    c.ViewCount,
			Duration:     c.Duration,
			CreatedAt:    c.CreatedAt,
			IsFeatured:   c.IsFeatured,
		}
	}
	return out, nil
}

// OpenURL opens the given URL in the user's default browser.
func (a *App) OpenURL(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
}

// SuggestClipTitle uses AI to suggest a clip title based on the current stream context.
// streamTitle and gameName should be the current stream title and game/category.
func (a *App) SuggestClipTitle(streamTitle, gameName string) (string, error) {
	processor, err := a.getAIProcessor()
	if err != nil {
		return "", err
	}
	return processor.SuggestClipTitle(a.ctx, streamTitle, gameName)
}

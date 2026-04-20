package twitch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const helixBase = "https://api.twitch.tv/helix"

// UserInfo holds basic Twitch user data.
type UserInfo struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	ProfileImageURL string `json:"profile_image_url"`
	OfflineImageURL string `json:"offline_image_url"`
}

// GetCurrentUser fetches the authenticated user's profile from Helix.
func GetCurrentUser(clientID, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequest(http.MethodGet, helixBase+"/users", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get users: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get users status %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Data []UserInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get users decode: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch: no user data returned")
	}
	return &payload.Data[0], nil
}

// GetUserByLogin fetches a Twitch user's profile by their login name.
func GetUserByLogin(clientID, accessToken, login string) (*UserInfo, error) {
	req, err := http.NewRequest(http.MethodGet, helixBase+"/users?login="+login, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get user by login: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get user by login status %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Data []UserInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get user by login decode: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch: user not found: %s", login)
	}
	return &payload.Data[0], nil
}

// TokenValidation holds the result of a Twitch token introspection.
type TokenValidation struct {
	ClientID  string   `json:"client_id"`
	Login     string   `json:"login"`
	UserID    string   `json:"user_id"`
	Scopes    []string `json:"scopes"`
	ExpiresIn int      `json:"expires_in"`
}

// ValidateToken calls the Twitch token introspection endpoint and returns
// the scopes the token was issued with. Returns an error if the token is invalid.
func ValidateToken(accessToken string) (*TokenValidation, error) {
	req, err := http.NewRequest(http.MethodGet, "https://id.twitch.tv/oauth2/validate", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "OAuth "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: validate token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: validate token status %d: %s", resp.StatusCode, body)
	}
	var v TokenValidation
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, fmt.Errorf("twitch: validate token decode: %w", err)
	}
	return &v, nil
}

// CreateChatMessageSubscription creates a channel.chat.message EventSub subscription
// via the WebSocket transport using the provided session ID.
func CreateChatMessageSubscription(clientID, accessToken, sessionID, broadcasterID, userID string) error {
	payload := map[string]interface{}{
		"type":    "channel.chat.message",
		"version": "1",
		"condition": map[string]string{
			"broadcaster_user_id": broadcasterID,
			"user_id":             userID,
		},
		"transport": map[string]string{
			"method":     "websocket",
			"session_id": sessionID,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, helixBase+"/eventsub/subscriptions", strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twitch: create subscription: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// 202 Accepted is the success code for EventSub subscriptions.
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("twitch: create subscription status %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ─── Poll Types ───────────────────────────────────────────────────────────────

// PollChoice represents a single choice in a poll.
type PollChoice struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Votes              int    `json:"votes"`
	ChannelPointsVotes int    `json:"channel_points_votes"`
	BitsVotes          int    `json:"bits_votes"`
}

// Poll represents a Twitch channel poll.
type Poll struct {
	ID                         string       `json:"id"`
	BroadcasterID              string       `json:"broadcaster_id"`
	BroadcasterName            string       `json:"broadcaster_name"`
	Title                      string       `json:"title"`
	Choices                    []PollChoice `json:"choices"`
	ChannelPointsVotingEnabled bool         `json:"channel_points_voting_enabled"`
	ChannelPointsPerVote       int          `json:"channel_points_per_vote"`
	Status                     string       `json:"status"`
	Duration                   int          `json:"duration"`
	StartedAt                  string       `json:"started_at"`
	EndedAt                    string       `json:"ended_at"`
}

// CreatePoll creates a new channel poll and returns the created poll.
// duration is in seconds (15–1800).
func CreatePoll(clientID, accessToken, broadcasterID, title string, choices []string, duration int) (*Poll, error) {
	choiceList := make([]map[string]string, len(choices))
	for i, c := range choices {
		choiceList[i] = map[string]string{"title": c}
	}
	body, _ := json.Marshal(map[string]interface{}{
		"broadcaster_id": broadcasterID,
		"title":          title,
		"choices":        choiceList,
		"duration":       duration,
	})

	req, err := http.NewRequest(http.MethodPost, helixBase+"/polls", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: create poll: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: create poll status %d: %s", resp.StatusCode, respBody)
	}

	var payload struct {
		Data []Poll `json:"data"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("twitch: create poll decode: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch: create poll: no data returned")
	}
	return &payload.Data[0], nil
}

// EndPoll terminates or archives a poll.
// status must be "TERMINATED" (end and show results) or "ARCHIVED" (end and hide).
func EndPoll(clientID, accessToken, broadcasterID, pollID, status string) (*Poll, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"broadcaster_id": broadcasterID,
		"id":             pollID,
		"status":         status,
	})

	req, err := http.NewRequest(http.MethodPatch, helixBase+"/polls", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: end poll: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: end poll status %d: %s", resp.StatusCode, respBody)
	}

	var payload struct {
		Data []Poll `json:"data"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("twitch: end poll decode: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch: end poll: no data returned")
	}
	return &payload.Data[0], nil
}

// GetPolls returns recent polls for the broadcaster (last 90 days, newest first).
func GetPolls(clientID, accessToken, broadcasterID string) ([]Poll, error) {
	req, err := http.NewRequest(http.MethodGet, helixBase+"/polls?broadcaster_id="+broadcasterID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get polls: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get polls status %d: %s", resp.StatusCode, respBody)
	}

	var payload struct {
		Data []Poll `json:"data"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get polls decode: %w", err)
	}
	return payload.Data, nil
}

// ─── Raid Types ──────────────────────────────────────────────────────────────

// LiveStream represents a stream returned from the Helix streams API.
type LiveStream struct {
	UserID       string   `json:"user_id"`
	UserLogin    string   `json:"user_login"`
	UserName     string   `json:"user_name"`
	GameID       string   `json:"game_id"`
	GameName     string   `json:"game_name"`
	Title        string   `json:"title"`
	ViewerCount  int      `json:"viewer_count"`
	ThumbnailURL string   `json:"thumbnail_url"`
	StartedAt    string   `json:"started_at"`
	Tags         []string `json:"tags"`
}

// ChannelInfo holds the result from GET /helix/channels.
type ChannelInfo struct {
	BroadcasterID       string   `json:"broadcaster_id"`
	BroadcasterLogin    string   `json:"broadcaster_login"`
	GameID              string   `json:"game_id"`
	GameName            string   `json:"game_name"`
	Title               string   `json:"title"`
	BroadcasterLanguage string   `json:"broadcaster_language"`
	Tags                []string `json:"tags"`
}

// CategoryResult is a game/category returned from GET /helix/search/categories.
type CategoryResult struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BoxArtURL string `json:"box_art_url"`
}

// ModifyChannel updates the broadcaster's channel information.
// Requires channel:manage:broadcast scope.
func ModifyChannel(clientID, accessToken, broadcasterID, title, gameID, language string, tags []string) error {
	body := map[string]interface{}{}
	if title != "" {
		body["title"] = title
	}
	if gameID != "" {
		body["game_id"] = gameID
	}
	if language != "" {
		body["broadcaster_language"] = language
	}
	if tags != nil {
		body["tags"] = tags
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/channels?broadcaster_id=%s", helixBase, broadcasterID)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twitch: modify channel: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("twitch: modify channel status %d: %s", resp.StatusCode, respBody)
}

// SearchCategories searches for games/categories matching a query string.
func SearchCategories(clientID, accessToken, query string, first int) ([]CategoryResult, error) {
	url := fmt.Sprintf("%s/search/categories?query=%s&first=%d",
		helixBase, query, first)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: search categories: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: search categories status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []CategoryResult `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: search categories decode: %w", err)
	}
	return payload.Data, nil
}

// GetFollowedStreams returns live streams from channels the user follows.
// Requires user:read:follows scope.
func GetFollowedStreams(clientID, accessToken, userID string, first int) ([]LiveStream, error) {
	url := fmt.Sprintf("%s/streams/followed?user_id=%s&first=%d", helixBase, userID, first)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get followed streams: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get followed streams status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []LiveStream `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get followed streams decode: %w", err)
	}
	return payload.Data, nil
}

// GetStreamsByCategory returns live streams in a given game/category.
func GetStreamsByCategory(clientID, accessToken, gameID string, first int) ([]LiveStream, error) {
	url := fmt.Sprintf("%s/streams?game_id=%s&first=%d", helixBase, gameID, first)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get streams by category: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get streams by category status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []LiveStream `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get streams by category decode: %w", err)
	}
	return payload.Data, nil
}

// GetChannelInfo returns channel metadata (title, current game) for a broadcaster.
func GetChannelInfo(clientID, accessToken, broadcasterID string) (*ChannelInfo, error) {
	url := fmt.Sprintf("%s/channels?broadcaster_id=%s", helixBase, broadcasterID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get channel info: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get channel info status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []ChannelInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get channel info decode: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch: no channel info returned")
	}
	return &payload.Data[0], nil
}

// SearchLiveChannels searches for live channels matching a query string.
func SearchLiveChannels(clientID, accessToken, query string, first int) ([]LiveStream, error) {
	url := fmt.Sprintf("%s/search/channels?query=%s&live_only=true&first=%d",
		helixBase, query, first)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: search live channels: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: search live channels status %d: %s", resp.StatusCode, body)
	}
	// Search Channels returns a different shape — adapt it to LiveStream.
	var payload struct {
		Data []struct {
			ID          string   `json:"id"`
			DisplayName string   `json:"display_name"`
			Login       string   `json:"broadcaster_login"`
			GameName    string   `json:"game_name"`
			GameID      string   `json:"game_id"`
			Title       string   `json:"title"`
			IsLive      bool     `json:"is_live"`
			StartedAt   string   `json:"started_at"`
			Thumbnail   string   `json:"thumbnail_url"`
			Tags        []string `json:"tags"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: search live channels decode: %w", err)
	}
	streams := make([]LiveStream, 0, len(payload.Data))
	for _, ch := range payload.Data {
		if !ch.IsLive {
			continue
		}
		streams = append(streams, LiveStream{
			UserID:       ch.ID,
			UserLogin:    ch.Login,
			UserName:     ch.DisplayName,
			GameID:       ch.GameID,
			GameName:     ch.GameName,
			Title:        ch.Title,
			ThumbnailURL: ch.Thumbnail,
			StartedAt:    ch.StartedAt,
			Tags:         ch.Tags,
		})
	}
	return streams, nil
}

// GetUserProfileImages fetches profile image URLs for up to 100 user IDs.
// Returns a map of userID → profileImageURL. Best-effort: errors are non-fatal.
func GetUserProfileImages(clientID, accessToken string, userIDs []string) (map[string]string, error) {
	if len(userIDs) == 0 {
		return map[string]string{}, nil
	}
	// Twitch user IDs are numeric so no URL encoding needed.
	reqURL := helixBase + "/users?id=" + strings.Join(userIDs, "&id=")
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get user profiles: %w", err)
	}
	defer resp.Body.Close()
	body2, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get user profiles status %d: %s", resp.StatusCode, body2)
	}
	var payload2 struct {
		Data []struct {
			ID              string `json:"id"`
			ProfileImageURL string `json:"profile_image_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body2, &payload2); err != nil {
		return nil, fmt.Errorf("twitch: get user profiles decode: %w", err)
	}
	result := make(map[string]string, len(payload2.Data))
	for _, u := range payload2.Data {
		result[u.ID] = u.ProfileImageURL
	}
	return result, nil
}

// StartRaid initiates a raid from the broadcaster to a target channel.
// Requires channel:manage:raids scope.
func StartRaid(clientID, accessToken, fromBroadcasterID, toBroadcasterID string) error {
	url := fmt.Sprintf("%s/raids?from_broadcaster_id=%s&to_broadcaster_id=%s",
		helixBase, fromBroadcasterID, toBroadcasterID)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twitch: start raid: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("twitch: start raid status %d: %s", resp.StatusCode, body)
	}
	return nil
}

// CancelRaid cancels a pending raid initiated by the broadcaster.
// Requires channel:manage:raids scope.
func CancelRaid(clientID, accessToken, broadcasterID string) error {
	url := fmt.Sprintf("%s/raids?broadcaster_id=%s", helixBase, broadcasterID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twitch: cancel raid: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("twitch: cancel raid status %d: %s", resp.StatusCode, body)
}

// ─── Channel Points Custom Rewards ────────────────────────────────────────────

// CustomRewardSetting is the nested max/cooldown setting on a custom reward.
type CustomRewardSetting struct {
	IsEnabled bool `json:"is_enabled"`
	Value     int  `json:"max_per_stream,omitempty"`
}

// CustomReward represents a broadcaster's channel point custom reward.
type CustomReward struct {
	ID              string `json:"id"`
	BroadcasterID   string `json:"broadcaster_id"`
	BroadcasterName string `json:"broadcaster_name"`
	Title           string `json:"title"`
	Prompt          string `json:"prompt"`
	Cost            int    `json:"cost"`
	BackgroundColor string `json:"background_color"`
	IsEnabled       bool   `json:"is_enabled"`
	IsPaused        bool   `json:"is_paused"`
	IsInStock       bool   `json:"is_in_stock"`

	IsUserInputRequired               bool `json:"is_user_input_required"`
	ShouldRedemptionsSkipRequestQueue bool `json:"should_redemptions_skip_request_queue"`

	MaxPerStreamSetting struct {
		IsEnabled    bool `json:"is_enabled"`
		MaxPerStream int  `json:"max_per_stream"`
	} `json:"max_per_stream_setting"`

	MaxPerUserPerStreamSetting struct {
		IsEnabled           bool `json:"is_enabled"`
		MaxPerUserPerStream int  `json:"max_per_user_per_stream"`
	} `json:"max_per_user_per_stream_setting"`

	GlobalCooldownSetting struct {
		IsEnabled             bool `json:"is_enabled"`
		GlobalCooldownSeconds int  `json:"global_cooldown_seconds"`
	} `json:"global_cooldown_setting"`

	CooldownExpiresAt string `json:"cooldown_expires_at"`
}

// GetCustomRewards returns the broadcaster's custom rewards.
// Requires channel:manage:redemptions scope.
func GetCustomRewards(clientID, accessToken, broadcasterID string) ([]CustomReward, error) {
	url := fmt.Sprintf("%s/channel_points/custom_rewards?broadcaster_id=%s&only_manageable_rewards=true",
		helixBase, broadcasterID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get custom rewards: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get custom rewards status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []CustomReward `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get custom rewards decode: %w", err)
	}
	return payload.Data, nil
}

// CreateCustomRewardInput holds parameters for creating a custom reward.
type CreateCustomRewardInput struct {
	Title                             string `json:"title"`
	Cost                              int    `json:"cost"`
	Prompt                            string `json:"prompt,omitempty"`
	IsEnabled                         bool   `json:"is_enabled"`
	BackgroundColor                   string `json:"background_color,omitempty"`
	IsUserInputRequired               bool   `json:"is_user_input_required"`
	ShouldRedemptionsSkipRequestQueue bool   `json:"should_redemptions_skip_request_queue"`
	IsMaxPerStreamEnabled             bool   `json:"is_max_per_stream_enabled,omitempty"`
	MaxPerStream                      int    `json:"max_per_stream,omitempty"`
	IsMaxPerUserPerStreamEnabled      bool   `json:"is_max_per_user_per_stream_enabled,omitempty"`
	MaxPerUserPerStream               int    `json:"max_per_user_per_stream,omitempty"`
	IsGlobalCooldownEnabled           bool   `json:"is_global_cooldown_enabled,omitempty"`
	GlobalCooldownSeconds             int    `json:"global_cooldown_seconds,omitempty"`
}

// CreateCustomReward creates a new custom channel point reward.
// Requires channel:manage:redemptions scope.
func CreateCustomReward(clientID, accessToken, broadcasterID string, input CreateCustomRewardInput) (*CustomReward, error) {
	b, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/channel_points/custom_rewards?broadcaster_id=%s", helixBase, broadcasterID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: create custom reward: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: create custom reward status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []CustomReward `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: create custom reward decode: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch: create custom reward: no data returned")
	}
	return &payload.Data[0], nil
}

// UpdateCustomRewardInput holds the optional fields for updating a custom reward.
// Fields that are not set are omitted from the request body.
type UpdateCustomRewardInput struct {
	Title                             *string `json:"title,omitempty"`
	Cost                              *int    `json:"cost,omitempty"`
	Prompt                            *string `json:"prompt,omitempty"`
	IsEnabled                         *bool   `json:"is_enabled,omitempty"`
	IsPaused                          *bool   `json:"is_paused,omitempty"`
	BackgroundColor                   *string `json:"background_color,omitempty"`
	IsUserInputRequired               *bool   `json:"is_user_input_required,omitempty"`
	ShouldRedemptionsSkipRequestQueue *bool   `json:"should_redemptions_skip_request_queue,omitempty"`
	IsMaxPerStreamEnabled             *bool   `json:"is_max_per_stream_enabled,omitempty"`
	MaxPerStream                      *int    `json:"max_per_stream,omitempty"`
	IsMaxPerUserPerStreamEnabled      *bool   `json:"is_max_per_user_per_stream_enabled,omitempty"`
	MaxPerUserPerStream               *int    `json:"max_per_user_per_stream,omitempty"`
	IsGlobalCooldownEnabled           *bool   `json:"is_global_cooldown_enabled,omitempty"`
	GlobalCooldownSeconds             *int    `json:"global_cooldown_seconds,omitempty"`
}

// UpdateCustomReward patches an existing custom reward.
// Requires channel:manage:redemptions scope.
func UpdateCustomReward(clientID, accessToken, broadcasterID, rewardID string, input UpdateCustomRewardInput) (*CustomReward, error) {
	b, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/channel_points/custom_rewards?broadcaster_id=%s&id=%s",
		helixBase, broadcasterID, rewardID)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: update custom reward: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: update custom reward status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []CustomReward `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: update custom reward decode: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch: update custom reward: no data returned")
	}
	return &payload.Data[0], nil
}

// DeleteCustomReward removes a custom reward created by this app.
// Requires channel:manage:redemptions scope.
func DeleteCustomReward(clientID, accessToken, broadcasterID, rewardID string) error {
	url := fmt.Sprintf("%s/channel_points/custom_rewards?broadcaster_id=%s&id=%s",
		helixBase, broadcasterID, rewardID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twitch: delete custom reward: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("twitch: delete custom reward status %d: %s", resp.StatusCode, body)
}

// Redemption represents a viewer's redemption of a custom reward.
type Redemption struct {
	ID            string `json:"id"`
	BroadcasterID string `json:"broadcaster_id"`
	UserID        string `json:"user_id"`
	UserLogin     string `json:"user_login"`
	UserName      string `json:"user_name"`
	UserInput     string `json:"user_input"`
	Status        string `json:"status"`
	RedeemedAt    string `json:"redeemed_at"`
	Reward        struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Cost  int    `json:"cost"`
	} `json:"reward"`
}

// GetRedemptions returns redemptions for the given reward filtered by status.
// status must be one of: UNFULFILLED, FULFILLED, CANCELED.
// Requires channel:manage:redemptions scope.
func GetRedemptions(clientID, accessToken, broadcasterID, rewardID, status string) ([]Redemption, error) {
	url := fmt.Sprintf("%s/channel_points/custom_rewards/redemptions?broadcaster_id=%s&reward_id=%s&status=%s&first=50",
		helixBase, broadcasterID, rewardID, status)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get redemptions: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get redemptions status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []Redemption `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get redemptions decode: %w", err)
	}
	return payload.Data, nil
}

// UpdateRedemptionStatus marks a redemption as FULFILLED or CANCELED.
// Only UNFULFILLED redemptions can be updated.
// Requires channel:manage:redemptions scope.
func UpdateRedemptionStatus(clientID, accessToken, broadcasterID, rewardID, redemptionID, status string) error {
	b, _ := json.Marshal(map[string]string{"status": status})
	url := fmt.Sprintf("%s/channel_points/custom_rewards/redemptions?broadcaster_id=%s&reward_id=%s&id=%s",
		helixBase, broadcasterID, rewardID, redemptionID)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twitch: update redemption status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("twitch: update redemption status %d: %s", resp.StatusCode, body)
}

// CreatePollEventSubscription subscribes to channel.poll.begin and channel.poll.end
// events for the broadcaster using a WebSocket transport.
func CreatePollEventSubscription(clientID, accessToken, sessionID, broadcasterID string) error {
	for _, subType := range []string{"channel.poll.begin", "channel.poll.progress", "channel.poll.end"} {
		payload := map[string]interface{}{
			"type":    subType,
			"version": "1",
			"condition": map[string]string{
				"broadcaster_user_id": broadcasterID,
			},
			"transport": map[string]string{
				"method":     "websocket",
				"session_id": sessionID,
			},
		}
		data, _ := json.Marshal(payload)
		req, err := http.NewRequest(http.MethodPost, helixBase+"/eventsub/subscriptions", strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		req.Header.Set("Client-Id", clientID)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("twitch: create %s subscription: %w", subType, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("twitch: create %s subscription status %d: %s", subType, resp.StatusCode, body)
		}
	}
	return nil
}

// ─── Predictions ─────────────────────────────────────────────────────────────

// PredictionOutcome is a single outcome in a channel prediction.
type PredictionOutcome struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Color         string `json:"color"`
	Users         int    `json:"users"`
	ChannelPoints int    `json:"channel_points"`
	TopPredictors []struct {
		UserID            string `json:"user_id"`
		UserName          string `json:"user_name"`
		UserLogin         string `json:"user_login"`
		ChannelPointsUsed int    `json:"channel_points_used"`
		ChannelPointsWon  int    `json:"channel_points_won"`
	} `json:"top_predictors"`
}

// Prediction represents a Twitch channel prediction.
type Prediction struct {
	ID               string              `json:"id"`
	BroadcasterID    string              `json:"broadcaster_id"`
	BroadcasterName  string              `json:"broadcaster_name"`
	Title            string              `json:"title"`
	WinningOutcomeID string              `json:"winning_outcome_id"`
	Outcomes         []PredictionOutcome `json:"outcomes"`
	PredictionWindow int                 `json:"prediction_window"`
	Status           string              `json:"status"`
	CreatedAt        string              `json:"created_at"`
	EndedAt          string              `json:"ended_at"`
	LockedAt         string              `json:"locked_at"`
}

// GetPredictions returns recent predictions for the broadcaster (newest first).
// Requires channel:manage:predictions scope.
func GetPredictions(clientID, accessToken, broadcasterID string) ([]Prediction, error) {
	req, err := http.NewRequest(http.MethodGet, helixBase+"/predictions?broadcaster_id="+broadcasterID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get predictions: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get predictions status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []Prediction `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get predictions decode: %w", err)
	}
	return payload.Data, nil
}

// CreatePrediction creates a new channel prediction.
// outcomes must have 2–10 items; window is in seconds (30–1800).
// Requires channel:manage:predictions scope.
func CreatePrediction(clientID, accessToken, broadcasterID, title string, outcomes []string, window int) (*Prediction, error) {
	outcomeList := make([]map[string]string, len(outcomes))
	for i, o := range outcomes {
		outcomeList[i] = map[string]string{"title": o}
	}
	b, _ := json.Marshal(map[string]interface{}{
		"broadcaster_id":    broadcasterID,
		"title":             title,
		"outcomes":          outcomeList,
		"prediction_window": window,
	})
	req, err := http.NewRequest(http.MethodPost, helixBase+"/predictions", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: create prediction: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: create prediction status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []Prediction `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: create prediction decode: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch: create prediction: no data returned")
	}
	return &payload.Data[0], nil
}

// EndPrediction resolves, cancels, or locks a prediction.
// status must be "RESOLVED", "CANCELED", or "LOCKED".
// winningOutcomeID is required when status is "RESOLVED".
// Requires channel:manage:predictions scope.
func EndPrediction(clientID, accessToken, broadcasterID, predictionID, status, winningOutcomeID string) (*Prediction, error) {
	body := map[string]interface{}{
		"broadcaster_id": broadcasterID,
		"id":             predictionID,
		"status":         status,
	}
	if winningOutcomeID != "" {
		body["winning_outcome_id"] = winningOutcomeID
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPatch, helixBase+"/predictions", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: end prediction: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: end prediction status %d: %s", resp.StatusCode, respBody)
	}
	var payload struct {
		Data []Prediction `json:"data"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("twitch: end prediction decode: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch: end prediction: no data returned")
	}
	return &payload.Data[0], nil
}

// CreatePredictionEventSubscription subscribes to channel.prediction.* EventSub events.
// Requires channel:manage:predictions scope.
func CreatePredictionEventSubscription(clientID, accessToken, sessionID, broadcasterID string) error {
	for _, subType := range []string{
		"channel.prediction.begin",
		"channel.prediction.progress",
		"channel.prediction.lock",
		"channel.prediction.end",
	} {
		payload := map[string]interface{}{
			"type":    subType,
			"version": "1",
			"condition": map[string]string{
				"broadcaster_user_id": broadcasterID,
			},
			"transport": map[string]string{
				"method":     "websocket",
				"session_id": sessionID,
			},
		}
		data, _ := json.Marshal(payload)
		req, err := http.NewRequest(http.MethodPost, helixBase+"/eventsub/subscriptions", strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		req.Header.Set("Client-Id", clientID)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("twitch: create %s subscription: %w", subType, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("twitch: create %s subscription status %d: %s", subType, resp.StatusCode, body)
		}
	}
	return nil
}

// ─── Chat Tools ───────────────────────────────────────────────────────────────

// SendAnnouncement sends a chat announcement in the broadcaster's channel.
// color must be one of: blue, green, orange, purple, primary (empty string defaults to primary).
// Requires moderator:manage:announcements scope.
func SendAnnouncement(clientID, accessToken, broadcasterID, moderatorID, message, color string) error {
	if color == "" {
		color = "primary"
	}
	b, _ := json.Marshal(map[string]string{
		"message": message,
		"color":   color,
	})
	url := fmt.Sprintf("%s/chat/announcements?broadcaster_id=%s&moderator_id=%s",
		helixBase, broadcasterID, moderatorID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twitch: send announcement: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("twitch: send announcement status %d: %s", resp.StatusCode, body)
}

// SendShoutout sends a shoutout from one broadcaster to another.
// Requires moderator:manage:shoutouts scope.
func SendShoutout(clientID, accessToken, fromBroadcasterID, toBroadcasterID, moderatorID string) error {
	url := fmt.Sprintf("%s/chat/shoutouts?from_broadcaster_id=%s&to_broadcaster_id=%s&moderator_id=%s",
		helixBase, fromBroadcasterID, toBroadcasterID, moderatorID)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twitch: send shoutout: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("twitch: send shoutout status %d: %s", resp.StatusCode, body)
}

// ─── Stream Markers ───────────────────────────────────────────────────────────

// CreateStreamMarker creates a marker in a live stream VOD at the current timestamp.
// description is optional (≤140 chars). Only works when the broadcaster is live
// and has VOD recording enabled.
// Requires channel:manage:broadcast scope.
func CreateStreamMarker(clientID, accessToken, userID, description string) error {
	body := map[string]string{"user_id": userID}
	if description != "" {
		body["description"] = description
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, helixBase+"/streams/markers", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twitch: create stream marker: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("twitch: create stream marker status %d: %s", resp.StatusCode, respBody)
}

// ─── Creator Goals ────────────────────────────────────────────────────────────

// CreatorGoal represents a Twitch creator goal.
type CreatorGoal struct {
	ID              string `json:"id"`
	BroadcasterID   string `json:"broadcaster_id"`
	BroadcasterName string `json:"broadcaster_name"`
	Type            string `json:"type"`
	Description     string `json:"description"`
	CurrentAmount   int    `json:"current_amount"`
	TargetAmount    int    `json:"target_amount"`
	CreatedAt       string `json:"created_at"`
}

// GetCreatorGoals returns active creator goals for the broadcaster.
// Requires channel:read:goals scope.
func GetCreatorGoals(clientID, accessToken, broadcasterID string) ([]CreatorGoal, error) {
	req, err := http.NewRequest(http.MethodGet, helixBase+"/goals?broadcaster_id="+broadcasterID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get creator goals: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get creator goals status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []CreatorGoal `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get creator goals decode: %w", err)
	}
	return payload.Data, nil
}

// ─── Hype Train ───────────────────────────────────────────────────────────────

// HypeTrainContribution is a top contribution in a hype train event.
type HypeTrainContribution struct {
	Total  int    `json:"total"`
	Type   string `json:"type"`
	UserID string `json:"user"`
}

// HypeTrainEvent represents a hype train event from the Helix API.
type HypeTrainEvent struct {
	ID            string `json:"id"`
	BroadcasterID string `json:"broadcaster_id"`
	EventData     struct {
		BroadcasterID    string                  `json:"broadcaster_id"`
		CooldownEndTime  string                  `json:"cooldown_end_time"`
		ExpiresAt        string                  `json:"expires_at"`
		Goal             int                     `json:"goal"`
		ID               string                  `json:"id"`
		LastContribution HypeTrainContribution   `json:"last_contribution"`
		Level            int                     `json:"level"`
		StartedAt        string                  `json:"started_at"`
		TopContributions []HypeTrainContribution `json:"top_contributions"`
		Total            int                     `json:"total"`
	} `json:"event_data"`
	EventType      string `json:"event_type"`
	EventTimestamp string `json:"event_timestamp"`
	Version        string `json:"version"`
}

// GetHypeTrainEvents returns the most recent hype train events for the broadcaster.
// Requires channel:read:hype_train scope.
func GetHypeTrainEvents(clientID, accessToken, broadcasterID string) ([]HypeTrainEvent, error) {
	url := fmt.Sprintf("%s/hypetrain/events?broadcaster_id=%s&first=1", helixBase, broadcasterID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get hype train events: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get hype train events status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []HypeTrainEvent `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get hype train events decode: %w", err)
	}
	return payload.Data, nil
}

// ── Clips ────────────────────────────────────────────────────────────────────

// ClipCreated is returned by POST /helix/clips.
type ClipCreated struct {
	ID      string `json:"id"`
	EditURL string `json:"edit_url"`
}

// Clip holds the full metadata for a Twitch clip.
type Clip struct {
	ID            string  `json:"id"`
	URL           string  `json:"url"`
	EditURL       string  `json:"edit_url"`
	BroadcasterID string  `json:"broadcaster_id"`
	CreatorName   string  `json:"creator_name"`
	VideoID       string  `json:"video_id"`
	GameID        string  `json:"game_id"`
	Title         string  `json:"title"`
	ViewCount     int     `json:"view_count"`
	CreatedAt     string  `json:"created_at"`
	ThumbnailURL  string  `json:"thumbnail_url"`
	Duration      float64 `json:"duration"`
	IsFeatured    bool    `json:"is_featured"`
}

// CreateClip creates a clip from the broadcaster's live stream.
// Requires the clips:edit scope.
// The returned EditURL can be opened in a browser to set the title/duration.
// Clip creation is asynchronous — the clip may not be immediately available via GetClips.
func CreateClip(clientID, accessToken, broadcasterID string, hasDelay bool) (*ClipCreated, error) {
	delay := "false"
	if hasDelay {
		delay = "true"
	}
	url := fmt.Sprintf("%s/clips?broadcaster_id=%s&has_delay=%s", helixBase, broadcasterID, delay)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: create clip: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("twitch: create clip status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []ClipCreated `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: create clip decode: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch: create clip returned no data")
	}
	return &payload.Data[0], nil
}

// GetClips returns the broadcaster's most recent clips (up to first, max 100).
func GetClips(clientID, accessToken, broadcasterID string, first int) ([]Clip, error) {
	if first <= 0 {
		first = 20
	}
	url := fmt.Sprintf("%s/clips?broadcaster_id=%s&first=%d", helixBase, broadcasterID, first)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch: get clips: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch: get clips status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []Clip `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("twitch: get clips decode: %w", err)
	}
	return payload.Data, nil
}

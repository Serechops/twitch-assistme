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

// CreatePollEventSubscription subscribes to channel.poll.begin and channel.poll.end
// events for the broadcaster using a WebSocket transport.
func CreatePollEventSubscription(clientID, accessToken, sessionID, broadcasterID string) error {
	for _, subType := range []string{"channel.poll.begin", "channel.poll.end"} {
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

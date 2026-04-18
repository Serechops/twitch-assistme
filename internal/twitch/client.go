package twitch

import (
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

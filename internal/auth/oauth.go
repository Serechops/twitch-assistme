package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	twitchDeviceURL   = "https://id.twitch.tv/oauth2/device"
	twitchTokenURL    = "https://id.twitch.tv/oauth2/token"
	twitchValidateURL = "https://id.twitch.tv/oauth2/validate"
)

// TokenResponse holds the Twitch OAuth token response fields we care about.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// DeviceFlowResponse is the response from the device authorization endpoint.
type DeviceFlowResponse struct {
	DeviceCode      string `json:"device_code"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
}

// StartDeviceFlow initiates the Twitch Device Code Grant Flow for a public client.
// The caller should open VerificationURI in the user's browser, then call PollForToken.
func StartDeviceFlow(clientID, scopes string) (*DeviceFlowResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scopes", scopes)

	resp, err := http.PostForm(twitchDeviceURL, form)
	if err != nil {
		return nil, fmt.Errorf("auth: start device flow: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: start device flow status %d: %s", resp.StatusCode, body)
	}

	var dfr DeviceFlowResponse
	if err := json.Unmarshal(body, &dfr); err != nil {
		return nil, fmt.Errorf("auth: start device flow decode: %w", err)
	}
	return &dfr, nil
}

// PollForToken polls the Twitch token endpoint until the user authorizes the device,
// the context is cancelled, or the device code expires.
func PollForToken(ctx context.Context, clientID, deviceCode string, intervalSecs int) (*TokenResponse, error) {
	interval := time.Duration(intervalSecs) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("auth: login cancelled")
		case <-time.After(interval):
		}

		resp, err := http.PostForm(twitchTokenURL, form)
		if err != nil {
			return nil, fmt.Errorf("auth: poll token: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var tr TokenResponse
			if err := json.Unmarshal(body, &tr); err != nil {
				return nil, fmt.Errorf("auth: poll token decode: %w", err)
			}
			return &tr, nil
		}

		var errResp struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			switch errResp.Message {
			case "authorization_pending":
				// Normal — user hasn't clicked Authorize yet. Keep polling.
				continue
			case "slow_down":
				interval += 5 * time.Second
				continue
			default:
				return nil, fmt.Errorf("auth: %s", errResp.Message)
			}
		}

		return nil, fmt.Errorf("auth: poll token status %d: %s", resp.StatusCode, body)
	}
}

// RefreshAccessToken exchanges a refresh token for a new access token.
// No client_secret required for public clients.
func RefreshAccessToken(clientID, refreshToken string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")

	resp, err := http.PostForm(twitchTokenURL, form)
	if err != nil {
		return nil, fmt.Errorf("auth: refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: refresh token status %d: %s", resp.StatusCode, body)
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("auth: refresh token decode: %w", err)
	}
	return &tr, nil
}

// ValidateToken calls the Twitch validate endpoint to check token validity.
// Returns (userID, userLogin, nil) on success.
func ValidateToken(accessToken string) (userID, userLogin string, err error) {
	req, _ := http.NewRequest(http.MethodGet, twitchValidateURL, nil)
	req.Header.Set("Authorization", "OAuth "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("auth: validate token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("auth: validate token status %d: %s", resp.StatusCode, body)
	}

	var v struct {
		UserID string `json:"user_id"`
		Login  string `json:"login"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", "", fmt.Errorf("auth: validate token decode: %w", err)
	}
	return v.UserID, v.Login, nil
}


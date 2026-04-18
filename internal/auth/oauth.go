package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	twitchAuthorizeURL = "https://id.twitch.tv/oauth2/authorize"
	twitchTokenURL     = "https://id.twitch.tv/oauth2/token"
	twitchValidateURL  = "https://id.twitch.tv/oauth2/validate"
	redirectPort       = "3000"
	redirectURI        = "http://localhost:3000"
)

// TokenResponse holds the Twitch OAuth token response fields we care about.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// BuildAuthURL constructs the Twitch authorization URL with PKCE params.
func BuildAuthURL(clientID, state, codeChallenge string) string {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "user:read:chat")
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	return twitchAuthorizeURL + "?" + params.Encode()
}

// GenerateState generates a random opaque state string for CSRF protection.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ListenForCallback starts a one-shot HTTP server on localhost:3000 that
// captures the OAuth callback code. It returns the authorization code or an error.
func ListenForCallback(ctx context.Context, expectedState string) (string, error) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		if q.Get("state") != expectedState {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("auth: state mismatch")
			return
		}
		if errMsg := q.Get("error"); errMsg != "" {
			http.Error(w, errMsg, http.StatusBadRequest)
			errCh <- fmt.Errorf("auth: twitch error: %s — %s", errMsg, q.Get("error_description"))
			return
		}

		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			errCh <- fmt.Errorf("auth: missing code in callback")
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:3rem">
<h2>✅ Authenticated!</h2><p>You can close this tab and return to the app.</p></body></html>`)
		codeCh <- code
	})

	listener, err := net.Listen("tcp", ":"+redirectPort)
	if err != nil {
		return "", fmt.Errorf("auth: cannot listen on port %s: %w", redirectPort, err)
	}

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)

	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()

	select {
	case code := <-codeCh:
		return code, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("auth: callback wait cancelled")
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("auth: timed out waiting for Twitch callback")
	}
}

// ExchangeCode exchanges an authorization code for tokens using PKCE.
// No client_secret is required — this app uses the public client PKCE flow.
func ExchangeCode(clientID, code, codeVerifier string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("code", code)
	form.Set("code_verifier", codeVerifier)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", redirectURI)

	resp, err := http.PostForm(twitchTokenURL, form)
	if err != nil {
		return nil, fmt.Errorf("auth: exchange code: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: exchange code status %d: %s", resp.StatusCode, body)
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("auth: exchange code decode: %w", err)
	}
	return &tr, nil
}

// RefreshAccessToken exchanges a refresh token for a new access token.
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
		return "", "", fmt.Errorf("auth: validate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", "", fmt.Errorf("auth: token is invalid or expired")
	}

	body, _ := io.ReadAll(resp.Body)
	var v struct {
		UserID    string `json:"user_id"`
		Login     string `json:"login"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", "", fmt.Errorf("auth: validate decode: %w", err)
	}

	_ = strings.TrimSpace // imported for clarity only
	return v.UserID, v.Login, nil
}

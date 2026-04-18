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
	"time"
)

const twitchAuthorizeURL = "https://id.twitch.tv/oauth2/authorize"

// GenerateState returns a cryptographically random CSRF state string.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// BuildAuthURL constructs the Twitch Authorization Code flow URL.
func BuildAuthURL(clientID, redirectURI, state, scopes string) string {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", scopes)
	params.Set("state", state)
	return twitchAuthorizeURL + "?" + params.Encode()
}

// ListenForCallback starts a one-shot HTTP server on the given port that captures
// the OAuth authorization code from the Twitch redirect. Returns the code or an error.
func ListenForCallback(ctx context.Context, expectedState, port string) (string, error) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		if q.Get("state") != expectedState {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("auth: state mismatch — possible CSRF attack")
			return
		}
		if errMsg := q.Get("error"); errMsg != "" {
			http.Error(w, errMsg, http.StatusBadRequest)
			errCh <- fmt.Errorf("auth: twitch: %s — %s", errMsg, q.Get("error_description"))
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			errCh <- fmt.Errorf("auth: missing code in callback")
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:3rem;background:#0e0e12;color:#efeff1">
<h2 style="color:#9147ff">Authorized!</h2>
<p>You can close this tab and return to Twitch AssistMe.</p>
</body></html>`)
		codeCh <- code
	})

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return "", fmt.Errorf("auth: listen on port %s: %w", port, err)
	}

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener) //nolint:errcheck

	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx) //nolint:errcheck
	}()

	select {
	case code := <-codeCh:
		return code, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("auth: login cancelled")
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("auth: timed out waiting for browser authorization")
	}
}

// ExchangeCode exchanges an authorization code for tokens.
// clientSecret must be provided for Twitch confidential clients.
func ExchangeCode(clientID, clientSecret, redirectURI, code string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
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

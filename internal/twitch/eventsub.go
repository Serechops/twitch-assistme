package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const eventSubWSURL = "wss://eventsub.wss.twitch.tv/ws"

// StatusConnected and friends are the possible EventSub connection statuses.
const (
	StatusDisconnected = "disconnected"
	StatusConnecting   = "connecting"
	StatusConnected    = "connected"
)

// PollEvent is the parsed event payload from channel.poll.begin and channel.poll.end notifications.
type PollEvent struct {
	ID                  string       `json:"id"`
	BroadcasterUserID   string       `json:"broadcaster_user_id"`
	BroadcasterUserName string       `json:"broadcaster_user_name"`
	Title               string       `json:"title"`
	Choices             []PollChoice `json:"choices"`
	Status              string       `json:"status"`
	StartedAt           string       `json:"started_at"`
	EndedAt             string       `json:"ended_at"`
	EndsAt              string       `json:"ends_at"`
}

// ChatMessageEvent is the parsed event payload from channel.chat.message notifications.
type ChatMessageEvent struct {
	BroadcasterUserID    string `json:"broadcaster_user_id"`
	BroadcasterUserLogin string `json:"broadcaster_user_login"`
	BroadcasterUserName  string `json:"broadcaster_user_name"`
	ChatterUserID        string `json:"chatter_user_id"`
	ChatterUserLogin     string `json:"chatter_user_login"`
	ChatterUserName      string `json:"chatter_user_name"`
	MessageID            string `json:"message_id"`
	Message              struct {
		Text string `json:"text"`
	} `json:"message"`
	Color       string `json:"color"`
	MessageType string `json:"message_type"`
}

// EventSubClient manages the Twitch EventSub WebSocket connection.
type EventSubClient struct {
	clientID      string
	accessToken   string
	broadcasterID string
	userID        string

	// Filter config (set before Connect; safe to read during operation).
	IgnoreOwnMessages bool
	CooldownMs        int64

	// Callbacks (set before Connect).
	OnChatMessage func(evt ChatMessageEvent)
	OnPollBegin   func(evt PollEvent)
	OnPollEnd     func(evt PollEvent)
	OnStatus      func(status string)

	mu           sync.Mutex
	conn         *websocket.Conn
	status       string
	lastEmitUnix int64 // unix ms of last emitted chat event

	cancel context.CancelFunc
}

// NewEventSubClient creates a new client ready to connect.
func NewEventSubClient(clientID, accessToken, broadcasterID, userID string) *EventSubClient {
	return &EventSubClient{
		clientID:      clientID,
		accessToken:   accessToken,
		broadcasterID: broadcasterID,
		userID:        userID,
		status:        StatusDisconnected,
	}
}

func (c *EventSubClient) setStatus(s string) {
	c.mu.Lock()
	c.status = s
	c.mu.Unlock()
	if c.OnStatus != nil {
		c.OnStatus(s)
	}
}

// GetStatus returns the current connection status string.
func (c *EventSubClient) GetStatus() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Connect dials the EventSub WebSocket and begins processing messages.
// It blocks until the context is cancelled or a fatal error occurs.
func (c *EventSubClient) Connect(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()
	defer cancel()

	c.connectLoop(ctx, eventSubWSURL)
}

// Disconnect stops the client.
func (c *EventSubClient) Disconnect() {
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// connectLoop manages reconnection logic.
func (c *EventSubClient) connectLoop(ctx context.Context, wsURL string) {
	for {
		select {
		case <-ctx.Done():
			c.setStatus(StatusDisconnected)
			return
		default:
		}

		c.setStatus(StatusConnecting)
		reconnectURL, err := c.runSession(ctx, wsURL)
		if err != nil {
			select {
			case <-ctx.Done():
				c.setStatus(StatusDisconnected)
				return
			default:
				// Transient error — wait briefly then reconnect.
				time.Sleep(5 * time.Second)
			}
			continue
		}

		// session_reconnect received: immediately dial the new URL.
		if reconnectURL != "" {
			wsURL = reconnectURL
		}
	}
}

// runSession handles one WebSocket session. Returns (reconnectURL, err).
// If reconnectURL is non-empty, the caller should reconnect to that URL.
func (c *EventSubClient) runSession(ctx context.Context, wsURL string) (string, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return "", fmt.Errorf("eventsub: dial: %w", err)
	}
	defer conn.Close()

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// keepalive_timeout_seconds from the welcome message.
	var keepaliveTimeout int64 = 10
	// last message time for watchdog.
	lastMsg := atomic.Int64{}
	lastMsg.Store(time.Now().Unix())

	// Watchdog goroutine.
	watchdogDone := make(chan struct{})
	go func() {
		defer close(watchdogDone)
		for {
			time.Sleep(time.Second)
			select {
			case <-ctx.Done():
				return
			default:
			}
			elapsed := time.Now().Unix() - lastMsg.Load()
			if elapsed > keepaliveTimeout+5 {
				conn.Close()
				return
			}
		}
	}()
	defer func() { <-watchdogDone }()

	reconnectURL := ""

	for {
		select {
		case <-ctx.Done():
			return "", nil
		default:
		}

		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return reconnectURL, fmt.Errorf("eventsub: read: %w", err)
		}

		lastMsg.Store(time.Now().Unix())

		var frame struct {
			Metadata struct {
				MessageType      string `json:"message_type"`
				SubscriptionType string `json:"subscription_type"`
			} `json:"metadata"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(msgBytes, &frame); err != nil {
			continue
		}

		switch frame.Metadata.MessageType {
		case "session_welcome":
			var p struct {
				Session struct {
					ID                      string `json:"id"`
					KeepaliveTimeoutSeconds int    `json:"keepalive_timeout_seconds"`
				} `json:"session"`
			}
			if err := json.Unmarshal(frame.Payload, &p); err != nil {
				continue
			}
			atomic.StoreInt64(&keepaliveTimeout, int64(p.Session.KeepaliveTimeoutSeconds))

			if err := CreateChatMessageSubscription(
				c.clientID, c.accessToken,
				p.Session.ID, c.broadcasterID, c.userID,
			); err != nil {
				return "", fmt.Errorf("eventsub: subscribe chat: %w", err)
			}
			if err := CreatePollEventSubscription(
				c.clientID, c.accessToken,
				p.Session.ID, c.broadcasterID,
			); err != nil {
				// Poll subscription requires channel:manage:polls scope.
				// Log but don't fail the entire connection.
				fmt.Printf("eventsub: poll subscribe warning: %v\n", err)
			}
			c.setStatus(StatusConnected)

		case "session_keepalive":
			// Already updated lastMsg above.

		case "session_reconnect":
			var p struct {
				Session struct {
					ReconnectURL string `json:"reconnect_url"`
				} `json:"session"`
			}
			if err := json.Unmarshal(frame.Payload, &p); err != nil {
				continue
			}
			reconnectURL = p.Session.ReconnectURL
			// Keep reading until the connection closes naturally.

		case "notification":
			switch frame.Metadata.SubscriptionType {
			case "channel.chat.message":
				c.handleChatMessage(frame.Payload)
			case "channel.poll.begin":
				c.handlePollEvent(frame.Payload, true)
			case "channel.poll.end":
				c.handlePollEvent(frame.Payload, false)
			}

		case "revocation":
			c.setStatus(StatusDisconnected)
			return "", fmt.Errorf("eventsub: subscription revoked")
		}
	}
}

func (c *EventSubClient) handleChatMessage(raw json.RawMessage) {
	var p struct {
		Event ChatMessageEvent `json:"event"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	evt := p.Event

	// Filter: ignore the broadcaster's own messages if configured.
	if c.IgnoreOwnMessages && evt.ChatterUserID == c.userID {
		return
	}

	// Filter: cooldown between notifications.
	if c.CooldownMs > 0 {
		now := time.Now().UnixMilli()
		last := atomic.LoadInt64(&c.lastEmitUnix)
		if now-last < c.CooldownMs {
			return
		}
		atomic.StoreInt64(&c.lastEmitUnix, now)
	}

	if c.OnChatMessage != nil {
		c.OnChatMessage(evt)
	}
}

func (c *EventSubClient) handlePollEvent(raw json.RawMessage, isBegin bool) {
	var p struct {
		Event PollEvent `json:"event"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	if isBegin && c.OnPollBegin != nil {
		c.OnPollBegin(p.Event)
	} else if !isBegin && c.OnPollEnd != nil {
		c.OnPollEnd(p.Event)
	}
}

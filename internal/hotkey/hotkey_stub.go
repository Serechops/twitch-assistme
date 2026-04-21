//go:build !windows

package hotkey

import "fmt"

// Config describes a hotkey binding.
type Config struct {
	Type      string   `json:"type"`
	Modifiers []string `json:"modifiers"`
	Key       string   `json:"key"`
	Button    int      `json:"button"`
}

// DefaultConfig returns the default binding.
func DefaultConfig() Config {
	return Config{Type: "keyboard", Modifiers: []string{"ctrl", "shift"}, Key: "Space"}
}

// Label returns a human-readable description.
func (c Config) Label() string {
	return fmt.Sprintf("%v+%s", c.Modifiers, c.Key)
}

// Manager is a no-op on non-Windows platforms.
type Manager struct{ trigger func() }

func New(trigger func()) *Manager        { return &Manager{trigger: trigger} }
func (m *Manager) Update(_ Config) error { return nil }
func (m *Manager) Stop()                 {}

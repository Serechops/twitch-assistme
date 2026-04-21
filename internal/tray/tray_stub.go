//go:build !windows

// Package tray manages the system tray icon. This stub is a no-op on non-Windows platforms.
package tray

// Callbacks holds the actions the tray can invoke on the rest of the app.
type Callbacks struct {
	Show func()
	Quit func()
}

// Run is a no-op on non-Windows platforms.
func Run(_ Callbacks) {}

// Stop is a no-op on non-Windows platforms.
func Stop() {}

// Package tray manages the Windows system tray icon.
package tray

import (
	_ "embed"

	"github.com/getlantern/systray"
)

//go:embed icon.ico
var iconData []byte

// Callbacks holds the actions the tray can invoke on the rest of the app.
type Callbacks struct {
	Show func()
	Quit func()
}

// Run starts the system tray. It blocks until Quit is called and must be
// invoked from its own goroutine (or as the last call in main).
func Run(cb Callbacks) {
	systray.Run(func() { onReady(cb) }, nil)
}

// Stop tears down the tray icon. Safe to call from any goroutine.
func Stop() {
	systray.Quit()
}

func onReady(cb Callbacks) {
	systray.SetIcon(iconData)
	systray.SetTitle("Twitch AssistMe")
	systray.SetTooltip("Twitch AssistMe")

	mShow := systray.AddMenuItem("Show Window", "Bring the window to front")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit Twitch AssistMe")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				if cb.Show != nil {
					cb.Show()
				}
			case <-mQuit.ClickedCh:
				if cb.Quit != nil {
					cb.Quit()
				}
				systray.Quit()
				return
			}
		}
	}()
}

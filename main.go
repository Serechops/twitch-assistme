package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Load .env if present — only needed for optional TWITCH_AISSISTME_SECRET_KEY.
	// End users do not need a .env file; the app works out of the box via Device Code Flow.
	if ex, err := os.Executable(); err == nil {
		_ = godotenv.Load(filepath.Join(filepath.Dir(ex), ".env"))
	}
	_ = godotenv.Load() // fallback: cwd

	// Assign after godotenv so the env var is actually populated.
	// Package-level var init in app.go runs before main(), before godotenv loads.
	twitchClientSecret = os.Getenv("TWITCH_AISSISTME_SECRET_KEY")

	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "Twitch AssistMe",
		Width:     1100,
		Height:    720,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 14, G: 14, B: 18, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

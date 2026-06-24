package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/svpchain/svpchain-agent/internal/desktop"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := desktop.NewApp()

	err := wails.Run(&options.App{
		Title:     app.WindowTitle(),
		Width:     900,
		Height:    640,
		MinWidth:  640,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.Startup,
		Bind:      []interface{}{app},
		Mac: &mac.Options{
			// TitleBarHidden (no toolbar) keeps traffic lights vertically centered;
			// HiddenInset + UseToolbar pins them to the bottom of the toolbar strip.
			TitleBar: mac.TitleBarHidden(),
			About: &mac.AboutInfo{
				Title:   "svpchain agent",
				Message: "Local-key trading assistant for svpchain",
			},
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}

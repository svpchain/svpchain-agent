package main

import (
	"embed"
	"net/http"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/svpchain/svpchain-agent/internal/brand"
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
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// The app embeds a new frontend bundle on every build. Avoid
					// WebKit reusing an older bundle from its persistent cache.
					w.Header().Set("Cache-Control", "no-store")
					next.ServeHTTP(w, r)
				})
			},
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Bind:       []interface{}{app},
		Mac: &mac.Options{
			// TitleBarHidden (no toolbar) keeps traffic lights vertically centered;
			// HiddenInset + UseToolbar pins them to the bottom of the toolbar strip.
			TitleBar: mac.TitleBarHidden(),
			About: &mac.AboutInfo{
				Title:   brand.AppDisplayName,
				Message: "Local-key on-chain assistant for svpchain",
			},
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}

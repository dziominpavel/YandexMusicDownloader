// Command app is the YandexMusicDownloader desktop entry point.
package main

import (
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"yamdl/frontend"
	"yamdl/internal/ui"
)

func main() {
	assets, err := frontend.Assets()
	if err != nil {
		log.Fatal(err)
	}
	app := ui.NewApp()
	width, height := app.InitialSize()
	if err := wails.Run(&options.App{
		Title:  "YandexMusicDownloader",
		Width:  width,
		Height: height,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:     app.Startup,
		OnBeforeClose: app.BeforeClose,
		Bind:          []any{app},
	}); err != nil {
		log.Fatal(err)
	}
}

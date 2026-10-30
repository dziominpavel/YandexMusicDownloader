// Command app is the YandexMusicDownloader desktop entry point.
package main

import (
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"yamdl/frontend"
	"yamdl/internal/applog"
	"yamdl/internal/ui"
)

func main() {
	// Open the diagnostic log before anything else, so startup errors
	// found before the window exists still reach the file.
	appLog, logErr := applog.OpenDefault(applog.MaxBytes)
	app := ui.NewApp()
	if logErr != nil {
		app.SetLogFileError(logErr)
	} else {
		defer appLog.Close()
		app.SetLogFile(appLog)
	}

	assets, err := frontend.Assets()
	if err != nil {
		fatal(appLog, "assets: "+err.Error(), err)
	}
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
		fatal(appLog, "wails: "+err.Error(), err)
	}
}

// fatal writes the last error to the diagnostic log (when open) and then
// exits; the window may not exist yet.
func fatal(l *applog.Logger, msg string, err error) {
	if l != nil {
		_ = l.Write(msg, "error")
	}
	log.Fatal(err)
}

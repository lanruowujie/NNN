// Package desktop wires the application service to the Wails desktop shell.
package desktop

import (
	"io/fs"

	nfcxapp "github.com/BennyThink/NFCX/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// Run starts the NFCX desktop window using the supplied embedded frontend.
func Run(assets fs.FS) error {
	application := nfcxapp.NewApplication()
	return wails.Run(&options.App{
		Title:         "NFCX",
		Width:         1280,
		Height:        820,
		DisableResize: false,
		MinWidth:      1040,
		MinHeight:     680,
		Mac: &mac.Options{
			DisableZoom: false,
		},
		BackgroundColour: &options.RGBA{R: 12, G: 18, B: 27, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  application.Startup,
		OnShutdown: application.Shutdown,
		Bind: []interface{}{
			application.Bindings(),
		},
	})
}

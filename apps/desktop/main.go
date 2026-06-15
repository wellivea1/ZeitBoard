package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	application := NewApp()
	err := wails.Run(&options.App{
		Title:             "ZeitBoard",
		Width:             1180,
		Height:            760,
		MinWidth:          900,
		MinHeight:         620,
		HideWindowOnClose: true,
		AssetServer:       &assetserver.Options{Assets: assets},
		BackgroundColour:  &options.RGBA{R: 244, G: 246, B: 248, A: 1},
		OnStartup:         application.startup,
		OnShutdown:        application.shutdown,
		OnBeforeClose:     application.beforeClose,
		Bind:              []interface{}{application},
	})
	if err != nil {
		log.Fatal(err)
	}
}

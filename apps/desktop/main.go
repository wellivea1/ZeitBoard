package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	application := NewApp()
	application.window.startHidden = launchedInBackground(os.Args[1:])
	dir, err := desktopDataDir()
	var instanceLock *options.SingleInstanceLock
	if err == nil {
		instanceLock = &options.SingleInstanceLock{UniqueId: desktopInstanceID(dir), OnSecondInstanceLaunch: application.onSecondInstance}
	}
	err = wails.Run(&options.App{
		Title:              "ZeitBoard",
		Width:              1180,
		Height:             760,
		MinWidth:           900,
		MinHeight:          620,
		StartHidden:        application.window.startHidden,
		AssetServer:        &assetserver.Options{Assets: assets},
		BackgroundColour:   &options.RGBA{R: 244, G: 246, B: 248, A: 1},
		OnStartup:          application.startup,
		OnDomReady:         application.onDomReady,
		OnShutdown:         application.shutdown,
		OnBeforeClose:      application.beforeClose,
		SingleInstanceLock: instanceLock,
		Bind:               []interface{}{application},
	})
	if err != nil {
		log.Fatal(err)
	}
}

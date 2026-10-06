package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

func main() {
	app := NewApp()
	config := app.GetConfig()
	err := wails.Run(&options.App{
		Title:    "进程亲和性管理器",
		Width:    config.General.WindowWidth,
		Height:   config.General.WindowHeight,
		MinWidth: 900, MinHeight: 650,
		MaxWidth: 3840, MaxHeight: 2160,
		BackgroundColour: &options.RGBA{R: 243, G: 245, B: 248, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Windows:          &windows.Options{Theme: windows.Light, DisablePinchZoom: true},
	})
	if err != nil {
		log.Fatal("启动失败：", err)
	}
}

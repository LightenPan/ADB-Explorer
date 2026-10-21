// Command adbexplorer is the native ADB Explorer client: a window MyGo
// draws itself, talking to the local ADB server directly.
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	app := newApp()
	mygo.App.WhenReady(func() {
		app.win = mygo.NewWindow(mygo.WindowOptions{
			Title:           "ADB Explorer",
			Width:           1180,
			Height:          760,
			MinWidth:        760,
			MinHeight:       480,
			StateKey:        "main",
			TitleBarStyle:   mygo.TitleBarHidden,
			TitleBarHeight:  40,
			BackgroundColor: "#f6f6f6",
			Content:         ui.View(app.view),
		})
		if err := app.win.SetIcon(appIconPNG); err != nil {
			log.Printf("window icon: %v", err)
		}
		app.loadSettings()
		app.start()
		app.startPoll()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

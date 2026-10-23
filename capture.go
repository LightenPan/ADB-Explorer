package main

import (
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/oldj/switchhosts/internal/appui"
	"github.com/oldj/switchhosts/internal/store"
)

// captureWindow writes a PNG of the real window when SWH_CAPTURE names a
// path, then quits. Development builds use it to check what the window
// draws.
func captureWindow(win *mygo.Window) {
	path := os.Getenv("SWH_CAPTURE")
	if path == "" {
		return
	}
	delay := 1500 * time.Millisecond
	if v := os.Getenv("SWH_CAPTURE_DELAY_MS"); v != "" {
		if ms, err := time.ParseDuration(v + "ms"); err == nil {
			delay = ms
		}
	}
	go func() {
		time.Sleep(delay)
		png, err := win.CapturePage()
		if err != nil {
			log.Printf("capture: %v", err)
		} else if err := os.WriteFile(path, png, 0o644); err != nil {
			log.Printf("capture: %v", err)
		} else {
			log.Printf("captured %s", path)
		}
		if os.Getenv("SWH_CAPTURE_QUIT") != "" {
			mygo.App.Quit()
		}
	}()
}

var _ = store.Paths{}
var _ = appui.App{}

package main

import (
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oldj/switchhosts/internal/appui"
	"github.com/oldj/switchhosts/internal/store"
)

var quitting atomic.Bool

// windows records the app's window for geometry persistence.
var windows struct {
	sync.Mutex
	win *mygo.Window
	// restoreBounds is the size a maximized window returns to, which a
	// maximized window's own bounds do not show.
	restoreBounds store.Geometry
}

// uiView adapts the app's view to MyGo's window content.
func uiView(app *appui.App) *ui.Content {
	return ui.View(app.View)
}

// loadGeometry returns the saved window bounds.
func loadGeometry(app *appui.App) (store.Geometry, bool) {
	geometry, ok := app.Geometry()
	if !ok || geometry == nil {
		return store.Geometry{}, false
	}
	if geometry.Width < 300 || geometry.Height < 200 {
		return store.Geometry{}, false
	}
	return *geometry, true
}

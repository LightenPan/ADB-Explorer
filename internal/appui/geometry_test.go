package appui

import (
	"testing"

	"github.com/oldj/switchhosts/internal/store"
)

// TestGeometryKeepsOtherState checks saving the window leaves the collapse
// state and the rest of the state file alone.
func TestGeometryKeepsOtherState(t *testing.T) {
	app := newTestApp(t)
	if err := store.SaveManifest(app.Paths(), &store.Manifest{Root: []store.NodeJSON{
		{"id": "f", "title": "Folder", "type": "folder", "is_collapsed": true,
			"children": []store.NodeJSON{}},
		{"id": "local-1", "title": "Development", "type": "local", "on": true},
	}}); err != nil {
		t.Fatal(err)
	}
	app.StoreGeometry(store.Geometry{X: 10, Y: 20, Width: 900, Height: 600})

	state := store.LoadState(app.Paths())
	if state.WindowMain == nil || state.WindowMain.Width != 900 {
		t.Fatalf("the window was not saved: %+v", state.WindowMain)
	}
	if len(state.Collapsed) != 1 || state.Collapsed[0] != "f" {
		t.Errorf("the collapse state changed: %v", state.Collapsed)
	}
}

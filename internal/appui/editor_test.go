package appui

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oldj/switchhosts/internal/store"
)

// TestEditingAnEntrySavesOnPause checks that typing in the editor writes the
// entry's content file once the pause elapses, which the earlier build never
// did.
func TestEditingAnEntrySavesOnPause(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.View, 1000, 640)
	if app.CurrentID() != "local-1" {
		t.Fatalf("selected %q", app.CurrentID())
	}
	app.SetEditorContent("203.0.113.7 edited.local\n")
	waitFor(t, func() bool {
		content, err := store.ReadEntry(app.paths, "local-1")
		return err == nil && strings.Contains(content, "edited.local")
	})
	tt.Frame()
}

// waitFor polls a condition until it holds or three seconds pass, for work
// the app does on a timer of its own.
func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the condition did not hold in time")
}

// TestEditorPauseDoesNotBlockOpeningAnEntry checks that switching entries
// saves the pending edit first.
func TestEditorPauseDoesNotBlockOpeningAnEntry(t *testing.T) {
	app := newTestApp(t)
	app.SetEditorContent("198.51.100.9 pending.local\n")
	app.Select("local-2")

	content, err := store.ReadEntry(app.paths, "local-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "pending.local") {
		t.Fatalf("the pending edit was lost: %q", content)
	}
}

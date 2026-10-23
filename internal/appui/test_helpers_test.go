package appui

import (
	"testing"

	"github.com/oldj/switchhosts/internal/store"
)

// newTestApp builds an app over a temporary data directory with a few
// entries, as a fresh install has them.
func newTestApp(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	paths := store.Under(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cfg := store.DefaultConfig()
	cfg.WriteMode = "append"
	app := New(paths, cfg, nil)
	app.version = "5.0.3"
	app.config.Theme = "light"
	app.themeMode = 0

	list := []store.NodeJSON{
		{"id": "local-1", "title": "Development", "type": "local", "on": true},
		{"id": "local-2", "title": "API Override", "type": "local", "on": false},
		{"id": "remote-1", "title": "Remote Blocklist", "type": "remote", "on": false,
			"url": "https://example.test/hosts", "refresh_interval": 3600},
		{"id": "group-1", "title": "Work Stack", "type": "group", "on": false,
			"include": []any{"local-1", "remote-1"}},
		{"id": "folder-1", "title": "Project A", "type": "folder", "on": false,
			"folder_mode": 2, "children": []any{
				store.NodeJSON{"id": "local-3", "title": "env1", "type": "local", "on": false},
				store.NodeJSON{"id": "local-4", "title": "env2", "type": "local", "on": false},
			}},
	}
	for _, node := range list {
		id := store.NodeID(node)
		if err := store.WriteEntry(paths, id, "127.0.0.1 "+id+".local\n::1 "+id+".local\n"); err != nil {
			t.Fatal(err)
		}
	}
	app.list = list
	app.loaded = true
	app.currentID = "local-1"
	app.currentKind = "local"
	app.loadEditor()
	return app
}

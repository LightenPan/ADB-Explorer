package main

import (
	"log"
	"os"
	"time"

	"github.com/oldj/switchhosts/internal/appui"
	"github.com/oldj/switchhosts/internal/hostswrite"
	"github.com/oldj/switchhosts/internal/store"
)

// selfCheck exercises the app's write path without touching the window when
// SWH_SELF_CHECK is set, so a build can be checked on a real machine: it
// selects a local entry, edits it and reports what the entry and the
// aggregated hosts content hold. It never writes the system file, so it does
// not raise an elevation prompt.
func selfCheck(app *appui.App) {
	if os.Getenv("SWH_SELF_CHECK") == "" {
		return
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		var target store.NodeJSON
		for _, node := range store.Flatten(app.List()) {
			if store.NodeType(node) == "local" {
				target = node
				break
			}
		}
		if target == nil {
			log.Printf("self-check: no local entry to edit")
			os.Exit(1)
		}
		id := store.NodeID(target)
		app.Select(id)
		app.SetEditorContent("# self-check\n203.0.113.7 selfcheck.local\n")
		time.Sleep(1500 * time.Millisecond)

		content, err := store.ReadEntry(app.Paths(), id)
		if err != nil {
			log.Printf("self-check: reading %s failed: %v", id, err)
		} else {
			log.Printf("self-check: entry %s holds %q", id, content)
		}
		aggregated, err := hostswrite.Aggregate(app.List(), app.Paths(), false)
		if err != nil {
			log.Printf("self-check: aggregating failed: %v", err)
		} else {
			log.Printf("self-check: aggregated hosts %q", aggregated)
		}
		ok, reason := hostswrite.CanWriteHosts()
		log.Printf("self-check: can write hosts: %v (elevated=%v) %s", ok, hostswrite.Elevated(), clip(reason))
		os.Exit(0)
	}()
}

// clip shortens text for a log line.
func clip(s string) string {
	const limit = 200
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}

package main

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// pendingUpload is a local file that already exists on the device.
// The user chooses replace or skip before it is enqueued.
type pendingUpload struct {
	local string
	name  string
}

var conflicts []pendingUpload

// clipboardFiles reads paths copied from Explorer. Tests replace it because
// the headless tester has no system clipboard.
var clipboardFiles = func() ([]string, error) {
	return mygo.Clipboard.ReadFiles()
}

// pasteClipboard uploads files copied in Explorer (or another app) into the
// focused device folder. Text on the clipboard is left alone.
func (a *app) pasteClipboard() {
	if a.serial == "" || a.page != pageExplorer {
		return
	}
	files, err := clipboardFiles()
	if err != nil || len(files) == 0 {
		if err != nil {
			a.status = err.Error()
		}
		return
	}
	a.status = a.trf("Pasting %d item(s)", len(files))
	for _, f := range files {
		a.queueUpload(f)
	}
}

func (a *app) queueUpload(local string) {
	if local == "" || a.serial == "" {
		return
	}
	info, err := os.Stat(local)
	if err != nil {
		a.status = err.Error()
		return
	}
	if info.IsDir() {
		a.upload(local)
		return
	}
	name := filepath.Base(local)
	if a.has(name) {
		for _, c := range conflicts {
			if c.local == local {
				return
			}
		}
		conflicts = append(conflicts, pendingUpload{local: local, name: name})
		return
	}
	a.upload(local)
}

func (a *app) conflictName() string {
	if len(conflicts) == 0 {
		return ""
	}
	return conflicts[0].name
}

func (a *app) resolveConflict(replace bool) {
	if len(conflicts) == 0 {
		return
	}
	item := conflicts[0]
	conflicts = conflicts[1:]
	if replace {
		a.upload(item.local)
		return
	}
	a.status = a.tr("Skipped")
}

// decideUpload is the pure form of the name-conflict choice.
// chosen means the user already answered the prompt.
func decideUpload(exists, replace, chosen bool) string {
	if !exists {
		return "upload"
	}
	if !chosen {
		return "ask"
	}
	if replace {
		return "upload"
	}
	return "skip"
}

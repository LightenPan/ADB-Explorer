package appui

import (
	"time"

	"github.com/oldj/switchhosts/internal/hostswrite"
	"github.com/oldj/switchhosts/internal/store"
)

// Host is what the app needs from the window that shows it: drawing again
// after a background action changed the state, and quitting.
type Host interface {
	// Refresh draws a new frame.
	Refresh()
	// Quit asks the application to quit.
	Quit()
	// WindowTitle sets the native window's title.
	WindowTitle(title string)
	// SelectedPath records and restores the window's geometry.
	SaveGeometry()
	// PickDataDir asks for a new data folder and returns when done.
	PickDataDir()
	// Minimize, ToggleMaximize and Close are the window controls the title
	// bar draws where the desktop has none.
	Minimize()
	ToggleMaximize()
	Close()
	// HideToTray hides the window, leaving the app running in the tray.
	HideToTray()
	// RestartElevated starts the app again with administrator rights.
	RestartElevated() error
}

// SetHost attaches the window showing the app.
func (a *App) SetHost(h Host) { a.host = h }

// refresh draws the window again, from any goroutine.
func (a *App) refresh() {
	if a.host != nil {
		a.host.Refresh()
	}
}

// toggleAsync applies a toggle without blocking the frame.
func (a *App) toggleAsync(id string, on bool) {
	var pending dialogPending
	needMode := false
	a.mu.Lock()
	if a.config.WriteMode == "" {
		needMode = true
		pending = dialogPending{ID: id, On: on}
	}
	a.mu.Unlock()
	if needMode {
		a.mu.Lock()
		a.openWriteModeDialog(pending)
		a.mu.Unlock()
		a.refresh()
		return
	}
	a.refresh()
	go func() {
		err := a.Toggle(id, on)
		a.mu.Lock()
		if err != nil {
			a.addToast(a.T("fail"), err.Error(), true)
		}
		a.mu.Unlock()
		a.refresh()
	}()
}

// applyPendingToggle is the toggle that waited for the write mode dialog.
func (a *App) applyPendingToggle(id string, on bool) {
	err := a.Toggle(id, on)
	a.mu.Lock()
	if err != nil {
		a.addToast(a.T("fail"), err.Error(), true)
	}
	a.mu.Unlock()
	a.refresh()
}

// refreshAsync refreshes a remote entry in the background.
func (a *App) refreshAsync(id string) {
	go func() {
		client := hostswrite.HTTPClient(a.config)
		result, err := hostswrite.RefreshNode(client, a.paths, id)
		a.mu.Lock()
		switch {
		case err != nil:
			a.addToast(a.T("refresh"), err.Error(), true)
		case result != nil && !result.Success:
			message := result.Message
			if message == "" {
				message = result.Code
			}
			a.addToast(a.T("refresh"), message, true)
		}
		if err == nil {
			_ = a.Load()
		}
		a.mu.Unlock()
		a.refresh()
	}()
}

// trashAsync moves entries to the trashcan in the background.
func (a *App) trashAsync(ids []string) {
	go func() {
		err := a.MoveToTrashcan(ids)
		a.mu.Lock()
		if err != nil {
			a.addToast(a.T("fail"), err.Error(), true)
		}
		a.mu.Unlock()
		a.refresh()
	}()
}

// restoreAsync restores an entry from the trashcan.
func (a *App) restoreAsync(id string) {
	go func() {
		err := a.RestoreFromTrashcan(id)
		a.mu.Lock()
		if err != nil {
			a.addToast(a.T("trashcan_restore"), err.Error(), true)
		}
		a.mu.Unlock()
		a.refresh()
	}()
}

// deleteAsync deletes an entry from the trashcan for good.
func (a *App) deleteAsync(id string) {
	go func() {
		err := a.DeleteFromTrashcan(id)
		a.mu.Lock()
		if err != nil {
			a.addToast(a.T("hosts_delete"), err.Error(), true)
		}
		a.mu.Unlock()
		a.refresh()
	}()
}

// ApplyAll reapplies the saved selection, as the recovery notice and the
// tray menu ask.
func (a *App) ApplyAll() {
	go func() {
		err := a.applyList()
		if err != nil {
			a.addToast(a.T("fail"), err.Error(), true)
		}
		a.refresh()
	}()
}

// StartRefreshLoop refreshes remote entries on their intervals.
func (a *App) StartRefreshLoop() {
	go func() {
		if a.config.RefreshRemoteOnStartup {
			a.refreshDueRemotes(true)
		}
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			a.refreshDueRemotes(false)
		}
	}()
}

// refreshDueRemotes refreshes the entries whose interval elapsed.
func (a *App) refreshDueRemotes(startup bool) {
	a.mu.Lock()
	type due struct {
		id       string
		interval int
	}
	var ids []due
	nowMS := store.NowMillis()
	for _, node := range store.Flatten(a.list) {
		if store.NodeType(node) != "remote" {
			continue
		}
		interval, _ := numberField(node["refresh_interval"])
		seconds := int(interval)
		if seconds <= 0 {
			continue
		}
		last, _ := numberField(node["last_refresh_ms"])
		if !startup && last > 0 && nowMS-int64(last) < int64(seconds)*1000 {
			continue
		}
		ids = append(ids, due{id: store.NodeID(node), interval: seconds})
	}
	client := hostswrite.HTTPClient(a.config)
	a.mu.Unlock()
	if len(ids) == 0 {
		return
	}
	changed := false
	for _, item := range ids {
		if _, err := hostswrite.RefreshNode(client, a.paths, item.id); err == nil {
			changed = true
		}
	}
	if changed {
		a.mu.Lock()
		_ = a.Load()
		a.mu.Unlock()
		a.refresh()
	}
}

// RunQuit handles a quit request from the interface.
func (a *App) RunQuit() {
	if a.host != nil {
		a.host.SaveGeometry()
		a.host.Quit()
	}
}

// restartWithPickedDir asks for a new data folder and points the app at it.
func (a *App) restartWithPickedDir() {
	if a.host == nil {
		return
	}
	a.host.PickDataDir()
}

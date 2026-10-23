package appui

import (
	"strings"

	"github.com/oldj/switchhosts/internal/hostswrite"
	"github.com/oldj/switchhosts/internal/store"
)

// Dialog kinds the window can show.
const (
	dialogNone        = ""
	dialogEdit        = "edit"
	dialogPreferences = "preferences"
	dialogWriteMode   = "writeMode"
	dialogHistory     = "history"
	dialogAbout       = "about"
	dialogRecovery    = "recovery"
	dialogFind        = "find"
	dialogPermission  = "permission"
	dialogClose       = "close"
)

// What closing the window does, chosen in the close dialog.
const (
	closeToTray = iota
	closeMinimize
	closeQuit
)

// dialogPending is a toggle waiting for the write mode to be chosen.
type dialogPending struct {
	ID string
	On bool
}

// dialogForm is what a dialog's fields edit.
type dialogForm struct {
	isNew  bool
	nodeID string

	kind        int // 0 local, 1 remote, 2 group, 3 folder
	source      int // 0 url, 1 domain
	title       string
	url         string
	domains     string
	interval    float64
	folderMode  int
	include     []string
	includeText string

	// Preferences.
	tab int

	// History and other lists.
	selected int

	// Recovery dialog.
	recoveryDefault string

	// Message the dialog shows when an action failed.
	error string

	// Write mode.
	writeMode    int // 0 append, 1 overwrite
	hasPending   bool
	pendingID    string
	pendingOn    bool
	historyLimit float64
	confirmShown bool
}

func (a *App) openEditDialog(node store.NodeJSON) {
	form := dialogForm{kind: 0, folderMode: 0}
	if node != nil {
		form.isNew = false
		form.nodeID = store.NodeID(node)
		form.title = store.NodeTitle(node)
		switch store.NodeType(node) {
		case "remote":
			form.kind = 1
			form.url, _ = node["url"].(string)
			if source, _ := node["source"].(string); source == "domain" {
				form.source = 1
			}
			if domains, ok := node["domains"].([]any); ok {
				var list []string
				for _, d := range domains {
					if s, ok := d.(string); ok {
						list = append(list, s)
					}
				}
				form.domains = strings.Join(list, "\n")
			} else if form.url != "" {
				form.domains = form.url
			}
			if v, ok := numberField(node["refresh_interval"]); ok {
				form.interval = v / 3600
			}
		case "group":
			form.kind = 2
			if include, ok := node["include"].([]any); ok {
				for _, id := range include {
					if s, ok := id.(string); ok {
						form.include = append(form.include, s)
					}
				}
			}
		case "folder":
			form.kind = 3
			if v, ok := numberField(node["folder_mode"]); ok {
				form.folderMode = int(v)
			}
		}
	} else {
		form.isNew = true
		form.title = ""
	}
	a.dialogForm = form
	a.dialogKind = dialogEdit
}

func numberField(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func (a *App) openPreferences() {
	a.dialogForm = dialogForm{tab: 0}
	a.dialogKind = dialogPreferences
}

func (a *App) openWriteModeDialog(pending dialogPending) {
	mode := 0
	if a.config.WriteMode == "overwrite" {
		mode = 1
	}
	a.dialogForm = dialogForm{
		writeMode:  mode,
		hasPending: pending.ID != "",
		pendingID:  pending.ID,
		pendingOn:  pending.On,
	}
	a.dialogKind = dialogWriteMode
}

func (a *App) openHistory() {
	a.dialogForm = dialogForm{}
	a.dialogKind = dialogHistory
}

// openFind opens the find dialog, keeping the last query.
func (a *App) openFind() {
	a.dialogKind = dialogFind
}

// ShowCloseDialog asks what closing the window should do. The window's own
// close button calls it, so that a click on ✕ never loses the app by
// accident.
func (a *App) ShowCloseDialog() {
	if a.config.QuitOnClose {
		// The preference says closing quits without asking.
		a.host.Quit()
		return
	}
	a.dialogForm.error = ""
	a.dialogKind = dialogClose
}

// chooseClose applies the close dialog's choice, then closes it.
func (a *App) chooseClose(choice int) {
	a.closeDialog()
	switch choice {
	case closeToTray:
		a.host.HideToTray()
	case closeMinimize:
		a.host.Minimize()
	case closeQuit:
		a.host.Quit()
	}
}

func (a *App) openAbout() {
	a.dialogKind = dialogAbout
}

// closeDialog dismisses the open dialog.
func (a *App) closeDialog() {
	a.dialogKind = dialogNone
}

// historyItems reads the apply history for the dialog.
func (a *App) historyItems() []hostswrite.HistoryItem {
	items, err := hostswrite.LoadHistory(hostswrite.HistoriesPath(a.paths))
	if err != nil {
		return nil
	}
	// Newest first, as the window lists them.
	out := make([]hostswrite.HistoryItem, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		out = append(out, items[i])
	}
	return out
}

// NodeKinds returns the entry types the edit dialog offers.
func (a *App) NodeKinds() []string {
	return []string{a.T("local"), a.T("remote"), a.T("group"), a.T("folder")}
}

// RefreshIntervalOptions are the automatic refresh choices, in hours.
func (a *App) RefreshIntervalOptions() []float64 {
	return []float64{0, 0.25, 1, 6, 12, 24, 72, 168}
}

// FormatInterval renders a refresh interval for the interface.

func (a *App) FormatInterval(seconds int) string {
	if seconds <= 0 {
		return a.T("never")
	}
	if seconds < 60 {
		return itoa(int64(seconds)) + " s"
	}
	minutes := (seconds + 30) / 60
	if minutes < 60 {
		unit := a.T("minutes")
		if minutes == 1 {
			unit = a.T("minute")
		}
		return itoa(int64(minutes)) + " " + unit
	}
	hours := (minutes + 30) / 60
	if hours < 24 {
		unit := a.T("hours")
		if hours == 1 {
			unit = a.T("hour")
		}
		return itoa(int64(hours)) + " " + unit
	}
	days := (hours + 12) / 24
	unit := a.T("days")
	if days == 1 {
		unit = a.T("day")
	}
	return itoa(int64(days)) + " " + unit
}

func trimFloat(v float64) string {
	s := strings.TrimRight(strings.TrimRight(formatFloat(v), "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}

func formatFloat(v float64) string {
	if v == float64(int64(v)) {
		return itoa(int64(v))
	}
	return strings.TrimRight(strings.TrimRight(sprintf("%.2f", v), "0"), ".")
}

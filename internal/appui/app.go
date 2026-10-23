// Package appui draws SwitchHosts' interface with MyGo's native UI: the
// hosts list, editor and details panel, drawn by Go.
package appui

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/oldj/switchhosts/internal/hostswrite"
	"github.com/oldj/switchhosts/internal/i18n"
	"github.com/oldj/switchhosts/internal/store"
)

// App is the whole state of the interface. The main window's view reads it
// on the main thread; work started by the interface changes it through
// Window.Update.
type App struct {
	mu sync.Mutex

	paths    store.Paths
	config   *store.Config
	tr       *i18n.Translator
	recovery *store.Recovery
	apply    *hostswrite.Recovery

	// Data.
	list      []store.NodeJSON
	trashcan  []store.TrashItem
	version   string
	loaded    bool
	loadError string

	// Selection and panes.
	currentID    string
	currentKind  string // "system" or a node id
	leftView     int    // 0 entries, 1 trashcan
	leftOpen     bool
	rightOpen    bool
	leftWidth    float32
	rightWidth   float32
	themeMode    int // 0 light, 1 dark, 2 system
	platformDark bool

	// Editor.
	editorContent string
	editorID      string
	editorDirty   bool
	editorError   string
	statusText    string
	editor        *editorState
	saveScheduled bool

	// Dialogs.
	dialogKind string
	dialogForm dialogForm
	// find is the find dialog's search state.
	find findState

	// Toasts.
	toasts map[string]toastEntry

	// quitRequested asks the window's host to quit the app.
	quitRequested bool
	// windowAction is the window control the title bar pressed.
	windowAction int

	// host is the window showing the app.
	host Host
	// tray is the menu bar icon, when the app has one.
	tray any

	// permission describes the missing right to write the hosts file, ""
	// when the app has it.
	permission          string
	permissionPath      string
	permissionDismissed bool
	// canElevate reports whether a restart can ask for the right.
	canElevate bool
	elevated   bool
	// elevationError is why the last restart attempt failed.
	elevationError string
	// exitForElevation asks the host to quit so the elevated instance runs.
	exitForElevation bool
}

type toastEntry struct {
	ID       string
	Title    string
	Message  string
	Error    bool
	ExpireAt int64
}

// New builds the app state from the data directory.
func New(paths store.Paths, cfg *store.Config, recovery *store.Recovery) *App {
	a := &App{
		paths:      paths,
		config:     cfg,
		recovery:   recovery,
		apply:      &hostswrite.Recovery{},
		toasts:     map[string]toastEntry{},
		leftOpen:   cfg.LeftPanelShow,
		rightOpen:  cfg.RightPanelShow,
		leftWidth:  float32(cfg.LeftPanelWidth),
		rightWidth: float32(cfg.RightPanelWidth),
		leftView:   0,
	}
	a.setLocale(cfg.Locale)
	switch cfg.Theme {
	case "light":
		a.themeMode = 0
	case "dark":
		a.themeMode = 1
	default:
		a.themeMode = 2
	}
	return a
}

func (a *App) setLocale(locale string) {
	a.tr = i18n.New(locale)
}

// T returns a translated message.
func (a *App) T(key string) string { return a.tr.T(key) }

// F returns a translated message with its placeholders filled.
func (a *App) F(key string, args ...string) string { return a.tr.F(key, args...) }

// Language returns the language key the app shows.
func (a *App) Language() string { return a.tr.Locale }

// Paths returns the data layout.
func (a *App) Paths() store.Paths { return a.paths }

// Config returns the preferences.
func (a *App) Config() *store.Config { return a.config }

// List returns the hosts tree.
func (a *App) List() []store.NodeJSON { return a.list }

// Trashcan returns the trashcan items.
func (a *App) Trashcan() []store.TrashItem { return a.trashcan }

// CurrentID returns the selected node's id, "0" for the system hosts.
func (a *App) CurrentID() string { return a.currentID }

// Current returns the selected node, nil for the system hosts. A trashed
// entry is still current while the trashcan shows it.
func (a *App) Current() store.NodeJSON {
	if a.currentID == "" || a.currentID == "0" {
		return nil
	}
	if node := store.FindNode(a.list, a.currentID); node != nil {
		return node
	}
	for _, item := range a.trashcan {
		if store.NodeID(item.Data) == a.currentID {
			return item.Data
		}
	}
	return nil
}

// IsReadOnly reports whether the selected entry can't be edited in place.
func (a *App) IsReadOnly() bool {
	node := a.Current()
	return a.currentID == "0" || node == nil || isReadOnlyType(store.NodeType(node))
}

func isReadOnlyType(kind string) bool {
	switch kind {
	case "group", "remote", "folder", "trashcan":
		return true
	}
	return false
}

// InTrashcan reports whether an id is in the trashcan.
func (a *App) InTrashcan(id string) bool {
	for _, item := range a.trashcan {
		if store.NodeID(item.Data) == id {
			return true
		}
	}
	return false
}

// Load reads the tree, trashcan and the selected entry's content.
func (a *App) Load() error {
	if a.recovery != nil {
		a.loadError = ""
		return nil
	}
	m, err := store.LoadManifest(a.paths)
	if err != nil {
		a.loadError = err.Error()
		return err
	}
	t, err := store.LoadTrashcan(a.paths)
	if err != nil {
		a.loadError = err.Error()
		return err
	}
	a.list = m.Root
	a.trashcan = t.Items
	a.loaded = true
	a.loadError = ""

	if a.currentID == "" {
		a.currentID = "0"
	}
	if a.currentID != "0" && store.FindNode(a.list, a.currentID) == nil && !a.InTrashcan(a.currentID) {
		a.currentID = "0"
	}
	a.loadEditor()
	return nil
}

// loadEditor replaces the editor's text with the selected entry's.
func (a *App) loadEditor() {
	if a.currentID == "" || a.currentID == "0" {
		content, _, err := hostswrite.ReadSystemHosts()
		if err != nil {
			a.editorContent = ""
			a.editorError = err.Error()
		} else {
			a.editorContent = content
			a.editorError = ""
		}
		a.editorID = "0"
		a.editorDirty = false
		return
	}
	content, err := store.ReadEntry(a.paths, a.currentID)
	if err != nil {
		a.editorError = err.Error()
		content = ""
	} else {
		a.editorError = ""
	}
	a.editorContent = content
	a.editorID = a.currentID
	a.editorDirty = false
}

// Select chooses an entry and loads its content. A trashed entry's text is
// its last saved content; the details panel offers to restore it.
func (a *App) Select(id string) {
	if id == a.currentID {
		return
	}
	a.saveEditorIfDirty()
	a.currentID = id
	if id != "0" && store.FindNode(a.list, id) == nil && !a.InTrashcan(id) {
		a.currentID = "0"
	}
	a.loadEditor()
}

// saveEditorIfDirty persists the editor's text.
func (a *App) saveEditorIfDirty() {
	if !a.editorDirty || a.editorID == "" || a.editorID == "0" {
		return
	}
	if err := store.WriteEntry(a.paths, a.editorID, a.editorContent); err != nil {
		a.editorError = err.Error()
		return
	}
	a.editorDirty = false
}

// EditorContent returns the text the editor shows.
func (a *App) EditorContent() string { return a.editorContent }

// SetEditorContent records an edit and schedules the entry's save, so every
// edit path persists the text, not only the frame that draws the editor.
func (a *App) SetEditorContent(content string) {
	if a.editorContent == content {
		return
	}
	a.editorContent = content
	a.editorDirty = true
	a.scheduleEditorSave()
}

// SaveEditor writes the editor's text to its entry now.
func (a *App) SaveEditor() {
	if a.editorID == "" || a.editorID == "0" {
		return
	}
	if err := store.WriteEntry(a.paths, a.editorID, a.editorContent); err != nil {
		a.addToast(a.T("fail"), err.Error(), true)
		return
	}
	a.editorDirty = false
}

// Permission returns the reason the app cannot write the system hosts file,
// "" when it can.
func (a *App) Permission() string { return a.permission }

// PermissionPath returns the hosts file the app could not write.
func (a *App) PermissionPath() string { return a.permissionPath }

// SetPermission records whether the app can write the system hosts file,
// and whether a restart could ask for the right.
func (a *App) SetPermission(reason, path string, canElevate, alreadyElevated bool) {
	a.permission = reason
	a.permissionPath = path
	a.canElevate = canElevate
	a.elevated = alreadyElevated
}

// permissionShowsDialog reports whether the startup dialog is due: the app
// cannot write the file, and the user has not dismissed the notice.
func (a *App) permissionShowsDialog() bool {
	return a.permission != "" && !a.permissionDismissed
}

// TakeExitForElevation reports whether the window should quit so the
// elevated instance takes over.
func (a *App) TakeExitForElevation() bool {
	exit := a.exitForElevation
	a.exitForElevation = false
	return exit
}

// SetupPermissionDialog records the permission state and opens the notice.
func (a *App) SetupPermissionDialog(reason, path string, canElevate, alreadyElevated bool) {
	a.SetPermission(reason, path, canElevate, alreadyElevated)
	if a.permissionShowsDialog() {
		a.dialogKind = dialogPermission
	}
}

// restartElevated asks the host to start the app with administrator rights.
func (a *App) restartElevated() {
	a.elevationError = ""
	if a.host == nil {
		return
	}
	if err := a.host.RestartElevated(); err != nil {
		a.elevationError = err.Error()
		return
	}
	a.exitForElevation = true
}

// dismissPermission closes the notice and keeps working without the right.
func (a *App) dismissPermission() {
	a.permissionDismissed = true
	a.closeDialog()
}

// SaveConfig writes the preferences.
func (a *App) SaveConfig() error {
	return a.config.Save(a.paths)
}

// UpdateConfig applies a patch and saves it.
func (a *App) UpdateConfig(patch map[string]any) error {
	if err := a.config.ApplyPatch(patch); err != nil {
		return err
	}
	if locale, ok := patch["locale"].(string); ok {
		a.setLocale(locale)
	}
	if theme, ok := patch["theme"].(string); ok {
		switch theme {
		case "light":
			a.themeMode = 0
		case "dark":
			a.themeMode = 1
		default:
			a.themeMode = 2
		}
	}
	return a.config.Save(a.paths)
}

// ThemeMode returns the appearance the user chose.
func (a *App) ThemeMode() int { return a.themeMode }

// SetSystemDark records the desktop's appearance.
func (a *App) SetSystemDark(dark bool) { a.platformDark = dark }

// Dark reports whether the window draws the dark theme.
func (a *App) Dark() bool {
	switch a.themeMode {
	case 0:
		return false
	case 1:
		return true
	}
	return a.platformDark
}

// Palette returns the colors the view draws with.
func (a *App) Palette() palette {
	if a.Dark() {
		return darkPalette
	}
	return lightPalette
}

// addToast shows a message in the corner of the window.
func (a *App) addToast(title, message string, isError bool) {
	id := fmt.Sprintf("toast-%d-%d", store.NowMillis(), len(a.toasts))
	a.toasts[id] = toastEntry{
		ID:       id,
		Title:    title,
		Message:  message,
		Error:    isError,
		ExpireAt: store.NowMillis() + 6000,
	}
}

// pruneToasts drops expired messages.
func (a *App) pruneToasts() []toastEntry {
	now := store.NowMillis()
	var out []toastEntry
	for id, entry := range a.toasts {
		if entry.ExpireAt <= now {
			delete(a.toasts, id)
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// HasToasts reports whether any message is showing.
func (a *App) HasToasts() bool { return len(a.toasts) > 0 }

// EntryCount returns how many configurations and folders the trashcan holds.
func (a *App) EntryCount() int {
	count := 0
	for _, node := range store.Flatten(a.list) {
		switch store.NodeType(node) {
		case "local", "remote", "group":
			count++
		}
	}
	return count
}

// EnabledTitles lists the titles of the switched-on entries, for the tray
// title.
func (a *App) EnabledTitles() string {
	var titles []string
	var walk func([]store.NodeJSON)
	walk = func(nodes []store.NodeJSON) {
		for _, node := range nodes {
			if store.NodeOn(node) {
				if title := store.NodeTitle(node); title != "" {
					titles = append(titles, title)
				}
			}
			walk(store.Children(node))
		}
	}
	walk(a.list)
	joined := strings.Join(titles, ",")
	if runes := []rune(joined); len(runes) > 20 {
		return string(runes[:17]) + "..."
	}
	return joined
}

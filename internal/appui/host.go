package appui

import (
	"github.com/oldj/switchhosts/internal/store"
)

// QuitOnClose reports whether closing the window quits the app.
func (a *App) QuitOnClose() bool { return a.config.QuitOnClose }

// SetTray keeps the tray so its menu follows the language.
func (a *App) SetTray(t any) { a.tray = t }

// Tray returns the tray icon, when the app has one.
func (a *App) Tray() any { return a.tray }

// AddEntry adds a local entry, as File → New does.
func (a *App) AddEntry() { a.CreateLocalEntry() }

// ShowPreferences opens the preferences dialog.
func (a *App) ShowPreferences() { a.openPreferences() }

// ShowHistory opens the history dialog.
func (a *App) ShowHistory() { a.openHistory() }

// ShowAbout opens the about dialog.
func (a *App) ShowAbout() { a.openAbout() }

// ToggleLeftPanel shows or hides the hosts list.
func (a *App) ToggleLeftPanel() { a.setLeftOpen(!a.leftOpen) }

// ToggleRightPanel shows or hides the details panel.
func (a *App) ToggleRightPanel() { a.setRightOpen(!a.rightOpen) }

// Geometry returns the window's saved bounds.
func (a *App) Geometry() (*store.Geometry, bool) {
	state := store.LoadState(a.paths)
	if state.WindowMain == nil {
		return nil, false
	}
	return state.WindowMain, true
}

// StoreGeometry records the window's bounds.
func (a *App) StoreGeometry(geometry store.Geometry) {
	state := store.LoadState(a.paths)
	state.WindowMain = &geometry
	_ = state.Save(a.paths)
}

// Version returns the app's version.
func (a *App) Version() string { return a.version }

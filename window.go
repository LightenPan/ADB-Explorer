package main

import (
	"embed"
	"log"
	"runtime"

	"github.com/oldj/switchhosts/internal/hostswrite"

	"github.com/egoist/mygo"
	"github.com/oldj/switchhosts/internal/appui"
	"github.com/oldj/switchhosts/internal/i18n"
	"github.com/oldj/switchhosts/internal/store"
)

//go:embed assets/icon.png
var iconPNG []byte

//go:embed assets/tray.png
var trayPNG []byte

//go:embed assets/tray-mac.png
var trayMacPNG []byte

var _ embed.FS

// windowHost connects the app's view to its native window.
type windowHost struct {
	app *appui.App
	win *mygo.Window
}

func (h *windowHost) Refresh() { h.win.Invalidate() }

func (h *windowHost) Quit() { quitApp(h.app) }

func (h *windowHost) WindowTitle(title string) { h.win.SetTitle(title) }

func (h *windowHost) SaveGeometry() { saveGeometry(h.app) }

// RestartElevated starts the app again with administrator rights, which the
// hosts file needs where the user cannot write it directly.
func (h *windowHost) RestartElevated() error {
	return hostswrite.RelaunchElevated()
}

func (h *windowHost) Minimize() { h.win.Minimize() }

func (h *windowHost) ToggleMaximize() { h.win.ToggleMaximize() }

func (h *windowHost) Close() { h.app.ShowCloseDialog() }

// HideToTray hides the window and leaves the app running in the tray, which
// the tray icon brings back.
func (h *windowHost) HideToTray() { h.win.Hide() }

// watchGeometry saves the window's size as the user resizes or moves it, so
// a session ended in any way keeps the last one. A maximized window keeps
// the size it restores to, which is what reopening should use.
func watchGeometry(app *appui.App, win *mygo.Window) {
	save := func() {
		if win.IsMaximized() {
			// The maximized bounds cover the screen and would restore to a
			// window with no room for its own frame.
			windows.Lock()
			restored := windows.restoreBounds
			windows.Unlock()
			if restored.Width >= 300 && restored.Height >= 200 {
				restored.Maximized = true
				app.StoreGeometry(restored)
			}
			return
		}
		geometry := currentGeometry(win)
		if geometry.Width < 300 || geometry.Height < 200 {
			return // a transient state before the window settles
		}
		windows.Lock()
		windows.restoreBounds = geometry
		windows.Unlock()
		app.StoreGeometry(geometry)
	}
	win.OnResize(save)
	win.OnMove(save)
}

// quitApp records the window's size and quits, whichever way the app ends:
// the window's close button, the tray menu, the app menu or the system.
func quitApp(app *appui.App) {
	saveGeometry(app)
	quitting.Store(true)
	mygo.App.Quit()
}

func (h *windowHost) PickDataDir() {
	dir, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:     "SwitchHosts",
		Directory: true,
		Parent:    h.win,
	})
	if err != nil || len(dir) == 0 {
		return
	}
	root, _ := store.DefaultRoot()
	target, _ := store.ResolveTarget(dir[0], root)
	if err := store.WritePointer(target); err == nil {
		mygo.App.Relaunch()
	}
}

// newMainWindow creates the app's window with its saved geometry.
func newMainWindow(app *appui.App) *mygo.Window {
	restoreMaximized := false
	opts := mygo.WindowOptions{
		Title:           "SwitchHosts",
		Width:           800,
		Height:          480,
		MinWidth:        300,
		MinHeight:       200,
		BackgroundColor: backgroundFor(app),
		Content:         uiView(app),
	}
	if runtime.GOOS == "darwin" {
		opts.TitleBarStyle = mygo.TitleBarHidden
		opts.TrafficLightPosition = &mygo.Point{X: 12, Y: 22}
	} else {
		opts.TitleBarStyle = mygo.TitleBarHidden
		opts.HideWindowButtons = true
		opts.TitleBarHeight = 40
	}
	if geometry, ok := loadGeometry(app); ok {
		opts.X, opts.Y = geometry.X, geometry.Y
		opts.Width, opts.Height = geometry.Width, geometry.Height
		restoreMaximized = geometry.Maximized
		windows.Lock()
		windows.restoreBounds = geometry
		windows.Unlock()
	} else {
		opts.Width, opts.Height = 800, 480
	}
	win := mygo.NewWindow(opts)
	host := &windowHost{app: app, win: win}
	app.SetHost(host)
	windows.Lock()
	windows.win = win
	windows.Unlock()
	win.SetIcon(iconPNG)
	watchGeometry(app, win)
	if restoreMaximized {
		win.Maximize()
	}
	win.OnClose(func(e *mygo.CloseEvent) {
		// The window's size belongs to the session, not to a clean quit: the
		// app usually ends up hidden in the tray, never quitting.
		saveGeometry(app)
		if quitting.Load() {
			return
		}
		// Closing asks what to do, as the title bar's button does: a stray
		// Alt+F4 must not end a session that was managing hosts.
		e.PreventDefault()
		app.ShowCloseDialog()
		win.Invalidate()
	})
	_ = app.Load()
	win.Invalidate()
	return win
}

// backgroundFor picks the window color the page shows before the first
// frame, matching the theme.
func backgroundFor(app *appui.App) string {
	if app.Dark() {
		return "#1a1b1e"
	}
	return "#f1f3f5"
}

// saveGeometry records the window's bounds in the data directory. A
// maximized window keeps the size it would return to, so reopening an app
// that was maximized when it closed reproduces both.
func saveGeometry(app *appui.App) {
	windows.Lock()
	defer windows.Unlock()
	if windows.win == nil {
		return
	}
	geometry := currentGeometry(windows.win)
	if geometry.Maximized {
		if windows.restoreBounds.Width >= 300 && windows.restoreBounds.Height >= 200 {
			restored := windows.restoreBounds
			restored.Maximized = true
			geometry = restored
		}
	} else {
		windows.restoreBounds = geometry
	}
	app.StoreGeometry(geometry)
}

// currentGeometry reads the window's placement.
func currentGeometry(win *mygo.Window) store.Geometry {
	bounds := win.Bounds()
	return store.Geometry{
		X:         bounds.X,
		Y:         bounds.Y,
		Width:     bounds.Width,
		Height:    bounds.Height,
		Maximized: win.IsMaximized(),
	}
}

// installTray adds the menu bar icon.
func installTray(app *appui.App, win *mygo.Window) {
	icon := trayPNG
	template := false
	if runtime.GOOS == "darwin" {
		icon = trayMacPNG
		template = true
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "main-tray",
		Icon:           icon,
		IconIsTemplate: template,
		ToolTip:        "SwitchHosts",
		Menu:           trayMenu(app, win),
	})
	if err != nil {
		log.Printf("tray: %v", err)
		return
	}
	app.SetTray(tray)
	tray.OnClick(func() {
		showMain(win)
	})
}

// trayMenu builds the tray's menu from the current language.
func trayMenu(app *appui.App, win *mygo.Window) *mygo.Menu {
	tr := i18n.New(app.Language())
	items := []*mygo.MenuItem{
		{Label: tr.T("show_main_window"), Click: func(*mygo.MenuItem, *mygo.Window) { showMain(win) }},
		{Label: tr.T("quit"), Role: mygo.RoleQuit},
	}
	return mygo.NewMenu(items)
}

func showMain(win *mygo.Window) {
	win.Show()
	win.Focus()
}

// installMenu gives the app its menu bar: File, Edit, View and Help, with
// the actions the interface has.
func installMenu(app *appui.App, win *mygo.Window) {
	tr := i18n.New(app.Language())
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: tr.T("file"), Submenu: []*mygo.MenuItem{
			{Label: tr.T("new"), Accelerator: "CmdOrCtrl+N", Click: func(*mygo.MenuItem, *mygo.Window) {
				app.AddEntry()
				win.Invalidate()
			}},
			mygo.Separator(),
			{Label: tr.T("show_history"), Accelerator: "CmdOrCtrl+Y", Click: func(*mygo.MenuItem, *mygo.Window) {
				app.ShowHistory()
				win.Invalidate()
			}},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Label: tr.T("edit"), Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo},
			{Role: mygo.RoleRedo},
			mygo.Separator(),
			{Role: mygo.RoleCut},
			{Role: mygo.RoleCopy},
			{Role: mygo.RolePaste},
			{Role: mygo.RoleSelectAll},
			mygo.Separator(),
			{Label: tr.T("preferences"), Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) {
				app.ShowPreferences()
				win.Invalidate()
			}},
		}},
		{Label: tr.T("view"), Submenu: []*mygo.MenuItem{
			{Label: tr.T("toggle_left_panel"), Accelerator: "CmdOrCtrl+B", Click: func(*mygo.MenuItem, *mygo.Window) {
				app.ToggleLeftPanel()
				win.Invalidate()
			}},
			{Label: tr.T("toggle_right_panel"), Accelerator: "CmdOrCtrl+Shift+B", Click: func(*mygo.MenuItem, *mygo.Window) {
				app.ToggleRightPanel()
				win.Invalidate()
			}},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
		}},
		{Label: tr.T("help"), Submenu: []*mygo.MenuItem{
			{Label: tr.T("homepage"), Click: func(*mygo.MenuItem, *mygo.Window) {
				_ = mygo.Shell.OpenExternal("https://switchhosts.app")
			}},
			{Label: tr.T("feedback"), Click: func(*mygo.MenuItem, *mygo.Window) {
				_ = mygo.Shell.OpenExternal("https://github.com/oldj/SwitchHosts/issues")
			}},
			mygo.Separator(),
			{Label: tr.T("about"), Click: func(*mygo.MenuItem, *mygo.Window) {
				app.ShowAbout()
				win.Invalidate()
			}},
		}},
	})
	mygo.App.SetMenu(menu)
}

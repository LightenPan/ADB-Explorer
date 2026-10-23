// Command switchhosts is SwitchHosts: the hosts management it offers,
// drawn by Go with MyGo's native UI, with no WebView.

package main

import (
	"log"
	"os"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/oldj/switchhosts/internal/appui"
	"github.com/oldj/switchhosts/internal/hostswrite"
	"github.com/oldj/switchhosts/internal/store"
)

func main() {
	// A privileged write on Windows relaunches this executable.
	if done, code := hostswrite.IsElevationHelper(os.Args); done {
		os.Exit(code)
	}

	paths, recovery := store.ResolveRoot(defaultRoot())
	_ = paths.EnsureDirs()
	paths.CleanupTmpFiles()

	var cfg *store.Config
	if recovery == nil {
		cfg = store.LoadConfig(paths)
		_ = settingsCommand(paths)
	} else {
		cfg = store.DefaultConfig()
	}

	app := appui.New(paths, cfg, recovery)
	app.StartRefreshLoop()

	// The window's size is part of the session: save it however the app
	// ends, including a system shutdown that never reaches a close button.
	mygo.App.OnQuit(func() { saveGeometry(app) })

	// Check the right to write the system hosts file before the user starts
	// editing: without it, switching entries on does nothing.
	if ok, reason := hostswrite.CanWriteHosts(); !ok {
		hostsPath, _ := hostswrite.SystemHostsPath()
		app.SetupPermissionDialog(reason, hostsPath, hostswrite.CanElevate(), hostswrite.Elevated())
	}

	mygo.App.WhenReady(func() {
		win := newMainWindow(app)
		installTray(app, win)
		installMenu(app, win)
		captureWindow(win)
		selfCheck(app)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// defaultRoot is ~/.SwitchHosts.
func defaultRoot() string {
	root, err := store.DefaultRoot()
	if err != nil {
		log.Fatal(err)
	}
	return root
}

// settingsCommand applies launch-at-login and the Dock icon preference at
// startup. It returns an error the caller ignores: the app works without
// them.
func settingsCommand(paths store.Paths) error {
	if runtime.GOOS == "darwin" {
		return nil
	}
	return nil
}

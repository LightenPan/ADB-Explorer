# SwitchHosts

SwitchHosts is a desktop app written in Go, drawn by
[MyGo](https://github.com/egoist/mygo)'s native UI: the window, the lists, the
editor, the dialogs and the tray icon are Go code, and no WebView runs.

## Layout

```
.
├── main.go              the app: data directory, window, tray, menu
├── window.go            the window, its title bar and the app menu
├── host.go              the bridge between the window and the app's state
├── capture.go           writes a PNG of the real window (SWH_CAPTURE)
├── selfcheck.go         checks the write path without a window (SWH_SELF_CHECK)
├── mygo.json            the app's name, identifier, version and icon
├── assets/              the app icon, the tray icons and the installer icon
├── cmd/
│   └── package-windows/ the Windows installer's NSIS script
└── internal/
    ├── appui/           the interface: the view, its dialogs and actions
    ├── store/           the data directory: manifest, entries, trashcan,
    │                    config, state, and the undo journal of writes
    ├── hostswrite/      applying hosts: aggregation, markers, elevation,
    │                    history and the post-apply command
    └── i18n/            the interface languages
```

## Install

`go.mod` depends on the released `github.com/egoist/mygo` module, so a clone
builds without any other checkout.

```bash
go build -o SwitchHosts.exe .
```

## Data

The app keeps its data in `~/.SwitchHosts`, with these files:

- `manifest.json`, `entries/`, `trashcan.json` for the hosts tree and content
- `internal/config.json` for the preferences
- `internal/state.json` for the collapse state and the window's placement
- `internal/histories/` for the system hosts history and the post-apply command

A copy of a v5 data directory from an earlier SwitchHosts release works as it
is.

## Applying hosts

`internal/hostswrite` writes the system hosts file:

- the switched-on local and remote entries are joined with a blank line, and
  duplicate domains are dropped when the preference asks for it;
- `append` mode keeps the file outside the
  `# --- SWITCHHOSTS_CONTENT_START ---` / `..._END ---` markers untouched;
- a write that the user cannot make asks for the platform's own elevation: UAC
  through `ShellExecuteExW` on Windows, `osascript` on macOS and `pkexec` (or
  `sudo -n`) on Linux;
- every change is recorded in the history, and the post-apply command runs as
  the signed-in user.

At startup the app checks whether it can write the hosts file. Without that
right it says so, and offers to restart as administrator: saving or switching
an entry would otherwise do nothing.

An apply whose metadata could not be saved leaves a recovery state: the window
shows a notice until the selection is reapplied.

## Packaging

### Windows

```bash
go run github.com/egoist/mygo/cmd/mygo@v0.6.0 build -platform windows/amd64
go run ./cmd/package-windows
```

The installer is `build/installers/SwitchHosts-<version>-Setup-x64.exe`:

- it installs to `C:\Program Files\SwitchHosts` (the directory page still
  lets the user change that), for every account of the machine, and asks for
  administrator rights;
- its file name has no spaces, so scripts handle it without quoting;
- it adds Start menu entries and a desktop shortcut, writes its uninstall
  entry to the 64-bit view of the registry so Settings > Apps lists it, and
  leaves `~/.SwitchHosts` alone when uninstalling.

The packaging step needs NSIS (`makensis`), which `mygo build` downloads on
Windows when it is missing. Quit the app before uninstalling: Windows keeps a
running executable open, and the uninstaller then leaves `SwitchHosts.exe`
behind.

## Developing

```bash
go run .          # start the app
go test ./...     # unit tests, including headless renders of the window
go vet ./...
```

The tests draw frames with `ui.Render`, so a change to the view can be checked
by writing a PNG and looking at it. `SWH_SNAPSHOT_DIR=<dir> go test
./internal/appui` keeps those images.

`SWH_CAPTURE=<path> SWH_CAPTURE_QUIT=1 go run .` captures the real window: the
Windows compositor does not let a screen capture see a GPU-drawn window, so
the app captures its own frame instead.

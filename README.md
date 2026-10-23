<div align="center" markdown="1">
  <sup>Special thanks to:</sup>
  <br>
  <a href="https://go.warp.dev/SwitchHosts">
    <img alt="Warp sponsorship" width="400" src="https://github.com/user-attachments/assets/bb4a0222-12bf-4c79-bb80-a8ed4672b801" />
  </a>

### [Warp, the intelligent terminal for developers](https://go.warp.dev/SwitchHosts)

[Available for MacOS, Linux, & Windows](https://go.warp.dev/SwitchHosts)<br>

</div>

---

# SwitchHosts

Homepage: [https://switchhosts.app](https://switchhosts.app)

SwitchHosts manages the system hosts file. It is written in Go and draws its
whole interface with [MyGo](https://github.com/egoist/mygo)'s native UI: the
window, lists, editor, dialogs and tray icon are Go code, with no WebView and
no JavaScript runtime.

## Features

**Entries**

- Local, remote, group and folder entries, alongside the system hosts
- Folders nest and collapse; folders can cascade their switches to children,
  and pick single- or multiple-choice behaviour
- Trashcan: move entries there, restore them, or delete them for good

**Editing**

- Code editor with hosts syntax highlighting, line numbers and selection
- Changes are saved shortly after typing pauses, and an edit to a switched-on
  entry is written to the system hosts file
- Find across every entry, with regular expression and case options, plus
  replace-all
- Read-only view for the system hosts, the remote copies and groups

**Remote entries**

- Fetch by URL, or build rules from a list of domains
- Refresh on demand, on a per-entry interval, or at startup
- Proxy support for the fetches

**Applying**

- `append` mode keeps whatever other tools wrote outside the managed
  `# --- SWITCHHOSTS_CONTENT_START ---` / `..._END ---` section, `overwrite`
  replaces the file
- Optional removal of duplicate domain mappings
- A post-apply command runs after every write
- Every change is recorded, and can be restored from the history
- Missing administrator rights are detected at startup, with an offer to
  restart elevated

**Interface**

- 16 languages, light, dark and follow-system themes
- Collapsible side panels with remembered widths
- Window position, size and maximized state persist across sessions
- Closing the window asks whether to hide to the tray, minimize, or quit
- Tray icon with a menu, and an application menu with the same actions

## Install

Download the installer from the
[GitHub releases](https://github.com/oldj/SwitchHosts/releases) page, or build
it yourself (see [Build](#build)).

The Windows installer puts the app in `Program Files\SwitchHosts` for every
account of the machine, and its file name has no spaces.

## Data

SwitchHosts stores its data in `~/.SwitchHosts` (the `.SwitchHosts` folder in
the user's home directory on Windows). The v5 layout:

- `manifest.json` holds the hosts tree
- `entries/` holds the content of local and remote hosts
- `trashcan.json` holds the trashed entries
- `internal/config.json` holds the preferences
- `internal/state.json` holds the collapse state and the window's placement
- `internal/histories/` holds the system hosts history and the runs of the
  post-apply command

A v5 data directory from an earlier SwitchHosts release works as it is. For a
complete manual backup, copy the whole `~/.SwitchHosts` folder.

The system hosts file is written as UTF-8 without a byte order mark. Before
converting an existing ANSI or BOM-marked UTF-16 Windows file, the original
bytes are saved in `internal/histories/hosts-encoding-backups/`.

## Build

The app needs [Go](https://go.dev/dl/) 1.27 or later. `go.mod` depends on the
released `github.com/egoist/mygo` module, so a clone builds without any other
checkout.

```bash
go run .                        # start the app
go test ./...                   # unit tests, including headless renders
go build -o SwitchHosts.exe .   # the app's executable
```

### Windows installer

Build the app with MyGo first, which embeds the icon and the version
information, then pack it into an installer:

```bash
go run github.com/egoist/mygo/cmd/mygo@v0.6.0 build -platform windows/amd64
go run ./cmd/package-windows
```

The result is `build/installers/SwitchHosts-<version>-Setup-x64.exe`. It
installs to `Program Files\SwitchHosts` (the directory page still lets the
user change that), adds Start menu entries and a desktop shortcut, and writes
its uninstall entry to the 64-bit view of the registry so Settings > Apps
lists it. Uninstalling leaves `~/.SwitchHosts` alone.

The packaging step needs NSIS (`makensis`); MyGo downloads it on Windows when
it is missing. Quit the app before uninstalling: Windows keeps a running
executable open, and the uninstaller would then leave `SwitchHosts.exe`
behind.

## Project layout

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

See [README.native.md](./README.native.md) for the installer details and how
the interface is tested.

## Copyright

SwitchHosts is a free and open source software, it is released under the [Apache License](./LICENSE).

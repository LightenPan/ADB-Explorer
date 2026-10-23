// Package store owns every read and write of SwitchHosts' data directory:
// the hosts tree, its content files, the trashcan, preferences and the
// per-machine UI state.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Paths are the files of the data layout. The v5 layout lives under
// ~/.SwitchHosts, or under the folder the user chose:
//
//	manifest.json          the hosts tree
//	entries/<id>.hosts     the content of local and remote hosts
//	trashcan.json          entries moved to the trashcan
//	internal/config.json   preferences
//	internal/state.json    folder collapse state and window geometry
//	internal/histories/    system hosts and post-apply command history
type Paths struct {
	Root         string
	ManifestFile string
	TrashcanFile string
	EntriesDir   string
	Internal     string
	ConfigFile   string
	StateFile    string
	HistoriesDir string
}

// Under builds the layout rooted at root.
func Under(root string) Paths {
	internal := filepath.Join(root, "internal")
	return Paths{
		Root:         root,
		ManifestFile: filepath.Join(root, "manifest.json"),
		TrashcanFile: filepath.Join(root, "trashcan.json"),
		EntriesDir:   filepath.Join(root, "entries"),
		Internal:     internal,
		ConfigFile:   filepath.Join(internal, "config.json"),
		StateFile:    filepath.Join(internal, "state.json"),
		HistoriesDir: filepath.Join(internal, "histories"),
	}
}

// EnsureDirs creates every directory of the layout.
func (p Paths) EnsureDirs() error {
	for _, dir := range []string{p.Root, p.EntriesDir, p.Internal, p.HistoriesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// CleanupTmpFiles removes leftovers of interrupted atomic writes.
func (p Paths) CleanupTmpFiles() {
	for _, dir := range []string{p.Root, p.EntriesDir, p.Internal, p.HistoriesDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".tmp") {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
}

// DefaultRoot is ~/.SwitchHosts.
func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".SwitchHosts"), nil
}

// DataDirName is the folder a chosen parent folder gets.
const DataDirName = "SwitchHosts.data"

// ResolveTarget maps a folder the user picked to the data directory it
// means: the default root means a reset, a folder already named
// SwitchHosts.data is used as is, and any other folder gets one.
func ResolveTarget(picked, defaultRoot string) (string, bool) {
	if samePath(picked, defaultRoot) {
		return defaultRoot, true
	}
	if filepath.Base(picked) == DataDirName {
		return picked, false
	}
	return filepath.Join(picked, DataDirName), false
}

func samePath(a, b string) bool {
	ca, errA := filepath.Abs(a)
	cb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}

// Recovery says why the recorded data directory is unusable. The app
// starts on the default root and asks the user to choose a location.
type Recovery struct {
	// Kind is "missing" or "invalid".
	Kind string
	// Path is the recorded directory, for "missing".
	Path string
}

// PointerFile is where the chosen data directory is recorded, outside the
// data directory so it can be read before the layout is known.
func PointerFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "net.oldj.switchhosts", "data_dir.json"), nil
}

// ErrNoPointer means no custom data directory is recorded.
var ErrNoPointer = errors.New("no data directory pointer")

// ReadPointer returns the recorded custom data directory, a Recovery when
// the pointer exists but cannot be used, or ErrNoPointer.
func ReadPointer() (string, *Recovery, error) {
	path, err := PointerFile()
	if err != nil {
		return "", nil, ErrNoPointer
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, ErrNoPointer
		}
		return "", &Recovery{Kind: "invalid"}, nil
	}
	if info.IsDir() {
		return "", &Recovery{Kind: "invalid"}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", &Recovery{Kind: "invalid"}, nil
	}
	var envelope struct {
		DataDir string `json:"data_dir"`
	}
	if err := unmarshalJSON(data, &envelope); err != nil || strings.TrimSpace(envelope.DataDir) == "" {
		return "", &Recovery{Kind: "invalid"}, nil
	}
	if !filepath.IsAbs(envelope.DataDir) {
		return "", &Recovery{Kind: "invalid"}, nil
	}
	return envelope.DataDir, nil, nil
}

// PointerExists reports whether a pointer entry is on disk.
func PointerExists() bool {
	path, err := PointerFile()
	if err != nil {
		return false
	}
	_, err = os.Lstat(path)
	return err == nil
}

// WritePointer records root as the custom data directory.
func WritePointer(root string) error {
	path, err := PointerFile()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("refusing to record a relative data directory: %s", root)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// A directory sitting where the pointer belongs cannot be renamed over.
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		_ = os.RemoveAll(path)
	}
	body, err := marshalIndentJSON(map[string]any{
		"format":        "switchhosts-data-dir-pointer",
		"schemaVersion": 1,
		"data_dir":      root,
	})
	if err != nil {
		return err
	}
	return AtomicWrite(path, body)
}

// ClearPointer removes the pointer, so the app uses the default root.
func ClearPointer() error {
	path, err := PointerFile()
	if err != nil {
		return err
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		_ = os.RemoveAll(path)
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ResolveRoot picks the active data directory and reports a recovery when
// the recorded one cannot be used.
func ResolveRoot(defaultRoot string) (Paths, *Recovery) {
	recorded, recovery, _ := ReadPointer()
	switch {
	case recovery != nil:
		return Under(defaultRoot), recovery
	case recorded == "":
		return Under(defaultRoot), nil
	}
	info, err := os.Stat(recorded)
	if err != nil || !info.IsDir() {
		return Under(defaultRoot), &Recovery{Kind: "missing", Path: recorded}
	}
	return Under(recorded), nil
}

// EnsureUsable verifies the layout exists and is writable.
func (p Paths) EnsureUsable() error {
	if err := p.EnsureDirs(); err != nil {
		return err
	}
	probe := filepath.Join(p.Internal, ".write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return fmt.Errorf("%s is not writable: %w", p.Internal, err)
	}
	_ = os.Remove(probe)
	return nil
}

// AtomicWrite writes contents to dest through a sibling temp file, so a
// crash leaves either the old file or the new one.
func AtomicWrite(dest string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, contents, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

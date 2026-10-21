package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// settings is the JSON file next to the app's user data.
type settings struct {
	Lang            string     `json:"lang"`
	Split           float32    `json:"split"`
	NavOpen         bool       `json:"navOpen"`
	ShowHidden      bool       `json:"showHidden"`
	PollSeconds     int        `json:"pollSeconds"`
	DefaultPath     string     `json:"defaultPath"`
	MaxPreviewKB    int        `json:"maxPreviewKB"`
	PreviewPDF      *bool      `json:"previewPDF"`
	Locations       []location `json:"locations"`
	LastUploadDir   string     `json:"lastUploadDir,omitempty"`
	LastDownloadDir string     `json:"lastDownloadDir,omitempty"`
}

type location struct {
	Path  string `json:"path"`
	Label string `json:"label"`
}

// current holds settings. The app struct is owned elsewhere, so this stays
// package-level.
var current settings

func defaultSettings() settings {
	return settings{
		Lang:         "zh",
		Split:        560,
		NavOpen:      true,
		ShowHidden:   false,
		PollSeconds:  3,
		DefaultPath:  "/sdcard",
		MaxPreviewKB: 300,
		PreviewPDF:   boolPtr(true),
		Locations: []location{
			{Path: "/sdcard", Label: "内部存储"},
			{Path: "/sdcard/Download", Label: "下载"},
			{Path: "/sdcard/DCIM", Label: "相机"},
			{Path: "/", Label: "根目录"},
			{Path: "/data/local/tmp", Label: "临时目录"},
		},
	}
}

func settingsPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func (a *app) loadSettings() {
	s := defaultSettings()
	if path, err := settingsPath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			var got settings
			if json.Unmarshal(data, &got) == nil {
				s = got
			}
		}
	}
	if s.Lang == "" {
		s.Lang = "zh"
	}
	a.lang = s.Lang
	if s.Split > 100 {
		a.split = s.Split
	}
	a.navOpen = s.NavOpen
	if s.MaxPreviewKB == 0 {
		s.MaxPreviewKB = 300
	}
	if s.PreviewPDF == nil {
		s.PreviewPDF = boolPtr(true)
	}
	current = s
	if a.tabs != nil && s.DefaultPath != "" && len(a.tabs) > 0 && len(a.tabs[0].panes) > 0 && a.tabs[0].panes[0].path == "/sdcard" {
		a.tabs[0].panes[0].path = cleanPath(s.DefaultPath)
	}
}

func (a *app) saveSettings() {
	current.Lang = a.lang
	current.Split = a.split
	current.NavOpen = a.navOpen
	path, err := settingsPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, append(data, '\n'), 0o644)
}

func (a *app) locations() []location {
	if len(current.Locations) == 0 {
		return defaultSettings().Locations
	}
	return current.Locations
}

func (a *app) setShowHidden(v bool) {
	current.ShowHidden = v
	a.saveSettings()
}

func (a *app) showHidden() bool { return current.ShowHidden }

func (a *app) setPoll(seconds int) {
	if seconds < 0 {
		seconds = 0
	}
	if seconds > 60 {
		seconds = 60
	}
	current.PollSeconds = seconds
	a.saveSettings()
}

func (a *app) pollSeconds() int { return current.PollSeconds }

func (a *app) setDefaultPath(p string) {
	current.DefaultPath = cleanPath(p)
	a.saveSettings()
}

// lastWindowsDir is the directory the upload or download dialog opens in.
// An empty string leaves the choice to Windows. A saved directory that is
// gone is ignored so the dialog does not start in a fixed folder.
func (a *app) lastWindowsDir(upload bool) string {
	dir := current.LastDownloadDir
	if upload {
		dir = current.LastUploadDir
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return ""
	}
	return dir
}

// rememberWindowsDir keeps the directory of a path the user just chose.
func (a *app) rememberWindowsDir(upload bool, path string) {
	dir := directoryOf(path)
	if dir == "" {
		return
	}
	if upload {
		current.LastUploadDir = dir
	} else {
		current.LastDownloadDir = dir
	}
	if a.win != nil {
		a.saveSettings()
	}
}

func directoryOf(path string) string {
	path = filepath.Clean(path)
	if path == "" || path == "." {
		return ""
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return path
	}
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return ""
	}
	return dir
}

func boolPtr(v bool) *bool { return &v }

func (a *app) maxPreviewBytes() int {
	kb := current.MaxPreviewKB
	if kb == 0 {
		kb = 300
	}
	if kb < 16 {
		kb = 16
	}
	if kb > 10240 {
		kb = 10240
	}
	return kb * 1024
}

func (a *app) previewPDFEnabled() bool {
	if current.PreviewPDF == nil {
		return true
	}
	return *current.PreviewPDF
}

func (a *app) setMaxPreviewKB(kb int) {
	if kb == 0 {
		kb = 300
	}
	if kb < 16 {
		kb = 16
	}
	if kb > 10240 {
		kb = 10240
	}
	current.MaxPreviewKB = kb
	a.saveSettings()
}

func (a *app) setPreviewPDF(v bool) {
	current.PreviewPDF = boolPtr(v)
	a.saveSettings()
}

// applyLocation adds or removes a bookmark. An empty path is ignored, and a
// path already in the list is not added again.
func applyLocation(list []location, path, label string, remove bool) []location {
	if path == "" {
		return list
	}
	if remove {
		out := make([]location, 0, len(list))
		for _, loc := range list {
			if loc.Path != path {
				out = append(out, loc)
			}
		}
		return out
	}
	for _, loc := range list {
		if loc.Path == path {
			return list
		}
	}
	return append(list, location{Path: path, Label: label})
}

func (a *app) addLocation(path, label string) {
	current.Locations = applyLocation(current.Locations, path, label, false)
	a.saveSettings()
}

func (a *app) removeLocation(path string) {
	current.Locations = applyLocation(current.Locations, path, "", true)
	a.saveSettings()
}

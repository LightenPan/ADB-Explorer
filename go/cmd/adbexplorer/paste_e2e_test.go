package main

import (
	"os"
	"path/filepath"
	"testing"

	"adbexplorer/internal/adb"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Pasting files copied in Explorer must land in the focused device folder.
// The headless tester has no system clipboard, so the test feeds the paths
// the paste action would have read.
func TestPasteFilesIntoDeviceFolder(t *testing.T) {
	dir := t.TempDir()
	note := filepath.Join(dir, "from-clipboard.txt")
	if err := os.WriteFile(note, []byte("pasted"), 0o644); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, "a-folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newApp()
	a.serial = "abc"
	a.devices = []adb.Device{{Serial: "abc", State: "device", Model: "Pixel_7"}}
	a.focus().path = "/sdcard/Download"
	a.focus().entries = []adb.Entry{{Name: "already.txt", Mode: 0o100644, Size: 4}}
	clipboardFiles = func() ([]string, error) { return []string{note, folder}, nil }
	t.Cleanup(func() {
		clipboardFiles = func() ([]string, error) { return mygo.Clipboard.ReadFiles() }
		conflicts = nil
	})

	tt := ui.NewTester(a.view, 1400, 720)
	if err := tt.Click("粘贴"); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, item := range a.ops {
		if item.kind == "upload" {
			got[item.label] = true
		}
	}
	if !got["from-clipboard.txt"] || !got["a-folder"] {
		t.Fatalf("ops: %+v", a.ops)
	}
	if err := tt.Click("传输"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("from-clipboard.txt") || !tt.HasText("a-folder") {
		t.Fatalf("transfers: %q", tt.Texts())
	}
	saveE2E(t, tt, "e2e-paste.png")

	// A name that is already in the folder asks before replacing.
	existing := filepath.Join(dir, "already.txt")
	if err := os.WriteFile(existing, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	clipboardFiles = func() ([]string, error) { return []string{existing}, nil }
	if err := tt.Click("文件"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyV)
	if !tt.HasText("already.txt 已存在") {
		t.Fatalf("conflict prompt missing, texts: %q", tt.Texts())
	}
	saveE2E(t, tt, "e2e-paste-conflict.png")
}

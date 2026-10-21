package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"adbexplorer/internal/adb"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func saveE2E(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := "testdata"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, tt.Image())
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("missing e2e artifact %s: %v", path, err)
	}
	if info.Size() < 1000 {
		t.Fatalf("%s is %d bytes, want at least 1000", path, info.Size())
	}
}

func TestExplorerRendersWithoutADevice(t *testing.T) {
	a := newApp()
	a.connErr = "adb server unavailable"
	tt := ui.NewTester(a.view, 1400, 720)
	if !tt.HasText("ADB Explorer") || !tt.HasText("连接设备") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if err := tt.Click("设备"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageDevices || !tt.HasText("adb server unavailable") {
		t.Fatalf("page %q texts %q", a.page, tt.Texts())
	}
	saveE2E(t, tt, "e2e-no-device.png")
}

func TestBrowsingAFolder(t *testing.T) {
	a := newApp()
	a.serial = "abc"
	a.devices = []adb.Device{{Serial: "abc", State: "device", Model: "Pixel_7"}}
	a.focus().entries = []adb.Entry{
		{Name: "DCIM", Mode: 0o040755},
		{Name: "notes.txt", Mode: 0o100644, Size: 2048},
	}
	tt := ui.NewTester(a.view, 1400, 720)
	if !tt.HasText("DCIM") || !tt.HasText("2.0 KB") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if err := tt.Click("notes.txt"); err != nil {
		t.Fatal(err)
	}
	if len(a.selected(a.visible())) != 1 {
		t.Fatalf("selection: %+v picked %d", a.focus().chosen, a.focus().picked)
	}
	saveE2E(t, tt, "e2e-browse.png")
}

func TestUploadFileAndFolderFromTheButtons(t *testing.T) {
	dir := t.TempDir()
	note := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(note, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, "Album")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "img.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := openUploadDialog
	t.Cleanup(func() { openUploadDialog = prev })
	openUploadDialog = func(opts mygo.OpenDialogOptions) ([]string, error) {
		if opts.Directory {
			return []string{folder}, nil
		}
		return []string{note}, nil
	}
	a := newApp()
	a.serial = "abc"
	a.devices = []adb.Device{{Serial: "abc", State: "device", Model: "Pixel_7"}}
	a.focus().path = "/sdcard/Download"
	tt := ui.NewTester(a.view, 1400, 720)
	if err := tt.Click("上传"); err != nil {
		t.Fatal(err)
	}
	if a.status != "正在上传 note.txt" {
		t.Fatalf("file upload status %q ops %+v", a.status, a.ops)
	}
	if err := tt.Click("上传文件夹"); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, item := range a.ops {
		if item.kind == "upload" {
			got[item.label] = true
		}
	}
	if !got["note.txt"] || !got["Album"] || a.status != "正在上传 Album" {
		t.Fatalf("status %q ops %+v", a.status, a.ops)
	}
	if err := tt.Click("传输"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("note.txt") || !tt.HasText("Album") {
		t.Fatalf("transfers: %q", tt.Texts())
	}
	saveE2E(t, tt, "e2e-upload.png")
}

func TestDialogsReopenTheLastWindowsDirectory(t *testing.T) {
	prev := current
	current = defaultSettings()
	current.Locations = []location{{Label: "内部存储", Path: "/sdcard"}}
	t.Cleanup(func() { current = prev })
	dir := t.TempDir()
	note := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(note, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	save := filepath.Join(dir, "saved")
	if err := os.Mkdir(save, 0o755); err != nil {
		t.Fatal(err)
	}
	var uploads, downloads []string
	prevUp := openUploadDialog
	prevDown := openDownloadDialog
	t.Cleanup(func() {
		openUploadDialog = prevUp
		openDownloadDialog = prevDown
	})
	openUploadDialog = func(opts mygo.OpenDialogOptions) ([]string, error) {
		uploads = append(uploads, opts.DefaultPath)
		return []string{note}, nil
	}
	openDownloadDialog = func(opts mygo.OpenDialogOptions) ([]string, error) {
		downloads = append(downloads, opts.DefaultPath)
		return []string{save}, nil
	}
	a := newApp()
	a.serial = "abc"
	a.devices = []adb.Device{{Serial: "abc", State: "device", Model: "Pixel_7"}}
	a.focus().entries = []adb.Entry{{Name: "notes.txt", Mode: 0o100644, Size: 4}}
	a.focus().chosen.Add("notes.txt")
	tt := ui.NewTester(a.view, 1400, 720)
	if err := tt.Click("上传"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("上传"); err != nil {
		t.Fatal(err)
	}
	if len(uploads) != 2 || uploads[0] != "" || uploads[1] != dir {
		t.Fatalf("upload dialogs opened at %q, want \"\" then %s", uploads, dir)
	}
	if err := tt.Click("下载"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("下载"); err != nil {
		t.Fatal(err)
	}
	if len(downloads) != 2 || downloads[0] != "" || downloads[1] != save {
		t.Fatalf("download dialogs opened at %q, want \"\" then %s", downloads, save)
	}
	for _, p := range append(append([]string{}, uploads...), downloads...) {
		if strings.HasSuffix(strings.ToLower(p), strings.ToLower(string(filepath.Separator)+"adb explorer")) {
			t.Fatalf("dialog used the fixed folder %s", p)
		}
	}
	saveE2E(t, tt, "e2e-dialog-dir.png")
}

func TestRedownloadAsksSkipOverwriteOrRename(t *testing.T) {
	prev := current
	current = defaultSettings()
	current.Locations = []location{{Label: "内部存储", Path: "/sdcard"}}
	t.Cleanup(func() { current = prev })
	dest := t.TempDir()
	local := filepath.Join(dest, "notes.txt")
	if err := os.WriteFile(local, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	prevDown := openDownloadDialog
	prevPull := pullRemote
	t.Cleanup(func() {
		openDownloadDialog = prevDown
		pullRemote = prevPull
	})
	openDownloadDialog = func(opts mygo.OpenDialogOptions) ([]string, error) {
		return []string{dest}, nil
	}
	var pulls int
	pullRemote = func(_ *app, _ context.Context, _, _ string, w io.Writer, onBytes func(int64)) (int64, error) {
		pulls++
		body := []byte("fresh")
		if _, err := w.Write(body); err != nil {
			return 0, err
		}
		if onBytes != nil {
			onBytes(int64(len(body)))
		}
		return int64(len(body)), nil
	}
	a := newApp()
	a.serial = "abc"
	a.devices = []adb.Device{{Serial: "abc", State: "device", Model: "Pixel_7"}}
	a.focus().entries = []adb.Entry{{Name: "notes.txt", Mode: 0o100644, Size: 3}}
	a.focus().chosen.Add("notes.txt")
	tt := ui.NewTester(a.view, 1400, 720)
	if err := tt.Click("下载"); err != nil {
		t.Fatal(err)
	}
	waitDownloadPrompt(t, a, tt)
	for _, s := range []string{"notes.txt 已存在", "跳过", "覆盖", "重命名", "另存为 notes (1).txt"} {
		if !tt.HasText(s) {
			t.Fatalf("missing %q in %q", s, tt.Texts())
		}
	}
	if tt.HasText("后面的文件也按这次的选择处理") {
		t.Fatal("one file offered to apply the choice to others")
	}
	saveE2E(t, tt, "e2e-download-conflict.png")
	if err := tt.Click("跳过"); err != nil {
		t.Fatal(err)
	}
	waitDownloads(t, a, tt, 1)
	if pulls != 0 {
		t.Fatalf("skip still downloaded, pulls=%d", pulls)
	}
	if b, _ := os.ReadFile(local); string(b) != "old" {
		t.Fatalf("skip changed the file to %q", b)
	}

	if err := tt.Click("下载"); err != nil {
		t.Fatal(err)
	}
	waitDownloadPrompt(t, a, tt)
	if err := tt.Click("覆盖"); err != nil {
		t.Fatal(err)
	}
	waitDownloads(t, a, tt, 2)
	if b, _ := os.ReadFile(local); string(b) != "fresh" {
		t.Fatalf("overwrite left %q", b)
	}
	if _, err := os.Stat(local + ".part"); !os.IsNotExist(err) {
		t.Fatal("part file left behind")
	}

	if err := os.WriteFile(local, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("下载"); err != nil {
		t.Fatal(err)
	}
	waitDownloadPrompt(t, a, tt)
	if err := tt.Click("重命名文件"); err != nil {
		t.Fatal(err)
	}
	waitDownloads(t, a, tt, 3)
	if b, _ := os.ReadFile(local); string(b) != "old" {
		t.Fatalf("rename overwrote the original with %q", b)
	}
	renamed := filepath.Join(dest, "notes (1).txt")
	if b, err := os.ReadFile(renamed); err != nil || string(b) != "fresh" {
		t.Fatalf("renamed file: %q %v", b, err)
	}
}

func TestBatchDownloadUsesTheFirstChoice(t *testing.T) {
	prev := current
	current = defaultSettings()
	current.Locations = []location{{Label: "内部存储", Path: "/sdcard"}}
	t.Cleanup(func() { current = prev })
	dest := t.TempDir()
	aLocal := filepath.Join(dest, "a.txt")
	bLocal := filepath.Join(dest, "b.txt")
	for _, p := range []string{aLocal, bLocal} {
		if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prevDown := openDownloadDialog
	prevPull := pullRemote
	t.Cleanup(func() {
		openDownloadDialog = prevDown
		pullRemote = prevPull
	})
	openDownloadDialog = func(opts mygo.OpenDialogOptions) ([]string, error) {
		return []string{dest}, nil
	}
	var pulls int
	pullRemote = func(_ *app, _ context.Context, _, _ string, w io.Writer, onBytes func(int64)) (int64, error) {
		pulls++
		body := []byte("fresh")
		if _, err := w.Write(body); err != nil {
			return 0, err
		}
		if onBytes != nil {
			onBytes(int64(len(body)))
		}
		return int64(len(body)), nil
	}
	a := newApp()
	a.serial = "abc"
	a.devices = []adb.Device{{Serial: "abc", State: "device", Model: "Pixel_7"}}
	a.focus().entries = []adb.Entry{
		{Name: "a.txt", Mode: 0o100644, Size: 3},
		{Name: "b.txt", Mode: 0o100644, Size: 3},
	}
	a.focus().chosen.Add("a.txt")
	a.focus().chosen.Add("b.txt")
	tt := ui.NewTester(a.view, 1400, 720)
	if err := tt.Click("下载"); err != nil {
		t.Fatal(err)
	}
	waitDownloadPrompt(t, a, tt)
	if !tt.HasText("后面的文件也按这次的选择处理") || !tt.HasText("a.txt 已存在") {
		t.Fatalf("batch prompt: %q", tt.Texts())
	}
	saveE2E(t, tt, "e2e-download-batch.png")
	if err := tt.Click("跳过"); err != nil {
		t.Fatal(err)
	}
	waitDownloads(t, a, tt, 1)
	if pulls != 0 {
		t.Fatalf("skip still downloaded, pulls=%d", pulls)
	}
	for _, p := range []string{aLocal, bLocal} {
		if b, _ := os.ReadFile(p); string(b) != "old" {
			t.Fatalf("skip changed %s to %q", p, b)
		}
	}

	if err := tt.Click("下载"); err != nil {
		t.Fatal(err)
	}
	waitDownloadPrompt(t, a, tt)
	if err := tt.Click("覆盖"); err != nil {
		t.Fatal(err)
	}
	waitDownloads(t, a, tt, 2)
	if pulls != 2 {
		t.Fatalf("overwrite pulls=%d", pulls)
	}
	for _, p := range []string{aLocal, bLocal} {
		if b, _ := os.ReadFile(p); string(b) != "fresh" {
			t.Fatalf("overwrite left %s as %q", p, b)
		}
	}

	for _, p := range []string{aLocal, bLocal} {
		if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click("下载"); err != nil {
		t.Fatal(err)
	}
	waitDownloadPrompt(t, a, tt)
	if err := tt.Click("重命名文件"); err != nil {
		t.Fatal(err)
	}
	waitDownloads(t, a, tt, 3)
	for _, p := range []string{aLocal, bLocal} {
		if b, _ := os.ReadFile(p); string(b) != "old" {
			t.Fatalf("rename overwrote %s with %q", p, b)
		}
	}
	for _, name := range []string{"a (1).txt", "b (1).txt"} {
		b, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || string(b) != "fresh" {
			t.Fatalf("%s: %q %v", name, b, err)
		}
	}
}

func waitDownloadPrompt(t *testing.T, a *app, tt *ui.Tester) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if a.downloadAsk != nil && tt.HasText("已存在") {
			tt.Frame()
			return
		}
		time.Sleep(15 * time.Millisecond)
		tt.Frame()
	}
	t.Fatalf("no download prompt, texts %q", tt.Texts())
}

func waitDownloads(t *testing.T, a *app, tt *ui.Tester, n int) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		done := 0
		for _, item := range a.ops {
			if item.kind == "download" && (item.state == opDone || item.state == opFailed) {
				done++
			}
		}
		if done >= n && a.downloadAsk == nil {
			tt.Frame()
			return
		}
		time.Sleep(15 * time.Millisecond)
		tt.Frame()
	}
	t.Fatalf("downloads not finished, ops=%d ask=%v", len(a.ops), a.downloadAsk != nil)
}

func TestDownloadSelectedDirectoryQueuesATransfer(t *testing.T) {
	prev := current
	current.Locations = []location{{Label: "内部存储", Path: "/sdcard"}}
	t.Cleanup(func() { current = prev })
	dest := t.TempDir()
	prevDialog := openDownloadDialog
	t.Cleanup(func() { openDownloadDialog = prevDialog })
	openDownloadDialog = func(opts mygo.OpenDialogOptions) ([]string, error) {
		if !opts.Directory {
			t.Errorf("download picker is not a directory dialog")
		}
		return []string{dest}, nil
	}
	a := newApp()
	a.serial = "abc"
	a.devices = []adb.Device{{Serial: "abc", State: "device", Model: "Pixel_7"}}
	a.focus().entries = []adb.Entry{
		{Name: "DCIM", Mode: 0o040755},
		{Name: "notes.txt", Mode: 0o100644, Size: 2048},
	}
	tt := ui.NewTester(a.view, 1400, 720)
	if err := tt.Click("DCIM"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("下载"); err != nil {
		t.Fatal(err)
	}
	if len(a.ops) != 1 || a.ops[0].kind != "download" || a.ops[0].label != "DCIM" {
		t.Fatalf("folder download did nothing: ops=%+v status=%q", a.ops, a.status)
	}
	if err := tt.Click("传输"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("DCIM") {
		t.Fatalf("transfer list has no folder: %q", tt.Texts())
	}
	saveE2E(t, tt, "e2e-download-dir.png")
}

// Several files used to share one bar counted from the directory listing,
// with no file name and no speed. The bar has to follow bytes actually
// copied, and the transfer has to name the file copying now.
func TestDownloadSeveralFilesShowsProgressSpeedAndNames(t *testing.T) {
	prev := current
	current.Locations = []location{{Label: "内部存储", Path: "/sdcard"}}
	t.Cleanup(func() { current = prev })
	dest := t.TempDir()
	prevDialog := openDownloadDialog
	t.Cleanup(func() { openDownloadDialog = prevDialog })
	openDownloadDialog = func(opts mygo.OpenDialogOptions) ([]string, error) {
		return []string{dest}, nil
	}
	prevPull := pullRemote
	release := make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() {
		free()
		pullRemote = prevPull
	})
	pullRemote = func(_ *app, _ context.Context, _, remote string, _ io.Writer, onBytes func(int64)) (int64, error) {
		if strings.HasSuffix(remote, "b.bin") {
			if onBytes != nil {
				onBytes(200)
			}
			time.Sleep(350 * time.Millisecond)
			if onBytes != nil {
				onBytes(500)
			}
			<-release
			if onBytes != nil {
				onBytes(3000)
			}
			return 3000, nil
		}
		if onBytes != nil {
			onBytes(1000)
		}
		return 1000, nil
	}
	a := newApp()
	a.serial = "abc"
	a.devices = []adb.Device{{Serial: "abc", State: "device", Model: "Pixel_7"}}
	a.focus().entries = []adb.Entry{
		{Name: "a.bin", Mode: 0o100644, Size: 1000},
		{Name: "b.bin", Mode: 0o100644, Size: 3000},
	}
	a.focus().chosen.Add("a.bin")
	a.focus().chosen.Add("b.bin")
	tt := ui.NewTester(a.view, 1400, 720)
	if err := tt.Click("下载"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	var got *op
	for time.Now().Before(deadline) {
		for _, item := range a.ops {
			if item.kind == "download" && item.file == "b.bin" && item.done == 1500 && item.total == 4000 && item.index == 2 && item.count == 2 && item.speed > 0 {
				got = item
			}
		}
		if got != nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
		tt.Frame()
	}
	if got == nil {
		var dump string
		for _, item := range a.ops {
			dump += fmt.Sprintf("file=%s done=%d total=%d index=%d count=%d speed=%d names=%v; ", item.file, item.done, item.total, item.index, item.count, item.speed, item.names)
		}
		t.Fatalf("progress stayed wrong: %s status=%q", dump, a.status)
	}
	if len(got.names) != 2 || got.names[0] != "a.bin" || got.names[1] != "b.bin" {
		t.Fatalf("files: %v", got.names)
	}
	if err := tt.Click("传输"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, s := range []string{"a.bin", "b.bin", "2/2", "正在下载 b.bin", "/s"} {
		if !tt.HasText(s) {
			t.Fatalf("missing %q in %q", s, tt.Texts())
		}
	}
	saveE2E(t, tt, "e2e-download-files.png")
	free()
	waitDownloads(t, a, tt, 1)
}

// A long listing must scroll inside its pane. Its rows used to stretch the
// page column past the window, so file names were painted over the toolbar.
func TestLongListingStaysBelowTheToolbar(t *testing.T) {
	a := newApp()
	a.serial = "abc"
	a.devices = []adb.Device{{Serial: "abc", State: "device", Model: "Pixel_7"}}
	entries := make([]adb.Entry, 60)
	for i := range entries {
		entries[i] = adb.Entry{Name: fmt.Sprintf("file-%02d.txt", i), Mode: 0o100644, Size: 100}
	}
	a.focus().entries = entries
	const height = 720
	tt := ui.NewTester(a.view, 1400, height)
	bar, ok := tt.Find("ADB Explorer")
	if !ok {
		t.Fatal("toolbar title missing")
	}
	tab, ok := tt.Find("sdcard")
	if !ok {
		t.Fatal("tab missing")
	}
	up, ok := tt.Find("向上")
	if !ok {
		t.Fatal("up button missing")
	}
	if tab.Y+tab.H > up.Y {
		t.Fatalf("tab strip ending at %.0f overlaps the path row at %.0f", tab.Y+tab.H, up.Y)
	}
	for _, name := range []string{"file-00.txt", "file-59.txt"} {
		r, ok := tt.Find(name)
		if !ok {
			continue
		}
		if r.Y < bar.Y+bar.H {
			t.Fatalf("%s at y %.0f overlaps the toolbar ending at %.0f", name, r.Y, bar.Y+bar.H)
		}
		if r.Y >= height {
			t.Fatalf("%s at y %.0f is outside the %d tall window", name, r.Y, height)
		}
	}
	saveE2E(t, tt, "e2e-long-listing.png")
}

func previewApp(entries []adb.Entry, files map[string][]byte, shells map[string]string) *app {
	current = defaultSettings()
	a := newApp()
	a.serial = "abc"
	a.devices = []adb.Device{{Serial: "abc", State: "device", Model: "Pixel_7"}}
	a.focus().entries = entries
	a.previewLocal = files
	a.previewShell = shells
	return a
}

func waitPreview(t *testing.T, a *app, tt *ui.Tester) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if a.detailOpen && !a.detail.loading {
			tt.Frame()
			return
		}
		time.Sleep(15 * time.Millisecond)
		tt.Frame()
	}
	t.Fatalf("preview still loading: err=%q preview=%q kind=%q", a.detail.err, a.detail.preview, a.detail.kind)
}

func openDetails(t *testing.T, a *app, name string) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(a.view, 1400, 720)
	if err := tt.Click(name); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("详情"); err != nil {
		t.Fatal(err)
	}
	waitPreview(t, a, tt)
	return tt
}

func fileEntry(name string, data []byte) adb.Entry {
	return adb.Entry{Name: name, Mode: 0o100644, Size: uint32(len(data))}
}

func unzipListing(members ...string) string {
	var b bytes.Buffer
	b.WriteString("Archive:  archive\n")
	b.WriteString("  Length      Date    Time    Name\n")
	b.WriteString("---------  ---------- -----   ----\n")
	for _, name := range members {
		fmt.Fprintf(&b, "     128  2024-01-01 12:00   %s\n", name)
	}
	b.WriteString("---------                     -------\n")
	fmt.Fprintf(&b, "     %d                     %d files\n", 128*len(members), len(members))
	return b.String()
}

func onePagePDF() []byte {
	var b bytes.Buffer
	offsets := map[int]int{}
	obj := func(n int, body string) {
		offsets[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
	}
	c := "0 0 1 rg 10 10 180 80 re f"
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	obj(2, "<< /Type /Pages /Kids [10 0 R] /Count 1 /MediaBox [0 0 200 100] >>")
	obj(10, "<< /Type /Page /Parent 2 0 R /Contents 11 0 R >>")
	obj(11, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c))
	last := 11
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", last+1)
	for n := 1; n <= last; n++ {
		if off, ok := offsets[n]; ok {
			fmt.Fprintf(&b, "%010d 00000 n \n", off)
		} else {
			b.WriteString("0000000000 65535 f \n")
		}
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", last+1, xref)
	return b.Bytes()
}

func TestAppsPageShowsNameSizeAndKind(t *testing.T) {
	a := newApp()
	a.serial = "abc"
	a.devices = []adb.Device{{Serial: "abc", State: "device", Model: "Pixel_8_Pro"}}
	a.previewShell = map[string]string{
		"pm list packages -f": "" +
			"package:/data/app/~~ab=cd~~/com.tencent.mm-x==/base.apk=com.tencent.mm\n" +
			"package:/system/priv-app/Settings/Settings.apk=com.android.settings\n" +
			"package:/data/app/com.example.fieldnotes/base.apk=com.example.fieldnotes\n",
		"dumpsys-packages": "" +
			"Package [com.tencent.mm] (1):\n" +
			"  versionName=8.0.56\n" +
			"  lastUpdateTime=2026-10-12 09:14:00\n" +
			"  nonLocalizedLabel=微信\n" +
			"Package [com.android.settings] (2):\n" +
			"  versionName=15\n" +
			"  lastUpdateTime=2026-08-05 08:00:00\n" +
			"  nonLocalizedLabel=设置\n" +
			"  pkgFlags=[ SYSTEM HAS_CODE ]\n" +
			"Package [com.example.fieldnotes] (3):\n" +
			"  versionName=1.4.2\n" +
			"  nonLocalizedLabel=null\n",
		"pkg-stat": "" +
			"260046848|/data/app/~~ab=cd~~/com.tencent.mm-x==/base.apk\n" +
			"42991616|/system/priv-app/Settings/Settings.apk\n",
	}
	a.previewLocal = map[string][]byte{
		"/data/app/com.example.fieldnotes/base.apk": zipOf(t, map[string][]byte{
			"res/mipmap-xxhdpi/ic_launcher.png": pngBytes(t, 32),
		}),
	}
	a.loadPackages()
	tt := ui.NewTester(a.view, 1400, 720)
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"微信", "com.tencent.mm", "8.0.56", "248.0 MB", "2026-10-12", "设置", "com.example.fieldnotes"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	var icon bool
	for _, p := range a.apps {
		if p.ID == "com.tencent.mm" && (p.Path == "" || !strings.Contains(p.Path, "==/base.apk")) {
			t.Fatalf("path lost an equals sign: %q", p.Path)
		}
		if p.ID == "com.example.fieldnotes" && p.Icon != nil {
			icon = true
		}
		if p.ID == "com.android.settings" && !p.System {
			t.Fatal("settings was not marked system")
		}
	}
	if !icon {
		t.Fatal("apk icon was not decoded")
	}
	if err := tt.Click("用户应用"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("com.android.settings") {
		t.Fatal("system app stayed in the user filter")
	}
	if !tt.HasText("微信") || !tt.HasText("248.0 MB") {
		t.Fatalf("user app missing: %q", tt.Texts())
	}
	if err := tt.Click("微信"); err != nil {
		t.Fatal(err)
	}
	if a.appSel != "com.tencent.mm" || !tt.HasText("应用详情") || !tt.HasText("卸载") {
		t.Fatalf("detail sel %q texts %q", a.appSel, tt.Texts())
	}
	saveE2E(t, tt, "e2e-apps-mica.png")
}

func TestPreviewRaster(t *testing.T) {
	img := pngBytes(t, 64)
	a := previewApp(
		[]adb.Entry{fileEntry("photo.png", img)},
		map[string][]byte{"/sdcard/photo.png": img},
		nil,
	)
	tt := openDetails(t, a, "photo.png")
	if tt.HasText("Not a text file") {
		t.Fatalf("image shown as text: %q", tt.Texts())
	}
	if !tt.HasText("图片 ·") || !tt.HasText("photo.png") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if len(a.detail.bitmaps) == 0 {
		t.Fatal("no image in the pane")
	}
	bar, ok := tt.Find("ADB Explorer")
	if !ok {
		t.Fatal("toolbar missing")
	}
	if r, ok := tt.Find("photo.png"); ok && r.Y < bar.Y+bar.H {
		t.Fatalf("listing overlaps toolbar")
	}
	saveE2E(t, tt, "e2e-preview-image.png")
}

func TestPreviewText(t *testing.T) {
	body := []byte("{\n  \"device\": \"Pixel\",\n  \"build\": 14\n}\n")
	a := previewApp(
		[]adb.Entry{fileEntry("notes.json", body)},
		map[string][]byte{"/sdcard/notes.json": body},
		nil,
	)
	tt := openDetails(t, a, "notes.json")
	if a.detail.edit == nil {
		t.Fatalf("expected editor, err=%q preview=%q", a.detail.err, a.detail.preview)
	}
	if !tt.HasText("notes.json") || !tt.HasText("JSON") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	saveE2E(t, tt, "e2e-preview-text.png")
}

func TestPreviewHex(t *testing.T) {
	data := []byte{0x7f, 'E', 'L', 'F', 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	a := previewApp(
		[]adb.Entry{fileEntry("libfoo.so", data)},
		map[string][]byte{"/sdcard/libfoo.so": data},
		nil,
	)
	tt := openDetails(t, a, "libfoo.so")
	if !tt.HasText("二进制 · 前 4 KB") || !tt.HasText("7f 45 4c 46") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if tt.HasText("Not a text file") {
		t.Fatal("hex pane still says not a text file")
	}
	saveE2E(t, tt, "e2e-preview-hex.png")
}

func TestPreviewAPK(t *testing.T) {
	apk := zipOf(t, map[string][]byte{
		"res/mipmap-xxhdpi/ic_launcher.png": pngBytes(t, 32),
		"META-INF/MANIFEST.MF":              []byte("Manifest-Version: 1.0\nCreated-By: test\n"),
		"classes.dex":                       []byte("dex\x00"),
	})
	quoted := shellQuote("/sdcard/app.apk")
	a := previewApp(
		[]adb.Entry{fileEntry("app.apk", apk)},
		map[string][]byte{"/sdcard/app.apk": apk},
		map[string]string{
			"unzip -l " + quoted:                           unzipListing("res/mipmap-xxhdpi/ic_launcher.png", "META-INF/MANIFEST.MF", "classes.dex"),
			"unzip -p " + quoted + " META-INF/MANIFEST.MF": "Manifest-Version: 1.0\nCreated-By: test\n",
		},
	)
	tt := openDetails(t, a, "app.apk")
	if !tt.HasText("APK ·") || !tt.HasText("ic_launcher.png") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if len(a.detail.bitmaps) == 0 {
		t.Fatal("apk icon missing")
	}
	saveE2E(t, tt, "e2e-preview-apk.png")
}

func TestPreviewZipTOC(t *testing.T) {
	zipBytes := zipOf(t, map[string][]byte{"readme.txt": []byte("hi"), "assets/keep": []byte("x")})
	quoted := shellQuote("/sdcard/notes.zip")
	a := previewApp(
		[]adb.Entry{
			{Name: "DCIM", Mode: 0o040755},
			fileEntry("notes.zip", zipBytes),
		},
		map[string][]byte{"/sdcard/notes.zip": zipBytes},
		map[string]string{"unzip -l " + quoted: unzipListing("readme.txt", "assets/")},
	)
	tt := openDetails(t, a, "notes.zip")
	if a.focus().path != "/sdcard" {
		t.Fatalf("details entered the zip: %s", a.focus().path)
	}
	if !tt.HasText("压缩包 ·") || !tt.HasText("readme.txt") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if !tt.HasText("/sdcard") {
		t.Fatal("folder path left the listing")
	}
	saveE2E(t, tt, "e2e-preview-zip.png")
}

func TestPreviewSVG(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#3366cc" d="M3 3h18v18H3z"/></svg>`)
	a := previewApp(
		[]adb.Entry{fileEntry("mark.svg", svg)},
		map[string][]byte{"/sdcard/mark.svg": svg},
		nil,
	)
	tt := openDetails(t, a, "mark.svg")
	if a.detail.svg == nil {
		t.Fatalf("svg not parsed: err=%q preview=%q", a.detail.err, a.detail.preview)
	}
	if !tt.HasText("SVG · 仅路径") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if tt.HasText("<svg") {
		t.Fatal("raw xml shown instead of vector")
	}
	saveE2E(t, tt, "e2e-preview-svg.png")
}

func TestPreviewPDF(t *testing.T) {
	pdf := onePagePDF()
	a := previewApp(
		[]adb.Entry{fileEntry("page.pdf", pdf)},
		map[string][]byte{"/sdcard/page.pdf": pdf},
		nil,
	)
	tt := openDetails(t, a, "page.pdf")
	if tt.HasText("无法预览 PDF") {
		t.Fatalf("engine missing: %q", tt.Texts())
	}
	if !tt.HasText("PDF ·") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if len(a.detail.bitmaps) == 0 {
		t.Fatalf("no page bitmap: err=%q", a.detail.err)
	}
	saveE2E(t, tt, "e2e-preview-pdf.png")
}

func TestPreviewTooLarge(t *testing.T) {
	a := previewApp(
		[]adb.Entry{{Name: "huge.log", Mode: 0o100644, Size: 400 * 1024}},
		map[string][]byte{"/sdcard/huge.log": []byte("unused")},
		nil,
	)
	tt := openDetails(t, a, "huge.log")
	if !tt.HasText("文件太大，无法预览") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if a.detail.edit != nil || a.detail.preview != "" {
		t.Fatalf("body leaked: preview=%q", a.detail.preview)
	}
	saveE2E(t, tt, "e2e-preview-too-large.png")
}

func TestPreviewLayout(t *testing.T) {
	body := []byte("ro.build.version.release=14\n")
	entries := make([]adb.Entry, 40)
	for i := range entries {
		entries[i] = adb.Entry{Name: fmt.Sprintf("file-%02d.txt", i), Mode: 0o100644, Size: uint32(len(body))}
	}
	files := map[string][]byte{}
	for _, e := range entries {
		files["/sdcard/"+e.Name] = body
	}
	a := previewApp(entries, files, nil)
	tt := openDetails(t, a, "file-00.txt")
	tab, ok := tt.Find("sdcard")
	if !ok {
		t.Fatal("tab missing")
	}
	up, ok := tt.Find("向上")
	if !ok {
		t.Fatal("up missing")
	}
	if tab.Y+tab.H > up.Y {
		t.Fatalf("tab strip ending at %.0f overlaps Up at %.0f", tab.Y+tab.H, up.Y)
	}
	file, ok := tt.Find("file-00.txt")
	if !ok {
		t.Fatal("file row missing")
	}
	closeBtn, ok := tt.Find("关闭")
	if !ok {
		t.Fatal("details close missing")
	}
	if closeBtn.X <= file.X {
		t.Fatalf("preview column at x %.0f is not right of the list at %.0f", closeBtn.X, file.X)
	}
	saveE2E(t, tt, "e2e-preview-layout.png")
}

func waitViewer(t *testing.T, a *app, tt *ui.Tester) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if a.viewer.open && !a.viewer.loading {
			tt.Frame()
			return
		}
		time.Sleep(15 * time.Millisecond)
		tt.Frame()
	}
	t.Fatalf("viewer still loading: err=%q kind=%q open=%v", a.viewer.err, a.viewer.kind, a.viewer.open)
}

func widePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 480, 270))
	for y := 0; y < 270; y++ {
		for x := 0; x < 480; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 24, G: 72, B: 140, A: 255})
		}
	}
	for y := 36; y < 140; y++ {
		for x := 36; x < 220; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 240, G: 176, B: 32, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFullscreenViewer(t *testing.T) {
	orig := launchFile
	t.Cleanup(func() { launchFile = orig })
	img := widePNG(t)
	second := pngBytes(t, 48)
	video := []byte("\x00\x00\x00\x18ftypisom")
	a := previewApp(
		[]adb.Entry{
			fileEntry("a-photo.png", img),
			fileEntry("b-second.png", second),
			fileEntry("c-clip.mp4", video),
			fileEntry("notes.txt", []byte("hello")),
		},
		map[string][]byte{
			"/sdcard/a-photo.png":  img,
			"/sdcard/b-second.png": second,
			"/sdcard/c-clip.mp4":   video,
			"/sdcard/notes.txt":    []byte("hello"),
		},
		nil,
	)
	tt := openDetails(t, a, "a-photo.png")
	if a.viewer.open {
		t.Fatal("details opened the fullscreen viewer")
	}
	if !tt.HasText("图片 ·") || !tt.HasText("查看大图") {
		t.Fatalf("side pane: %q", tt.Texts())
	}
	if err := tt.Click("查看大图"); err != nil {
		t.Fatal(err)
	}
	waitViewer(t, a, tt)
	for _, s := range []string{"关闭", "上一张", "下一张", "放大", "缩小", "适应窗口", "实际大小", "向左旋转", "向右旋转", "全屏", "a-photo.png", "1 / 3"} {
		if !tt.HasText(s) {
			t.Fatalf("missing %q in %q", s, tt.Texts())
		}
	}
	if a.viewer.bmp == nil {
		t.Fatalf("no bitmap: %s", a.viewer.err)
	}
	saveE2E(t, tt, "e2e-viewer.png")

	if err := tt.Click("向右旋转"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.viewer.rot != 1 {
		t.Fatalf("rotation %d", a.viewer.rot)
	}
	w, h := a.viewer.bmp.Size()
	if w >= h {
		t.Fatalf("rotated bitmap %dx%d", w, h)
	}
	if err := tt.Click("放大"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.viewer.zoom != 1.5 || !tt.HasText("150%") {
		t.Fatalf("zoom %v texts %q", a.viewer.zoom, tt.Texts())
	}
	if err := tt.Click("下一张"); err != nil {
		t.Fatal(err)
	}
	waitViewer(t, a, tt)
	if a.viewer.name != "b-second.png" || !tt.HasText("2 / 3") {
		t.Fatalf("next: name=%s texts=%q", a.viewer.name, tt.Texts())
	}
	tt.Key(0, ui.KeyLeft)
	waitViewer(t, a, tt)
	if a.viewer.name != "a-photo.png" {
		t.Fatalf("left key: %s", a.viewer.name)
	}
	tt.Key(0, ui.KeyRight)
	waitViewer(t, a, tt)
	if a.viewer.name != "b-second.png" {
		t.Fatalf("right key: %s", a.viewer.name)
	}
	if err := tt.Click("下一张"); err != nil {
		t.Fatal(err)
	}
	waitViewer(t, a, tt)
	if a.viewer.kind != kindVideo || !tt.HasText("播放") || !tt.HasText("视频将用系统播放器播放") {
		t.Fatalf("video: kind=%s err=%s texts=%q", a.viewer.kind, a.viewer.err, tt.Texts())
	}
	var played string
	launchFile = func(path string) error {
		played = path
		return nil
	}
	if err := tt.Click("播放"); err != nil {
		t.Fatal(err)
	}
	if played == "" {
		t.Fatal("play did not open a file")
	}
	t.Cleanup(func() { os.Remove(played) })
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.viewer.open {
		t.Fatal("escape left the viewer open")
	}
	if !tt.HasText("notes.txt") {
		t.Fatal("file list hidden after close")
	}

	r, ok := tt.Find("a-photo.png")
	if !ok {
		t.Fatal("photo left the list")
	}
	x := r.X + 28
	if x > r.X+r.W-2 {
		x = r.X + r.W/2
	}
	y := r.Y + r.H/2
	tt.ClickAt(x, y)
	tt.ClickAt(x, y)
	waitViewer(t, a, tt)
	if !a.viewer.open || a.viewer.name != "a-photo.png" {
		t.Fatalf("double-click viewer name=%s open=%v err=%s", a.viewer.name, a.viewer.open, a.viewer.err)
	}
}

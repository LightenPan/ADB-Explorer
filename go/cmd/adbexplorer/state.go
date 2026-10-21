package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"adbexplorer/internal/adb"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	pageExplorer = "explorer"
	pageDevices  = "devices"
	pageTerm     = "terminal"
	pageOps      = "ops"
	pageApps     = "apps"
	pageSettings = "settings"
	pageLog      = "log"
)

// app is everything the window shows. The view reads it on the main thread;
// background work publishes through win.Update.
type app struct {
	win *mygo.Window

	page     string
	devices  []adb.Device
	serial   string
	status   string
	connErr  string
	logLines []string

	tabs     []*tab
	tabIndex int
	split    float32

	navOpen bool
	asking  bool

	// prompt is a small in-window question: a new name, or empty when closed.
	promptTitle string
	promptValue string
	promptFor   string

	termInput  string
	termOutput string

	browse *job
	jobMu  sync.Mutex

	ops    []*op
	nextOp int

	detailOpen bool
	detail     detail
	viewer     viewer

	// downloadAsk is set while a re-download waits for skip, overwrite, or rename.
	downloadAsk *downloadAsk

	deviceInfo string
	infoGen    int

	apps        []pkgInfo
	pkgErr      string
	pkgQuery    string
	pkgGen      int
	pkgLoading  bool
	appFilter   int
	appSort     string
	appSortDesc bool
	appSel      string

	lang string

	previewLocal map[string][]byte
	previewShell map[string]string
}

// detail is the side panel for the focused file.
type detail struct {
	path     string
	name     string
	kind     string
	size     string
	sizeN    int64
	mod      string
	mode     string
	preview  string
	caption  string
	err      string
	gen      int
	loading  bool
	bytes    []byte
	hexMode  bool
	edit     *editor.State
	lang     string
	bitmaps  []*ui.Bitmap
	svg      *ui.SVG
	svgOwn   bool
	pdfNeed  bool
	pdfWrong bool
	password string
	pages    int
	canEdit  bool
}

func newApp() *app {
	a := &app{
		page:    pageExplorer,
		navOpen: true,
		split:   560,
		lang:    "zh",
		appSort: "name",
	}
	a.tabs = []*tab{{title: "Files", panes: []*pane{newPane()}}}
	return a
}

// tab is one explorer tab. A second pane makes it a split view.
type tab struct {
	title  string
	panes  []*pane
	active int
}

// pane is one directory listing. Each keeps its own list state so a split
// view scrolls and selects independently.
type pane struct {
	path    string
	parent  string
	entries []adb.Entry
	loading bool
	listErr string
	query   string
	deep    bool
	icons   bool

	list   ui.ListState
	grid   ui.GridState
	sort   ui.SortOrder
	chosen ui.Selection[string]
	picked int

	browse *job
}

func newPane() *pane {
	return &pane{path: "/sdcard", picked: -1, sort: ui.SortOrder{Column: "name"}}
}

func (a *app) client() *adb.Client { return &adb.Client{} }

func (a *app) tab() *tab {
	if len(a.tabs) == 0 {
		a.tabs = []*tab{{title: "Files", panes: []*pane{newPane()}}}
	}
	if a.tabIndex < 0 || a.tabIndex >= len(a.tabs) {
		a.tabIndex = 0
	}
	return a.tabs[a.tabIndex]
}

func (a *app) focus() *pane { return a.tab().panes[a.tab().active] }

func (t *tab) titleText() string {
	if len(t.panes) == 0 {
		return "Files"
	}
	return baseName(t.panes[t.active].path)
}

func (a *app) start() {
	go a.refreshDevices()
}

// job is one cancellable piece of background work. A newer job of the same
// kind replaces it; browsing and transferring do not cancel each other.
type job struct {
	gen    int
	cancel context.CancelFunc
}

func (a *app) begin(slot **job) (context.Context, int) {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	gen := 1
	if *slot != nil {
		(*slot).cancel()
		gen = (*slot).gen + 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	*slot = &job{gen: gen, cancel: cancel}
	return ctx, gen
}

func (a *app) current(slot **job, gen int) bool {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	return *slot != nil && (*slot).gen == gen
}

func (a *app) refreshDevices() {
	ctx, gen := a.begin(&a.browse)
	list, err := a.client().Devices(ctx)
	a.publish(gen, func() {
		if err != nil {
			a.connErr = err.Error()
			a.status = a.tr("ADB server unavailable")
			return
		}
		a.connErr = ""
		a.devices = list
		if !a.hasDevice(a.serial) {
			next := ""
			for _, d := range list {
				if d.Ready() {
					next = d.Serial
					break
				}
			}
			a.setSerial(next)
		}
		a.status = a.trf("%d device(s)", len(list))
		if a.serial != "" && a.page == pageExplorer {
			for _, t := range a.tabs {
				for _, p := range t.panes {
					a.loadPane(p, p.path)
				}
			}
		}
	})
}

func (a *app) hasDevice(serial string) bool {
	for _, d := range a.devices {
		if d.Serial == serial && d.Ready() {
			return true
		}
	}
	return false
}

func (a *app) device() (adb.Device, bool) {
	for _, d := range a.devices {
		if d.Serial == a.serial {
			return d, true
		}
	}
	return adb.Device{}, false
}

func (a *app) load(path string) { a.loadPane(a.focus(), path) }

func (a *app) loadPane(p *pane, path string) {
	path = cleanPath(path)
	serial := a.serial
	ctx, gen := a.begin(&p.browse)
	p.path = path
	p.loading = true
	p.listErr = ""
	p.chosen.Clear()
	p.picked = -1
	go func() {
		entries, err := a.client().List(ctx, serial, path)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		a.publishSlot(&p.browse, gen, func() {
			p.loading = false
			if err != nil {
				p.listErr = msg
				p.entries = nil
				return
			}
			p.entries = entries
			p.parent = parentPath(path)
			a.status = a.trf("%d items", len(entries))
		})
	}()
}

func (a *app) newTab() {
	p := newPane()
	if cur := a.focus(); cur != nil {
		p.path = cur.path
	}
	a.tabs = append(a.tabs, &tab{title: "Files", panes: []*pane{p}})
	a.tabIndex = len(a.tabs) - 1
	if a.serial != "" {
		a.loadPane(p, p.path)
	}
}

func (a *app) closeTab(i int) {
	if len(a.tabs) == 1 {
		return
	}
	for _, p := range a.tabs[i].panes {
		a.begin(&p.browse)
	}
	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
	if a.tabIndex >= len(a.tabs) {
		a.tabIndex = len(a.tabs) - 1
	}
}

func (a *app) splitTab() {
	t := a.tab()
	if len(t.panes) == 2 {
		a.begin(&t.panes[1].browse)
		t.panes = t.panes[:1]
		t.active = 0
		return
	}
	p := newPane()
	p.path = t.panes[0].path
	t.panes = append(t.panes, p)
	t.active = 1
	if a.serial != "" {
		a.loadPane(p, p.path)
	}
}

func (a *app) publish(gen int, fn func()) { a.publishSlot(&a.browse, gen, fn) }

func (a *app) publishSlot(slot **job, gen int, fn func()) {
	win := a.win
	if win == nil {
		if a.current(slot, gen) {
			fn()
		}
		return
	}
	win.Update(func() {
		if a.current(slot, gen) {
			fn()
		}
	})
}

func (a *app) note(line string) {
	a.logLines = append(a.logLines, time.Now().Format("15:04:05 ")+line)
	if len(a.logLines) > 200 {
		a.logLines = a.logLines[len(a.logLines)-200:]
	}
}

func (a *app) openArchive(p *pane, name string) {
	path := joinPath(p.path, name)
	low := strings.ToLower(name)
	quoted := shellQuote(path)
	var cmd string
	switch {
	case zipFamily(low):
		cmd = "unzip -l " + quoted
	case tarFamily(low):
		cmd = tarListCmd(low, quoted)
	default:
		return
	}
	serial := a.serial
	p.loading = true
	p.listErr = ""
	ctx, gen := a.begin(&p.browse)
	go func() {
		out, err := a.client().Shell(ctx, serial, cmd)
		a.publishSlot(&p.browse, gen, func() {
			p.loading = false
			if err != nil {
				p.listErr = err.Error()
				return
			}
			p.path = path
			p.parent = parentPath(path)
			p.entries = archiveEntries(out, zipFamily(low))
			p.deep = true
			a.status = a.trf("%d members", len(p.entries))
		})
	}()
}

func archiveEntries(out string, zip bool) []adb.Entry {
	var rows []adb.Entry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name := line
		var size uint32
		if zip {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			n, err := strconv.ParseUint(fields[0], 10, 32)
			if err != nil {
				continue
			}
			size = uint32(n)
			name = fields[len(fields)-1]
			if name == "Name" || name == "----" {
				continue
			}
		}
		mode := uint32(0o100644)
		if strings.HasSuffix(name, "/") {
			mode = 0o040755
			name = strings.TrimSuffix(name, "/")
		}
		if name == "" {
			continue
		}
		rows = append(rows, adb.Entry{Name: name, Mode: mode, Size: size})
	}
	return rows
}

func hiddenName(name string) bool {
	base := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		base = name[i+1:]
	}
	return strings.HasPrefix(base, ".")
}

func isArchive(name string) bool {
	low := strings.ToLower(name)
	return zipFamily(low) || tarFamily(low)
}

func zipFamily(name string) bool {
	return hasExt(name, ".zip", ".jar", ".aar")
}

func tarFamily(name string) bool {
	return hasExt(name, ".tar", ".tar.gz", ".tgz", ".tpz", ".tbz", ".tbz2", ".txz", ".tzst",
		".tar.bz2", ".tar.xz", ".tar.zst")
}

func hasExt(name string, exts ...string) bool {
	low := strings.ToLower(name)
	for _, ext := range exts {
		if strings.HasSuffix(low, ext) {
			return true
		}
	}
	return false
}

func tarListCmd(low, quoted string) string {
	switch {
	case strings.HasSuffix(low, ".tar.gz"), strings.HasSuffix(low, ".tgz"), strings.HasSuffix(low, ".tpz"):
		return "tar -tzf " + quoted
	case strings.HasSuffix(low, ".tar.bz2"), strings.HasSuffix(low, ".tbz2"), strings.HasSuffix(low, ".tbz"):
		return "tar -tjf " + quoted
	case strings.HasSuffix(low, ".tar.xz"), strings.HasSuffix(low, ".txz"):
		return "tar -tJf " + quoted
	case strings.HasSuffix(low, ".tar.zst"), strings.HasSuffix(low, ".tzst"):
		return "tar --zstd -tf " + quoted
	default:
		return "tar -tf " + quoted
	}
}

func tarExtractCmd(low, quoted, dest string) string {
	switch {
	case strings.HasSuffix(low, ".tar.gz"), strings.HasSuffix(low, ".tgz"), strings.HasSuffix(low, ".tpz"):
		return "tar -xzf " + quoted + " -C " + dest
	case strings.HasSuffix(low, ".tar.bz2"), strings.HasSuffix(low, ".tbz2"), strings.HasSuffix(low, ".tbz"):
		return "tar -xjf " + quoted + " -C " + dest
	case strings.HasSuffix(low, ".tar.xz"), strings.HasSuffix(low, ".txz"):
		return "tar -xJf " + quoted + " -C " + dest
	case strings.HasSuffix(low, ".tar.zst"), strings.HasSuffix(low, ".tzst"):
		return "tar --zstd -xf " + quoted + " -C " + dest
	default:
		return "tar -xf " + quoted + " -C " + dest
	}
}

func archiveDest(name string) string {
	low := strings.ToLower(name)
	for _, s := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst", ".tgz", ".tbz2", ".tbz", ".txz", ".tzst", ".tpz", ".tar", ".zip", ".jar", ".aar"} {
		if strings.HasSuffix(low, s) {
			return name[:len(name)-len(s)]
		}
	}
	return name + "-out"
}

// extract unpacks the selected archive next to itself.
func (a *app) extract() {
	p := a.focus()
	sel := selectedOf(p, a.visibleOf(p))
	if len(sel) != 1 || !isArchive(sel[0].Name) {
		a.status = a.tr("Select one archive")
		return
	}
	name := sel[0].Name
	remote := joinPath(p.path, name)
	dest := joinPath(p.path, archiveDest(name))
	low := strings.ToLower(name)
	var cmd string
	if zipFamily(low) {
		cmd = "mkdir -p " + shellQuote(dest) + " && unzip -o " + shellQuote(remote) + " -d " + shellQuote(dest)
	} else {
		cmd = "mkdir -p " + shellQuote(dest) + " && " + tarExtractCmd(low, shellQuote(remote), shellQuote(dest))
	}
	a.shellOp("extract", name, cmd, p.path)
}

// pack stores the selection in a zip next to it.
func (a *app) pack() {
	p := a.focus()
	sel := selectedOf(p, a.visibleOf(p))
	if len(sel) == 0 {
		a.status = a.tr("Select items to zip")
		return
	}
	var names []string
	for _, e := range sel {
		names = append(names, shellQuote(e.Name))
	}
	out := "archive.zip"
	for i := 2; a.has(out); i++ {
		out = fmt.Sprintf("archive-%d.zip", i)
	}
	cmd := "cd " + shellQuote(p.path) + " && zip -r " + shellQuote(out) + " " + strings.Join(names, " ")
	a.shellOp("zip", out, cmd, p.path)
}

// chown changes the owner of the one selected item.
func (a *app) chown(owner string) {
	owner = strings.TrimSpace(owner)
	if a.serial == "" || owner == "" || strings.ContainsAny(owner, " ;|&`$") {
		a.status = a.tr("Invalid owner")
		return
	}
	p := a.focus()
	sel := selectedOf(p, a.visibleOf(p))
	if len(sel) != 1 {
		a.status = a.tr("Select one item")
		return
	}
	path := joinPath(p.path, sel[0].Name)
	a.shellOp("chown", sel[0].Name, "chown "+owner+" "+shellQuote(path), p.path)
}

// searchContent finds files whose contents contain the query.
func (a *app) searchContent() {
	p := a.focus()
	q := strings.TrimSpace(p.query)
	if a.serial == "" || q == "" || strings.ContainsAny(q, `'"\$`) {
		a.status = a.tr("Type a simple word to search")
		return
	}
	serial, dir := a.serial, p.path
	p.loading = true
	p.listErr = ""
	ctx, gen := a.begin(&p.browse)
	go func() {
		out, err := a.client().Shell(ctx, serial, "grep -rl "+shellQuote(q)+" "+shellQuote(dir)+" | head -n 200")
		a.publishSlot(&p.browse, gen, func() {
			p.loading = false
			if err != nil {
				p.listErr = err.Error()
				return
			}
			var rows []adb.Entry
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || line == dir {
					continue
				}
				name := strings.TrimPrefix(strings.TrimPrefix(line, dir), "/")
				rows = append(rows, adb.Entry{Name: name, Mode: 0o100644})
			}
			p.entries = rows
			p.deep = true
			a.status = a.trf("%d files contain it", len(rows))
		})
	}()
}

func (a *app) searchDeep() {
	p := a.focus()
	q := strings.TrimSpace(p.query)
	if a.serial == "" || q == "" || strings.ContainsAny(q, `'"\$`) {
		a.status = a.tr("Type a simple name to search")
		return
	}
	serial, path := a.serial, p.path
	p.loading = true
	p.listErr = ""
	ctx, gen := a.begin(&p.browse)
	go func() {
		out, err := a.client().Shell(ctx, serial, "find "+shellQuote(path)+" -iname "+shellQuote("*"+q+"*")+" | head -n 200")
		a.publishSlot(&p.browse, gen, func() {
			p.loading = false
			if err != nil {
				p.listErr = err.Error()
				return
			}
			var rows []adb.Entry
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || line == path {
					continue
				}
				name := strings.TrimPrefix(line, path)
				name = strings.TrimPrefix(name, "/")
				mode := uint32(0o100644)
				if strings.HasSuffix(line, "/") {
					mode = 0o040755
					name = strings.TrimSuffix(name, "/")
				}
				rows = append(rows, adb.Entry{Name: name, Mode: mode})
			}
			p.entries = rows
			p.deep = true
			a.status = a.trf("%d matches", len(rows))
		})
	}()
}

func (a *app) visible() []adb.Entry { return a.visibleOf(a.focus()) }

func (a *app) visibleOf(p *pane) []adb.Entry {
	q := strings.ToLower(strings.TrimSpace(p.query))
	rows := make([]adb.Entry, 0, len(p.entries))
	for _, e := range p.entries {
		if !a.showHidden() && hiddenName(e.Name) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.Name), q) {
			continue
		}
		rows = append(rows, e)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].IsDir() != rows[j].IsDir() {
			return rows[i].IsDir()
		}
		less := false
		switch p.sort.Column {
		case "size":
			less = rows[i].Size < rows[j].Size
		case "modified":
			less = rows[i].MTime.Before(rows[j].MTime)
		case "type":
			less = kindOf(rows[i]) < kindOf(rows[j])
		default:
			less = strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
		}
		if p.sort.Descending {
			return !less
		}
		return less
	})
	return rows
}

func (a *app) selected(rows []adb.Entry) []adb.Entry { return selectedOf(a.focus(), rows) }

func selectedOf(p *pane, rows []adb.Entry) []adb.Entry {
	if p.chosen.Len() == 0 {
		if p.picked >= 0 && p.picked < len(rows) {
			return []adb.Entry{rows[p.picked]}
		}
		return nil
	}
	var out []adb.Entry
	for _, e := range rows {
		if p.chosen.Has(e.Name) {
			out = append(out, e)
		}
	}
	return out
}

func (a *app) remove(entries []adb.Entry) {
	if len(entries) == 0 || a.serial == "" {
		return
	}
	remote := a.focus().path
	var parts []string
	for _, e := range entries {
		flag := ""
		if e.IsDir() {
			flag = "-r "
		}
		parts = append(parts, "rm -f "+flag+shellQuote(joinPath(remote, e.Name)))
	}
	label := entries[0].Name
	if len(entries) > 1 {
		label = a.trf("%d items", len(entries))
	}
	a.shellOp("delete", label, strings.Join(parts, " && "), remote)
}

func (a *app) mkdir() {
	if a.serial == "" {
		return
	}
	a.ask(a.tr("New folder"), a.tr("New folder"), "folder")
}

// copyTo copies or moves the focused selection into the other pane.
func (a *app) uninstall(name string) {
	name = strings.TrimSpace(name)
	if a.serial == "" || name == "" || strings.ContainsAny(name, " ;|&") {
		a.status = a.tr("Invalid package")
		return
	}
	serial := a.serial
	a.enqueue("uninstall", name, 0, func(_ *op, ctx context.Context, _ func(int64)) error {
		out, err := a.client().Shell(ctx, serial, "pm uninstall "+name)
		if err != nil {
			return err
		}
		if !strings.Contains(out, "Success") {
			return fmt.Errorf("%s", strings.TrimSpace(out))
		}
		a.winUpdate(func() { a.loadPackages() })
		return nil
	})
}

func (a *app) installAPK(local string) {
	if a.serial == "" || local == "" {
		return
	}
	serial := a.serial
	name := filepath.Base(local)
	info, err := os.Stat(local)
	if err != nil {
		a.status = err.Error()
		return
	}
	a.enqueue("install", name, info.Size(), func(_ *op, ctx context.Context, _ func(int64)) error {
		if err := a.client().Install(ctx, serial, local); err != nil {
			return err
		}
		a.winUpdate(func() { a.loadPackages() })
		return nil
	})
}

func (a *app) loadDeviceInfo() {
	if a.serial == "" {
		a.deviceInfo = ""
		return
	}
	a.infoGen++
	gen := a.infoGen
	serial := a.serial
	a.deviceInfo = a.tr("Reading…")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		df, err1 := a.client().Shell(ctx, serial, "df -h /data /sdcard 2>/dev/null | head -n 6")
		bat, err2 := a.client().Shell(ctx, serial, "dumpsys battery | grep -E 'level|status|temperature' | head -n 8")
		text := strings.TrimSpace(df)
		if err1 != nil {
			text = err1.Error()
		}
		if err2 == nil && strings.TrimSpace(bat) != "" {
			text += "\n\n" + strings.TrimSpace(bat)
		}
		a.winUpdate(func() {
			if a.infoGen == gen {
				a.deviceInfo = text
			}
		})
	}()
}

func (a *app) chmod(mode string) {
	mode = strings.TrimSpace(mode)
	if a.serial == "" || mode == "" {
		return
	}
	for _, r := range mode {
		if r < '0' || r > '7' {
			a.status = a.tr("Mode must be octal, like 644")
			return
		}
	}
	p := a.focus()
	sel := selectedOf(p, a.visibleOf(p))
	if len(sel) != 1 {
		a.status = a.tr("Select one item")
		return
	}
	path := joinPath(p.path, sel[0].Name)
	a.shellOp("chmod", sel[0].Name, "chmod "+mode+" "+shellQuote(path), p.path)
}

func modeString(mode uint32) string {
	perm := mode & 0o777
	kind := '-'
	switch mode & 0o170000 {
	case 0o040000:
		kind = 'd'
	case 0o120000:
		kind = 'l'
	}
	return string(kind) + fmt.Sprintf("%03o", perm)
}

func (a *app) copyTo(move bool) {
	tab := a.tab()
	if len(tab.panes) != 2 || a.serial == "" {
		a.status = a.tr("Split the view first")
		return
	}
	src := tab.panes[tab.active]
	dst := tab.panes[1-tab.active]
	if src.path == dst.path {
		a.status = a.tr("Both panes show the same folder")
		return
	}
	sel := selectedOf(src, a.visibleOf(src))
	if len(sel) == 0 {
		a.status = a.tr("Select items to copy")
		return
	}
	var parts []string
	for _, e := range sel {
		from := shellQuote(joinPath(src.path, e.Name))
		to := shellQuote(dst.path + "/")
		if move {
			parts = append(parts, "mv "+from+" "+to)
		} else if e.IsDir() {
			parts = append(parts, "cp -r "+from+" "+to)
		} else {
			parts = append(parts, "cp "+from+" "+to)
		}
	}
	kind := "copy"
	if move {
		kind = "move"
	}
	label := sel[0].Name
	if len(sel) > 1 {
		label = a.trf("%d items", len(sel))
	}
	other := dst
	otherPath := dst.path
	serial := a.serial
	command := strings.Join(parts, " && ")
	a.enqueue(kind, label, 0, func(_ *op, ctx context.Context, _ func(int64)) error {
		out, err := a.client().Shell(ctx, serial, command)
		if err != nil {
			return err
		}
		msg := strings.TrimSpace(out)
		low := strings.ToLower(msg)
		if msg != "" && (strings.Contains(low, "no such") || strings.Contains(msg, "Permission denied") || strings.Contains(low, "failed")) {
			return fmt.Errorf("%s", msg)
		}
		a.winUpdate(func() {
			a.loadPane(src, src.path)
			a.loadPane(other, otherPath)
		})
		return nil
	})
}

func (a *app) rename(from, to string) {
	to = strings.TrimSpace(to)
	if a.serial == "" || from == "" || to == "" || from == to || strings.ContainsAny(to, `/\`) {
		a.status = a.tr("Invalid name")
		return
	}
	remote := a.focus().path
	a.shellOp("rename", to, "mv "+shellQuote(joinPath(remote, from))+" "+shellQuote(joinPath(remote, to)), remote)
}

func (a *app) runShell(command string) {
	command = strings.TrimSpace(command)
	if a.serial == "" || command == "" {
		return
	}
	serial := a.serial
	a.termOutput += "$ " + command + "\n"
	a.enqueue("shell", command, 0, func(_ *op, ctx context.Context, _ func(int64)) error {
		out, err := a.client().Shell(ctx, serial, command)
		a.winUpdate(func() {
			if err != nil {
				a.termOutput += err.Error() + "\n"
				return
			}
			a.termOutput += out
			if !strings.HasSuffix(a.termOutput, "\n") {
				a.termOutput += "\n"
			}
		})
		return err
	})
}

func (a *app) ask(title, value, target string) {
	a.promptTitle = title
	a.promptValue = value
	a.promptFor = target
}

func (a *app) acceptPrompt() {
	target := a.promptFor
	value := a.promptValue
	a.promptFor = ""
	switch {
	case target == "folder":
		a.makeNamed(value)
	case strings.HasPrefix(target, "rename:"):
		a.rename(strings.TrimPrefix(target, "rename:"), value)
	case target == "chmod":
		a.chmod(value)
	case target == "chown":
		a.chown(value)
	}
}

func (a *app) makeNamed(name string) {
	name = strings.TrimSpace(name)
	if a.serial == "" || name == "" || strings.ContainsAny(name, `/\`) {
		a.status = a.tr("Invalid name")
		return
	}
	remote := a.focus().path
	a.shellOp("folder", name, "mkdir "+shellQuote(joinPath(remote, name)), remote)
}

func (a *app) has(name string) bool {
	for _, e := range a.focus().entries {
		if e.Name == name {
			return true
		}
	}
	return false
}

func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return pathClean(p)
}

func pathClean(p string) string {
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return "/"
	}
	return "/" + strings.Join(out, "/")
}

func baseName(p string) string {
	if p == "/" {
		return "root"
	}
	return p[strings.LastIndex(p, "/")+1:]
}

func parentPath(p string) string {
	if p == "/" {
		return ""
	}
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}

func joinPath(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func kindOf(e adb.Entry) string {
	switch {
	case e.IsLink():
		return "Link"
	case e.IsDir():
		return "Folder"
	default:
		return "File"
	}
}

func (a *app) setSerial(s string) {
	if a.serial == s {
		return
	}
	a.serial = s
	clearIconCache()
	clearPreviewCache()
	a.detail = detail{}
	a.detailOpen = false
	a.closeViewer()
}

func sizeOf(e adb.Entry) string {
	if e.IsDir() {
		return "—"
	}
	return sizeBytes(int64(e.Size))
}

func sizeBytes(n int64) string {
	f := float64(n)
	for _, unit := range []string{"B", "KB", "MB", "GB"} {
		if f < 1024 || unit == "GB" {
			if unit == "B" {
				return fmt.Sprintf("%d B", n)
			}
			return fmt.Sprintf("%.1f %s", f, unit)
		}
		f /= 1024
	}
	return ""
}

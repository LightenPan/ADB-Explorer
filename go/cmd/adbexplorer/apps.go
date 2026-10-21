package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// pkgInfo is one installed package. Name is the label when adb reports one,
// otherwise the package id. Icon is filled later, and stays nil until then.
type pkgInfo struct {
	ID      string
	Name    string
	Version string
	Updated string
	Path    string
	Size    int64
	System  bool
	Icon    *ui.Bitmap
}

// Ways the package listing can come back wrong, which a click-through of the
// page would not catch on its own:
//   - the apk path contains "=" and the package id is only the last field
//   - dumpsys indents "versionName=" and "nonLocalizedLabel=null"
//   - a package block has no version
//   - stat prints "size|path" and the path itself contains "|"
//   - /system, /product, /vendor, /apex and pkgFlags SYSTEM both mean a system app

func (a *app) loadPackages() {
	if a.previewShell != nil {
		pkgs := parsePM(a.previewShell["pm list packages -f"])
		mergeDump(pkgs, a.previewShell["dumpsys-packages"])
		applyStat(pkgs, a.previewShell["pkg-stat"])
		a.attachLocalIcons(pkgs)
		a.apps = pkgs
		a.pkgErr = ""
		a.pkgLoading = false
		a.status = a.trf("%d packages", len(pkgs))
		return
	}
	if a.serial == "" {
		a.pkgErr = a.tr("No device")
		a.pkgLoading = false
		return
	}
	a.pkgGen++
	gen := a.pkgGen
	serial := a.serial
	a.pkgErr = ""
	a.pkgLoading = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		pm, err := a.client().Shell(ctx, serial, "pm list packages -f")
		msg := ""
		var pkgs []pkgInfo
		if err != nil {
			msg = err.Error()
		} else {
			pkgs = parsePM(pm)
			if dump, dumpErr := a.client().Shell(ctx, serial, `dumpsys package | grep -E 'Package \[|versionName=|lastUpdateTime=|nonLocalizedLabel=|pkgFlags='`); dumpErr == nil {
				mergeDump(pkgs, dump)
			}
			a.statPackages(ctx, serial, pkgs)
		}
		a.winUpdate(func() {
			if a.pkgGen != gen {
				return
			}
			a.apps = pkgs
			a.pkgErr = msg
			a.pkgLoading = false
			if msg == "" {
				a.status = a.trf("%d packages", len(pkgs))
			}
		})
		if err == nil {
			a.loadPkgIcons(gen, serial, pkgs)
		}
	}()
}

func (a *app) statPackages(ctx context.Context, serial string, pkgs []pkgInfo) {
	const batch = 40
	for i := 0; i < len(pkgs); i += batch {
		end := i + batch
		if end > len(pkgs) {
			end = len(pkgs)
		}
		var b strings.Builder
		b.WriteString("stat -c '%s|%n'")
		n := 0
		for _, p := range pkgs[i:end] {
			if p.Path == "" {
				continue
			}
			b.WriteByte(' ')
			b.WriteString(shellQuote(p.Path))
			n++
		}
		if n == 0 {
			continue
		}
		out, err := a.client().Shell(ctx, serial, b.String())
		if err != nil {
			continue
		}
		applyStat(pkgs, out)
	}
}

func (a *app) attachLocalIcons(pkgs []pkgInfo) {
	if a.previewLocal == nil {
		return
	}
	for i := range pkgs {
		data := a.previewLocal[pkgs[i].Path]
		if len(data) == 0 {
			continue
		}
		png, ok := iconFor(data)
		if !ok {
			continue
		}
		if bmp, err := ui.DecodeBitmap(png); err == nil {
			pkgs[i].Icon = bmp
		}
	}
}

// loadPkgIcons reads each apk's launcher image on the device and publishes
// it as it arrives. The whole apk is not pulled: unzip -l finds the png or
// webp, and unzip -p | base64 brings back only that file. A package with no
// bitmap icon keeps the letter tile.
//
// Ways this can fail that a click-through would miss:
//   - the apk path or entry contains a quote or a space
//   - unzip -l puts the entry after "res/" and the name itself contains spaces
//   - base64 is wrapped, or the shell inserts CR
//   - the bytes are an error string, not a png or webp
//   - a newer listing started, so a slow batch must not paint over it
func (a *app) loadPkgIcons(gen int, serial string, pkgs []pkgInfo) {
	const batch = 20
	for i := 0; i < len(pkgs); i += batch {
		if a.pkgGen != gen {
			return
		}
		end := i + batch
		if end > len(pkgs) {
			end = len(pkgs)
		}
		chunk := pkgs[i:end]
		script := iconListingScript(chunk)
		if script == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		listings, err := a.client().Shell(ctx, serial, script)
		var jobs []iconJob
		if err == nil {
			for id, listing := range parseMarked(listings) {
				entry := bestIconEntry(listing)
				if entry == "" {
					continue
				}
				for _, p := range chunk {
					if p.ID == id && p.Path != "" {
						jobs = append(jobs, iconJob{id: id, path: p.Path, entry: entry})
					}
				}
			}
		}
		var blobs map[string]string
		if len(jobs) > 0 {
			raw, blobErr := a.client().Shell(ctx, serial, iconBlobScript(jobs))
			if blobErr == nil {
				blobs = parseMarked(raw)
			}
		}
		cancel()
		decoded := map[string]*ui.Bitmap{}
		for id, text := range blobs {
			bmp, ok := bitmapFromBase64(text)
			if ok {
				decoded[id] = bmp
			}
		}
		if len(decoded) == 0 {
			continue
		}
		a.winUpdate(func() {
			if a.pkgGen != gen {
				return
			}
			for i := range a.apps {
				if bmp := decoded[a.apps[i].ID]; bmp != nil {
					a.apps[i].Icon = bmp
				}
			}
		})
	}
}

type iconJob struct {
	id, path, entry string
}

func iconListingScript(pkgs []pkgInfo) string {
	var b strings.Builder
	for _, p := range pkgs {
		if p.Path == "" || !safePkgID(p.ID) {
			continue
		}
		q := shellQuote(p.Path)
		fmt.Fprintf(&b, "echo '##PKG %s'; unzip -l %s 2>/dev/null || toybox unzip -l %s 2>/dev/null; echo '##END';", p.ID, q, q)
	}
	return b.String()
}

func iconBlobScript(jobs []iconJob) string {
	var b strings.Builder
	for _, job := range jobs {
		if !safePkgID(job.id) {
			continue
		}
		fmt.Fprintf(&b, "echo '##PKG %s'; unzip -p %s %s 2>/dev/null | (base64 2>/dev/null || toybox base64); echo; echo '##END';",
			job.id, shellQuote(job.path), shellQuote(job.entry))
	}
	return b.String()
}

func safePkgID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_':
		default:
			return false
		}
	}
	return true
}

func parseMarked(out string) map[string]string {
	res := map[string]string{}
	var id string
	var buf strings.Builder
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "##PKG ") {
			id = strings.TrimSpace(strings.TrimPrefix(line, "##PKG "))
			buf.Reset()
			continue
		}
		if strings.TrimSpace(line) == "##END" {
			if id != "" {
				res[id] = buf.String()
			}
			id = ""
			continue
		}
		if id != "" {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	return res
}

func bestIconEntry(listing string) string {
	best := ""
	bestScore := -1 << 30
	for _, line := range strings.Split(listing, "\n") {
		i := strings.Index(line, "res/")
		if i < 0 {
			continue
		}
		name := strings.TrimSpace(line[i:])
		low := strings.ToLower(name)
		if !strings.HasSuffix(low, ".png") && !strings.HasSuffix(low, ".webp") {
			continue
		}
		if !strings.Contains(low, "icon") && !strings.Contains(low, "launcher") {
			continue
		}
		size := leadingInt(line)
		if size > 512*1024 {
			continue
		}
		score := iconScore(low, size)
		if strings.Contains(low, "background") {
			score -= 80000
		}
		if score > bestScore {
			bestScore = score
			best = name
		}
	}
	return best
}

func leadingInt(line string) int {
	line = strings.TrimSpace(line)
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0
	}
	n, err := strconv.Atoi(line[:i])
	if err != nil {
		return 0
	}
	return n
}

func bitmapFromBase64(text string) (*ui.Bitmap, bool) {
	clean := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, text)
	if clean == "" {
		return nil, false
	}
	raw, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(clean)
		if err != nil {
			return nil, false
		}
	}
	if len(raw) < 12 {
		return nil, false
	}
	bmp, err := ui.DecodeBitmap(raw)
	if err != nil {
		return nil, false
	}
	return bmp, true
}

func parsePM(out string) []pkgInfo {
	var list []pkgInfo
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "package:") {
			continue
		}
		rest := strings.TrimPrefix(line, "package:")
		i := strings.LastIndex(rest, "=")
		if i <= 0 || i >= len(rest)-1 {
			continue
		}
		path, id := rest[:i], rest[i+1:]
		if j := strings.IndexAny(id, " \t"); j >= 0 {
			id = id[:j]
		}
		if id == "" || seen[id] || strings.ContainsAny(id, " \t") {
			continue
		}
		seen[id] = true
		list = append(list, pkgInfo{
			ID:     id,
			Name:   id,
			Path:   path,
			System: isSystemPath(path),
		})
	}
	return list
}

func isSystemPath(p string) bool {
	for _, pre := range []string{"/system/", "/system_ext/", "/product/", "/vendor/", "/apex/", "/odm/"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

func mergeDump(pkgs []pkgInfo, dump string) {
	idx := map[string]*pkgInfo{}
	for i := range pkgs {
		idx[pkgs[i].ID] = &pkgs[i]
	}
	var cur *pkgInfo
	for _, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Package [") {
			rest := strings.TrimPrefix(line, "Package [")
			id := rest
			if i := strings.IndexByte(rest, ']'); i >= 0 {
				id = rest[:i]
			}
			cur = idx[id]
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "versionName="):
			v := strings.TrimPrefix(line, "versionName=")
			if v != "" && v != "null" {
				cur.Version = v
			}
		case strings.HasPrefix(line, "lastUpdateTime="):
			v := strings.TrimPrefix(line, "lastUpdateTime=")
			if len(v) >= 10 && v[4] == '-' {
				cur.Updated = v[:10]
			}
		case strings.HasPrefix(line, "nonLocalizedLabel="):
			v := strings.TrimPrefix(line, "nonLocalizedLabel=")
			if v != "" && v != "null" {
				cur.Name = v
			}
		case strings.HasPrefix(line, "pkgFlags=") && strings.Contains(line, "SYSTEM"):
			cur.System = true
		}
	}
}

func applyStat(pkgs []pkgInfo, out string) {
	byPath := map[string]int{}
	for i := range pkgs {
		if pkgs[i].Path != "" {
			byPath[pkgs[i].Path] = i
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		i := strings.IndexByte(line, '|')
		if i <= 0 {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(line[:i]), 10, 64)
		if err != nil {
			continue
		}
		path := line[i+1:]
		if idx, ok := byPath[path]; ok {
			pkgs[idx].Size = n
		}
	}
}

func (a *app) visibleApps() []pkgInfo {
	q := strings.ToLower(strings.TrimSpace(a.pkgQuery))
	out := make([]pkgInfo, 0, len(a.apps))
	for _, p := range a.apps {
		if a.appFilter == 1 && p.System {
			continue
		}
		if a.appFilter == 2 && !p.System {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(p.Name), q) && !strings.Contains(strings.ToLower(p.ID), q) {
			continue
		}
		out = append(out, p)
	}
	sortPkgs(out, a.appSort, a.appSortDesc)
	if len(out) > 400 {
		out = out[:400]
	}
	return out
}

func sortPkgs(out []pkgInfo, key string, desc bool) {
	less := func(i, j int) bool {
		var before bool
		switch key {
		case "size":
			before = out[i].Size < out[j].Size
		case "version":
			before = out[i].Version < out[j].Version
		case "updated":
			before = out[i].Updated < out[j].Updated
		default:
			before = strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		if desc {
			return !before
		}
		return before
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && less(j, j-1); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
}

func (a *app) selectedApp(rows []pkgInfo) *pkgInfo {
	for i := range rows {
		if rows[i].ID == a.appSel {
			return &rows[i]
		}
	}
	return nil
}

func (a *app) appsPage(c *ui.Context) {
	t := c.Theme()
	rows := a.visibleApps()
	ui.Column(c).Fill().Padding(12, 16).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, a.tr("Apps")).FontSize(20).Bold()
			ui.Spacer(c)
			for i, key := range []string{"All", "User apps", "System apps"} {
				b := ui.Button(c.Key("app-filter-"+key), a.tr(key))
				if a.appFilter == i {
					b.Background(t.SurfacePressed).TextColor(t.Accent)
				}
				if b.Clicked() {
					a.appFilter = i
				}
			}
			ui.SearchField(c, &a.pkgQuery).Width(200).Placeholder(a.tr("Filter apps"))
			if ui.Button(c, a.tr("Install apk")).Disabled(a.serial == "").Clicked() {
				a.pickInstall()
			}
			if ui.Button(c, a.tr("Refresh")).Disabled(a.serial == "" && a.previewShell == nil).Clicked() {
				a.loadPackages()
			}
		})
		if a.pkgErr != "" {
			ui.Text(c, a.pkgErr).TextColor(t.Danger)
		}
		if a.pkgLoading && len(a.apps) == 0 {
			ui.Text(c, a.tr("Reading…")).TextColor(t.TextMuted)
			return
		}
		if len(a.apps) == 0 {
			ui.Text(c, a.tr("Refresh to list installed packages.")).TextColor(t.TextMuted)
			return
		}
		userN := 0
		var total int64
		for _, p := range rows {
			if !p.System {
				userN++
			}
			total += p.Size
		}
		ui.Text(c, a.trf("%d apps · %d user · %s", len(rows), userN, sizeBytes(total))).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Grow(1).Children(func() {
			ui.Column(c).Grow(1).FillHeight().Children(func() {
				a.appHeader(c)
				if len(rows) == 0 {
					ui.Text(c, a.tr("No matching apps")).Padding(16).TextColor(t.TextMuted)
					return
				}
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).Children(func() {
						for _, p := range rows {
							a.appRow(c, p)
						}
					})
				})
			})
			ui.Box(c).Width(1).FillHeight().Background(t.Border)
			a.appDetail(c, a.selectedApp(rows))
		})
	})
}

func (a *app) appHeader(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(28).Padding(0, 8).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(36)
		for _, col := range []struct{ key, label string }{
			{"name", "Name"},
			{"version", "Version"},
			{"size", "Size"},
			{"updated", "Updated"},
		} {
			label := a.tr(col.label)
			if a.appSort == col.key {
				if a.appSortDesc {
					label += " ↓"
				} else {
					label += " ↑"
				}
			}
			b := ui.Button(c.Key("app-sort-"+col.key), label)
			if col.key == "name" {
				b.Grow(1)
			} else if col.key == "version" {
				b.Width(120)
			} else {
				b.Width(88)
			}
			if a.appSort == col.key {
				b.TextColor(t.Text)
			} else {
				b.TextColor(t.TextMuted)
			}
			key := col.key
			if b.Clicked() {
				if a.appSort == key {
					a.appSortDesc = !a.appSortDesc
				} else {
					a.appSort = key
					a.appSortDesc = key == "size" || key == "updated"
				}
			}
		}
		ui.Text(c, a.tr("Type")).Width(48).TextColor(t.TextMuted).FontSize(12)
	})
}

func (a *app) appRow(c *ui.Context, p pkgInfo) {
	t := c.Theme()
	row := ui.Row(c.Key("pkg-"+p.ID)).Height(56).Padding(4, 8).Gap(8).AlignItems(ui.Center)
	if p.ID == a.appSel {
		row.Background(t.SurfacePressed)
	} else if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		a.pkgIcon(c, p, 36)
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, p.Name).Bold().SingleLine()
			ui.Text(c, p.ID).FontSize(12).TextColor(t.TextMuted).SingleLine()
		})
		ver := p.Version
		if ver == "" {
			ver = "—"
		}
		ui.Text(c, ver).Width(120).SingleLine()
		size := "—"
		if p.Size > 0 {
			size = sizeBytes(p.Size)
		}
		ui.Text(c, size).Width(88).SingleLine()
		updated := p.Updated
		if updated == "" {
			updated = "—"
		}
		ui.Text(c, updated).Width(88).SingleLine()
		kind := a.tr("User")
		if p.System {
			kind = a.tr("System")
		}
		ui.Text(c, kind).Width(48).TextColor(t.TextMuted)
	})
	if row.Clicked() {
		a.appSel = p.ID
	}
}

func (a *app) pkgIcon(c *ui.Context, p pkgInfo, size float32) {
	if p.Icon != nil {
		ui.Image(c, p.Icon).Size(size, size).Radius(8)
		return
	}
	ui.Column(c).Width(size).Height(size).Radius(8).Background(pkgColor(p.ID)).Center().Children(func() {
		ui.Text(c, pkgMark(p.Name)).Bold().TextColor(ui.Hex("#ffffff"))
	})
}

func (a *app) appDetail(c *ui.Context, p *pkgInfo) {
	t := c.Theme()
	ui.Column(c).Width(300).FillHeight().Padding(12).Gap(6).Background(t.Surface).Children(func() {
		ui.Text(c, a.tr("App details")).Bold()
		if p == nil {
			ui.Text(c, a.tr("Select an app")).TextColor(t.TextMuted)
			return
		}
		a.pkgIcon(c, *p, 64)
		ui.Text(c, p.Name).Bold().FontSize(16)
		ui.Text(c, p.ID).FontSize(12).TextColor(t.TextMuted)
		a.appFact(c, a.tr("Version"), blank(p.Version))
		size := "—"
		if p.Size > 0 {
			size = sizeBytes(p.Size)
		}
		a.appFact(c, a.tr("Size"), size)
		kind := a.tr("User app")
		if p.System {
			kind = a.tr("System app")
		}
		a.appFact(c, a.tr("Type"), kind)
		a.appFact(c, a.tr("Updated"), blank(p.Updated))
		if ui.Button(c, a.tr("Uninstall")).Disabled(p.System || a.serial == "").Clicked() {
			a.uninstall(p.ID)
		}
	})
}

func (a *app) appFact(c *ui.Context, k, v string) {
	t := c.Theme()
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, k).Width(72).TextColor(t.TextMuted)
		ui.Text(c, v).Grow(1)
	})
}

func blank(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

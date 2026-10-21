package main

import (
	"fmt"
	"strings"

	"adbexplorer/internal/adb"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// paneDrag is a selection dragged from one pane to the other.
type paneDrag struct{ from int }

func (a *app) view(c *ui.Context) {
	if c.Shortcut(ui.Ctrl, ui.KeyV) {
		a.pasteClipboard()
	}
	c.SetTheme(micaTheme())
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.caption(c)
		ui.Row(c).Grow(1).Children(func() {
			a.rail(c)
			if a.page == pageExplorer && a.navOpen {
				a.sidebar(c)
				ui.Box(c).Width(1).FillHeight().Background(t.Border)
			}
			ui.Column(c).Grow(1).FillHeight().Background(ui.Hex("#ffffff")).Children(func() {
				switch a.page {
				case pageDevices:
					a.devicesPage(c)
				case pageTerm:
					a.terminalPage(c)
				case pageOps:
					a.opsPage(c)
				case pageApps:
					a.appsPage(c)
				case pageLog:
					a.logPage(c)
				case pageSettings:
					a.settingsPage(c)
				default:
					a.explorer(c)
				}
			})
		})
		a.statusbar(c)
	})
	a.namePrompt(c)
	a.editOverlay(c)
	a.conflictPrompt(c)
	a.downloadConflict(c)
	if a.asking {
		rows := a.visible()
		n := len(a.selected(rows))
		switch ui.AlertDialog(c, &a.asking, a.tr("Delete"), a.trf("Delete %d item(s)? This cannot be undone.", n), a.tr("Cancel"), a.tr("Delete")) {
		case 1:
			a.remove(a.selected(rows))
		}
	}
	a.viewerOverlay(c)
}

func (a *app) caption(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	h := bar.Height
	if h < 32 {
		h = 40
	}
	ui.Row(c).Height(h).Padding(0, 8+bar.Right, 0, 12+bar.Left).Gap(8).AlignItems(ui.Center).
		DragWindow().
		Background(ui.Hex("#f6f6f6")).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if bmp := appIcon(); bmp != nil {
			ui.Image(c, bmp).Size(16, 16).Radius(4).DragWindow()
		} else {
			ui.Box(c).Width(16).Height(16).Radius(4).Background(t.Accent).DragWindow()
		}
		ui.Text(c, "ADB Explorer").Bold().DragWindow()
		a.deviceMenu(c)
		ui.Spacer(c).DragWindow()
		lang := "EN"
		if a.lang == "en" {
			lang = "中文"
		}
		if ui.Button(c, lang).Clicked() {
			if a.lang == "en" {
				a.lang = "zh"
			} else {
				a.lang = "en"
			}
			a.saveSettings()
		}
	})
}

func (a *app) rail(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(76).FillHeight().Padding(8, 6).Gap(2).
		Background(ui.Hex("#fbfbfb")).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
		for _, item := range []struct{ id, label string }{
			{pageExplorer, "Files"},
			{pageDevices, "Devices"},
			{pageOps, "Transfers"},
			{pageApps, "Apps"},
			{pageLog, "Log"},
			{pageSettings, "Settings"},
		} {
			b := ui.Button(c.Key("page-"+item.id), "")
			b.FillWidth()
			if a.page == item.id {
				b.Background(t.SurfacePressed).TextColor(t.Accent)
			}
			id, label := item.id, item.label
			b.Children(func() {
				ui.Column(c).AlignItems(ui.Center).Gap(2).Children(func() {
					if glyph := railGlyphs[id]; glyph != nil {
						ui.Icon(c, glyph).Size(16, 16)
					}
					ui.Text(c, a.tr(label)).FontSize(11)
				})
			})
			if b.Clicked() {
				a.page = id
				if id == pageApps && len(a.apps) == 0 && a.pkgErr == "" {
					a.loadPackages()
				}
			}
		}
	})
}

func (a *app) commandBar(c *ui.Context) {
	t := c.Theme()
	p := a.focus()
	rows := a.visibleOf(p)
	sel := selectedOf(p, rows)
	busy := a.serial == "" || p.loading
	ui.Row(c).Height(44).Padding(4, 8).Gap(4).AlignItems(ui.Center).
		Background(ui.Hex("#ffffff")).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if ui.PrimaryButton(c, a.tr("Upload")).Disabled(busy).Clicked() {
			a.pickUpload(false)
		}
		if ui.Button(c, a.tr("Upload folder")).Disabled(busy).Clicked() {
			a.pickUpload(true)
		}
		if ui.Button(c, a.tr("Download")).Disabled(len(sel) == 0).Clicked() {
			a.pull(sel)
		}
		if ui.Button(c, a.tr("Paste")).Disabled(busy).Clicked() {
			a.pasteClipboard()
		}
		if ui.Button(c, a.tr("New folder")).Disabled(busy).Clicked() {
			a.mkdir()
		}
		if ui.Button(c, a.tr("Rename")).Disabled(len(sel) != 1).Clicked() {
			a.ask(a.tr("Rename"), sel[0].Name, "rename:"+sel[0].Name)
		}
		if ui.Button(c, a.tr("Delete")).Disabled(len(sel) == 0).Clicked() {
			a.asking = true
		}
		if ui.Button(c, a.tr("Details")).Disabled(len(sel) != 1).Clicked() {
			a.showDetail()
		}
		ui.Spacer(c)
		if ui.Button(c, a.tr("Refresh")).Disabled(busy).Clicked() {
			a.loadPane(p, p.path)
		}
		ui.MenuButton(c, a.tr("More"), func(m *ui.Menu) {
			split := len(a.tab().panes) == 2
			if m.Item(a.tr("Copy to other")).Disabled(!split || len(sel) == 0).Chosen() {
				a.copyTo(false)
			}
			if m.Item(a.tr("Move")).Disabled(!split || len(sel) == 0).Chosen() {
				a.copyTo(true)
			}
			m.Separator()
			if m.Item(a.tr("Permissions")).Disabled(len(sel) != 1).Chosen() {
				a.ask(a.tr("Permissions"), "644", "chmod")
			}
			if m.Item(a.tr("Owner")).Disabled(len(sel) != 1).Chosen() {
				a.ask(a.tr("Owner"), "root", "chown")
			}
			if m.Item(a.tr("Unzip")).Disabled(len(sel) != 1).Chosen() {
				a.extract()
			}
			if m.Item(a.tr("Zip")).Disabled(len(sel) == 0).Chosen() {
				a.pack()
			}
			if m.Item(a.tr("Grep")).Disabled(a.serial == "").Chosen() {
				a.searchContent()
			}
		})
	})
}

func (a *app) deviceMenu(c *ui.Context) {
	label := a.tr("No device")
	if d, ok := a.device(); ok {
		label = d.Label()
	}
	ui.MenuButton(c, label, func(m *ui.Menu) {
		if len(a.devices) == 0 {
			m.Item(a.tr("No devices")).Disabled(true)
		}
		for _, d := range a.devices {
			item := m.Item(d.Label() + "  " + d.State)
			if !d.Ready() {
				item.Disabled(true)
			}
			if d.Serial == a.serial {
				item.Checked(true)
			}
			if item.Chosen() {
				a.setSerial(d.Serial)
				a.page = pageExplorer
				a.load("/sdcard")
			}
		}
		m.Separator()
		if m.Item(a.tr("Refresh")).Chosen() {
			go a.refreshDevices()
		}
	})
}

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(220).FillHeight().Padding(8).Gap(2).Background(t.Surface).Children(func() {
		ui.Text(c, a.tr("Locations")).FontSize(11).TextColor(t.TextMuted).Padding(6, 8)
		for _, loc := range a.locations() {
			row := ui.Row(c.Key("loc-"+loc.Path)).Padding(6, 8).Radius(6).Children(func() {
				ui.Text(c, loc.Label).SingleLine()
			})
			if a.focus().path == loc.Path {
				row.Background(t.SurfacePressed)
			} else if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			if row.Clicked() && a.serial != "" {
				a.load(loc.Path)
			}
		}
		ui.Text(c, a.tr("Folders")).FontSize(11).TextColor(t.TextMuted).Padding(10, 8, 4, 8)
		a.treeView(c)
	})
}

func (a *app) explorer(c *ui.Context) {
	a.syncDetail()
	ui.Column(c).Fill().Clip().Children(func() {
		a.tabStrip(c)
		a.commandBar(c)
		ui.Row(c).Grow(1).Clip().Children(func() {
			tab := a.tab()
			if len(tab.panes) == 2 {
				ui.Split(c, &a.split, func() { a.paneView(c, tab, 0) }, func() { a.paneView(c, tab, 1) }).Grow(1)
			} else {
				ui.Column(c).Grow(1).FillHeight().Clip().Children(func() { a.paneView(c, tab, 0) })
			}
			if a.detailOpen {
				a.detailPanel(c)
			}
		})
	})
}

func (a *app) tabStrip(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(4, 8).Gap(4).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		for i, tab := range a.tabs {
			label := tab.titleText()
			if len(tab.panes) == 2 {
				label += "  ‖"
			}
			b := ui.Button(c.Key(fmt.Sprintf("tab-%d", i)), label)
			if i == a.tabIndex {
				b.Background(t.SurfacePressed)
			}
			if b.Clicked() {
				a.tabIndex = i
			}
			if len(a.tabs) > 1 && ui.Button(c.Key(fmt.Sprintf("tab-close-%d", i)), "×").Width(28).Clicked() {
				a.closeTab(i)
			}
		}
		ui.Spacer(c)
		if ui.Button(c, a.tr("New tab")).Clicked() {
			a.newTab()
		}
		label := a.tr("Split")
		if len(a.tab().panes) == 2 {
			label = a.tr("Unsplit")
		}
		if ui.Button(c, label).Clicked() {
			a.splitTab()
		}
	})
}

func (a *app) paneView(c *ui.Context, tab *tab, index int) {
	p := tab.panes[index]
	t := c.Theme()
	active := tab.active == index
	col := ui.Column(c.Key(fmt.Sprintf("pane-%d-%d", a.tabIndex, index))).Fill()
	if got, ok := ui.Drop[paneDrag](col); ok && got.from != index && len(tab.panes) == 2 {
		tab.active = got.from
		a.copyTo(false)
	}
	if col.FileDragOver() {
		col.Background(t.SurfaceHover)
	}
	if files := col.DroppedFiles(); len(files) > 0 && a.serial != "" {
		tab.active = index
		for _, f := range files {
			a.queueUpload(f)
		}
	}
	col.Children(func() {
		head := ui.Row(c).Height(40).Padding(4, 8).Gap(6).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, a.tr("Up")).Disabled(p.parent == "" || p.loading).Clicked() {
				tab.active = index
				a.loadPane(p, p.parent)
			}
			if ui.Button(c, a.tr("Refresh")).Disabled(a.serial == "" || p.loading).Clicked() {
				tab.active = index
				a.loadPane(p, p.path)
			}
			a.paneCrumbs(c, p, index)
			ui.SearchField(c, &p.query).Width(180)
			if ui.Button(c, a.tr("Find")).Disabled(a.serial == "" || p.loading).Clicked() {
				tab.active = index
				a.searchDeep()
			}
			mode := a.tr("Icons")
			if p.icons {
				mode = a.tr("List")
			}
			if ui.Button(c, mode).Clicked() {
				tab.active = index
				p.icons = !p.icons
			}
		})
		if active {
			head.Background(t.SurfaceHover)
		}
		if head.Clicked() {
			tab.active = index
		}
		if p.loading {
			ui.Row(c).Padding(0, 8).Children(func() { ui.Spinner(c) })
		}
		if a.serial == "" {
			a.empty(c, a.tr("Connect a device"), a.connErr)
			return
		}
		if p.listErr != "" {
			a.empty(c, a.trf("Cannot open %s", p.path), p.listErr)
			return
		}
		if p.icons {
			a.iconGrid(c, p, index)
		} else {
			a.table(c, p, index)
		}
	})
}

func (a *app) table(c *ui.Context, p *pane, index int) {
	rows := a.visibleOf(p)
	cols := []ui.TableColumn{
		{Title: a.tr("Name"), ID: "name", Sortable: true},
		{Title: a.tr("Type"), ID: "type", Width: 110, Sortable: true},
		{Title: a.tr("Size"), ID: "size", Width: 110, Align: ui.End, Sortable: true},
		{Title: a.tr("Modified"), ID: "modified", Width: 170, Sortable: true},
	}
	p.list.Key = func(i int) any { return rows[i].Name }
	p.list.Label = func(i int) string { return rows[i].Name }
	p.list.Selection = &p.chosen
	p.list.Selected = &p.picked
	p.list.Sort = &p.sort
	table := ui.Table(c.Key(fmt.Sprintf("files-%d", index)), &p.list, cols, len(rows), func(row, col int) {
		e := rows[row]
		switch col {
		case 0:
			// Drag makes this cell the press target, so the row behind it
			// never sees the click. Select and open here.
			name := ui.Row(c).AlignItems(ui.Center).Gap(6).Padding(0, 8).Children(func() {
				a.fileIcon(c, p, e, 18)
				ui.Text(c, e.Name).SingleLine()
			}).Drag(paneDrag{from: index})
			if name.DoubleClicked() {
				a.openListed(p, index, rows, row)
			} else if name.Clicked() {
				a.chooseListed(p, index, rows, row, name.ClickModifiers())
			}
		case 1:
			ui.Text(c, a.tr(kindOf(e))).SingleLine().Padding(4, 8)
		case 2:
			ui.Text(c, sizeOf(e)).SingleLine().Padding(4, 8)
		case 3:
			ui.Text(c, e.MTime.Format("2006-01-02 15:04")).SingleLine().Padding(4, 8)
		}
	}).Grow(1).ContextMenu(func(m *ui.Menu) {
		a.tab().active = index
		sel := selectedOf(p, rows)
		openRow := -1
		if len(sel) == 1 {
			for i := range rows {
				if rows[i].Name == sel[0].Name {
					openRow = i
					break
				}
			}
		}
		canOpen := openRow >= 0 && (rows[openRow].IsDir() || canViewLarge(previewKind(rows[openRow])))
		if m.Item(a.tr("Open")).Disabled(!canOpen).Chosen() {
			a.openListed(p, index, rows, openRow)
		}
		if m.Item(a.tr("Download")).Disabled(len(sel) == 0).Chosen() {
			a.pull(sel)
		}
		if m.Item(a.tr("Rename")).Disabled(len(sel) != 1).Chosen() {
			a.ask(a.tr("Rename"), sel[0].Name, "rename:"+sel[0].Name)
		}
		if m.Item(a.tr("Copy path")).Disabled(len(sel) != 1).Chosen() {
			mygo.Clipboard.WriteText(joinPath(p.path, sel[0].Name))
			a.status = a.tr("Path copied")
		}
		if m.Item(a.tr("Paste")).Disabled(a.serial == "" || p.loading).Chosen() {
			a.pasteClipboard()
		}
		m.Separator()
		if m.Item(a.tr("Delete")).Disabled(len(sel) == 0).Chosen() {
			a.asking = true
		}
	})
	if table.Submitted() && p.picked >= 0 && p.picked < len(rows) {
		a.openListed(p, index, rows, p.picked)
	}
	if len(rows) == 0 && !p.loading {
		ui.Text(c, a.tr("Empty folder")).TextColor(c.Theme().TextMuted).Padding(16)
	}
}

func (a *app) chooseListed(p *pane, index int, rows []adb.Entry, row int, mods ui.Modifiers) {
	if row < 0 || row >= len(rows) {
		return
	}
	a.tab().active = index
	name := rows[row].Name
	switch {
	case mods&ui.Cmd != 0:
		if p.chosen.Has(name) {
			p.chosen.Remove(name)
		} else {
			p.chosen.Add(name)
		}
	case mods&ui.Shift != 0 && p.picked >= 0 && p.picked < len(rows):
		lo, hi := p.picked, row
		if lo > hi {
			lo, hi = hi, lo
		}
		p.chosen.Clear()
		for i := lo; i <= hi; i++ {
			p.chosen.Add(rows[i].Name)
		}
	default:
		p.chosen.Clear()
		p.chosen.Add(name)
	}
	p.picked = row
}

func (a *app) openListed(p *pane, index int, rows []adb.Entry, row int) {
	if row < 0 || row >= len(rows) {
		return
	}
	a.tab().active = index
	p.picked = row
	e := rows[row]
	switch {
	case e.IsDir():
		a.loadPane(p, joinPath(p.path, e.Name))
	case isArchive(e.Name) && !p.deep:
		a.openArchive(p, e.Name)
	case canViewLarge(previewKind(e)):
		a.openViewer(e.Name)
	default:
		a.showDetail()
	}
}

func (a *app) devicesPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(16).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("Devices")).FontSize(20).Bold().Grow(1)
			if ui.Button(c, a.tr("Refresh")).Clicked() {
				go a.refreshDevices()
			}
		})
		if a.connErr != "" {
			ui.Text(c, a.connErr).TextColor(t.Danger)
			ui.Text(c, a.tr("Start the server with adb start-server, then refresh.")).TextColor(t.TextMuted)
		}
		if len(a.devices) == 0 && a.connErr == "" {
			ui.Text(c, a.tr("No devices yet.")).TextColor(t.TextMuted)
		}
		if a.serial != "" {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, a.tr("Storage and battery")).Bold().Grow(1)
				if ui.Button(c, a.tr("Read")).Clicked() {
					a.loadDeviceInfo()
				}
			})
			if a.deviceInfo != "" {
				ui.Text(c, a.deviceInfo).FontSize(12)
			}
		}
		for _, d := range a.devices {
			row := ui.Row(c.Key("dev-"+d.Serial)).Padding(10, 12).Radius(8).Gap(12).
				AlignItems(ui.Center).Border(1, t.Border).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, d.Label()).Bold()
					ui.Text(c, d.Serial).FontSize(12).TextColor(t.TextMuted)
				})
				ui.Text(c, d.State).TextColor(t.TextMuted)
				if ui.Button(c, a.tr("Browse")).Disabled(!d.Ready()).Clicked() {
					a.setSerial(d.Serial)
					a.page = pageExplorer
					a.load("/sdcard")
				}
			})
			_ = row
		}
	})
}

func (a *app) paneCrumbs(c *ui.Context, p *pane, index int) {
	t := c.Theme()
	label := p.path
	if label == "" {
		label = "/"
	}
	ui.Text(c, label).Grow(1).SingleLine().TextColor(t.TextMuted)
}

func (a *app) namePrompt(c *ui.Context) {
	if a.promptFor == "" {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Text(c, a.promptTitle).Bold()
		if ui.TextInput(c, &a.promptValue).AutoFocus().Submitted() {
			a.acceptPrompt()
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, a.tr("Cancel")).Clicked() {
				a.promptFor = ""
			}
			if ui.PrimaryButton(c, a.tr("OK")).Clicked() {
				a.acceptPrompt()
			}
		})
	})
}

// openUploadDialog is the system file picker. Tests replace it.
var openUploadDialog = func(opts mygo.OpenDialogOptions) ([]string, error) {
	return mygo.Dialog.Open(opts)
}

// openDownloadDialog asks for the local directory a download is saved into.
var openDownloadDialog = func(opts mygo.OpenDialogOptions) ([]string, error) {
	return mygo.Dialog.Open(opts)
}

func (a *app) pickUpload(dirs bool) {
	if a.serial == "" {
		a.status = a.tr("No device")
		return
	}
	title := a.trf("Upload to %s", a.focus().path)
	if dirs {
		a.status = a.tr("Choose a folder")
	} else {
		a.status = a.tr("Choose files")
	}
	opts := mygo.OpenDialogOptions{
		Parent:      a.win,
		Title:       title,
		Multiple:    true,
		Directory:   dirs,
		DefaultPath: a.lastWindowsDir(true),
	}
	finish := func(paths []string, err error) {
		adb.UploadLog("dialog dirs=%t err=%v paths=%d", dirs, err, len(paths))
		if err != nil {
			a.status = err.Error()
			return
		}
		if len(paths) == 0 {
			a.status = a.tr("Skipped")
			return
		}
		a.rememberWindowsDir(true, paths[0])
		for _, p := range paths {
			a.queueUpload(p)
		}
	}
	win := a.win
	if win == nil {
		paths, err := openUploadDialog(opts)
		finish(paths, err)
		return
	}
	go func() {
		paths, err := openUploadDialog(opts)
		win.Update(func() { finish(paths, err) })
	}()
}

func (a *app) terminalPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(16).Gap(8).Children(func() {
		ui.Text(c, a.tr("Terminal")).FontSize(20).Bold()
		ui.Text(c, a.tr("Runs on the device through adb shell.")).FontSize(12).TextColor(t.TextMuted)
		ui.Scroll(c).Grow(1).Background(t.Surface).Radius(8).Padding(10).Children(func() {
			text := a.termOutput
			if text == "" {
				text = a.tr("No commands yet.")
			}
			ui.Text(c, text).FontSize(12)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.termInput).Grow(1).Placeholder("ls -la").Label(a.tr("Command"))
			if ui.PrimaryButton(c, a.tr("Run")).Disabled(a.serial == "" || strings.TrimSpace(a.termInput) == "").Clicked() {
				cmd := a.termInput
				a.termInput = ""
				a.runShell(cmd)
			}
		})
	})
}

func (a *app) opsPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(16).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("Transfers")).FontSize(20).Bold().Grow(1)
			if ui.Button(c, a.tr("Clear finished")).Disabled(len(a.ops) == 0).Clicked() {
				a.clearFinished()
			}
		})
		if len(a.ops) == 0 {
			ui.Text(c, a.tr("Downloads and uploads show up here.")).TextColor(t.TextMuted)
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(8).Children(func() {
				for i := len(a.ops) - 1; i >= 0; i-- {
					item := a.ops[i]
					ui.Column(c.Key(fmt.Sprintf("op-%d", item.id))).Padding(10, 12).Radius(8).Gap(6).
						Border(1, t.Border).Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
							ui.Text(c, a.opTitle(item)).Bold().Grow(1).SingleLine()
							ui.Text(c, a.opState(item.state)).FontSize(12).TextColor(t.TextMuted)
							busy := item.state == opQueued || item.state == opRunning
							if ui.Button(c, a.tr("Cancel")).Disabled(!busy).Clicked() {
								a.cancelOp(item.id)
							}
						})
						if item.file != "" && item.file != item.label {
							line := item.file
							if item.count > 1 && item.index > 0 {
								line = fmt.Sprintf("%d/%d  %s", item.index, item.count, item.file)
							}
							ui.Text(c, line).SingleLine()
						}
						if item.total > 0 {
							ui.Meter(c, float64(item.done), 0, float64(item.total), nil).Label(item.label)
							pct := int(item.progress()*100 + 0.5)
							if pct > 100 {
								pct = 100
							}
							meta := fmt.Sprintf("%d%%  %s / %s", pct, sizeBytes(item.done), sizeBytes(item.total))
							if item.speed > 0 {
								meta += "  " + speedText(item.speed)
							}
							ui.Text(c, meta).FontSize(12).TextColor(t.TextMuted)
						} else if item.speed > 0 {
							ui.Text(c, speedText(item.speed)).FontSize(12).TextColor(t.TextMuted)
						}
						shown := len(item.names)
						if shown > 8 {
							shown = 8
						}
						for n := 0; n < shown; n++ {
							ui.Text(c, item.names[n]).FontSize(12).SingleLine().TextColor(t.TextMuted)
						}
						if extra := len(item.names) - shown; extra > 0 {
							ui.Text(c, a.trf("%d more", extra)).FontSize(12).TextColor(t.TextMuted)
						}
						if item.detail != "" {
							ui.Text(c, item.detail).FontSize(12).TextColor(t.Danger)
						}
					})
				}
			})
		})
	})
}

func (a *app) logPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(16).Gap(8).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, a.tr("Activity")).FontSize(20).Bold().Grow(1)
			if ui.Button(c, a.tr("Clear")).Disabled(len(a.logLines) == 0).Clicked() {
				a.logLines = nil
			}
		})
		if len(a.logLines) == 0 {
			ui.Text(c, a.tr("Transfers and errors show up here.")).TextColor(t.TextMuted)
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				for _, line := range a.logLines {
					ui.Text(c, line).FontSize(12)
				}
			})
		})
	})
}

func (a *app) opTitle(item *op) string {
	kind := item.kind
	switch item.kind {
	case "download":
		kind = a.tr("Download")
	case "upload":
		kind = a.tr("Upload")
	}
	return kind + "  " + item.label
}

func (a *app) opState(state string) string {
	switch state {
	case opQueued:
		return a.tr("Queued")
	case opRunning:
		return a.tr("Running")
	case opDone:
		return a.tr("Done")
	case opFailed:
		return a.tr("Failed")
	case opCancel:
		return a.tr("Cancelled")
	default:
		return state
	}
}

func speedText(n int64) string {
	return sizeBytes(n) + "/s"
}

func operationMessage(s string) bool {
	return strings.Contains(s, "正在") || strings.Contains(s, "失败") || strings.Contains(s, "已满") || strings.Contains(s, "已停止")
}

func (a *app) statusbar(c *ui.Context) {
	t := c.Theme()
	p := a.focus()
	rows := a.visibleOf(p)
	sel := len(selectedOf(p, rows))
	text := a.status
	if cur := a.activeOp(); cur != nil {
		verb := a.tr("Downloading %s")
		if cur.kind == "upload" {
			verb = a.tr("Uploading %s")
		}
		name := cur.label
		if cur.file != "" {
			name = cur.file
		}
		text = fmt.Sprintf(verb, name)
		if cur.count > 1 && cur.index > 0 {
			text += fmt.Sprintf("  %d/%d", cur.index, cur.count)
		}
		if cur.total > 0 {
			text += fmt.Sprintf("  %.0f%%", cur.progress()*100)
		}
		if cur.speed > 0 {
			text += "  " + speedText(cur.speed)
		}
	} else if operationMessage(a.status) {
		text = a.status
	} else if a.page == pageExplorer && a.serial != "" && !p.loading && p.listErr == "" {
		switch {
		case sel > 0 && len(a.tab().panes) == 2:
			text = a.trf("%d items · %d selected · pane %d", len(rows), sel, a.tab().active+1)
		case sel > 0:
			text = a.trf("%d items · %d selected", len(rows), sel)
		case len(a.tab().panes) == 2:
			text = a.trf("%d items · pane %d", len(rows), a.tab().active+1)
		default:
			text = a.trf("%d items", len(rows))
		}
	}
	ui.Row(c).Height(28).Padding(0, 12).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, text).FontSize(11).TextColor(t.TextMuted).SingleLine()
	})
}

func (a *app) empty(c *ui.Context, title, detail string) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(6).Children(func() {
		ui.Text(c, title).FontSize(18).Bold()
		if detail != "" {
			ui.Text(c, detail).TextColor(t.TextMuted)
		}
	})
}

func (a *app) detailPanel(c *ui.Context) {
	t := c.Theme()
	d := a.detail
	ui.Column(c).Width(360).FillHeight().Padding(12).Gap(6).Background(t.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Clip().Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, a.tr("Details")).Bold().Grow(1)
			if d.canEdit && ui.Button(c, a.tr("Open in editor")).Clicked() {
				a.openEditor()
			}
			if canViewLarge(d.kind) && ui.Button(c, a.tr("View large")).Clicked() {
				a.openViewer(d.name)
			}
			if ui.Button(c, a.tr("Close")).Clicked() {
				a.detailOpen = false
			}
		})
		ui.Text(c, d.name).Bold().SingleLine()
		ui.Text(c, d.path).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, a.tr(d.kind)+" · "+d.size)
		ui.Text(c, d.mod).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, d.mode).FontSize(12)
		if d.err != "" {
			ui.Text(c, d.err).FontSize(12).TextColor(t.Danger)
		}
		if d.caption != "" {
			ui.Text(c, d.caption).FontSize(12).TextColor(t.TextMuted)
		}
		if (d.kind == kindText || d.kind == kindBinary || d.kind == kindSVG) && len(d.bytes) > 0 {
			label := a.tr("Hex")
			if d.hexMode {
				label = a.tr("Text")
			}
			if ui.Button(c, label).Clicked() {
				a.toggleHex()
			}
		}
		if d.pdfNeed {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.detail.password).Grow(1).Label(a.tr("Password"))
				if ui.Button(c, a.tr("Unlock")).Clicked() {
					a.retryPDF()
				}
			})
		}
		if d.loading {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, a.tr("Loading preview…")).FontSize(12).TextColor(t.TextMuted)
			})
		}
		ui.Scroll(c).Grow(1).Clip().Children(func() {
			ui.Column(c).Gap(8).Children(func() {
				if d.svg != nil && !d.hexMode {
					if d.svgOwn {
						ui.Image(c, d.svg)
					} else {
						ui.Icon(c, d.svg).Size(120, 120).TextColor(t.Accent)
					}
				}
				for i, bmp := range d.bitmaps {
					if bmp == nil {
						continue
					}
					w, h := bmp.Size()
					dw := float32(336)
					if d.kind == kindAPK {
						dw = 96
					}
					dh := float32(h)
					if w > 0 {
						dh = float32(h) * dw / float32(w)
					}
					ui.Image(c.Key(fmt.Sprintf("preview-bmp-%d", i)), bmp).Size(dw, dh)
				}
				if d.edit != nil && !d.hexMode {
					editor.View(c, d.edit).Grow(1).MinHeight(200)
				} else if d.preview != "" {
					txt := ui.Text(c, d.preview).FontSize(12)
					if d.hexMode || d.kind == kindBinary {
						txt.Font("monospace")
					}
				}
			})
		})
	})
}

func (a *app) pickInstall() {
	win := a.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:  win,
			Title:   a.tr("Install apk"),
			Filters: []mygo.FileFilter{{Name: a.tr("Android package"), Extensions: []string{"apk"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		win.Update(func() { a.installAPK(paths[0]) })
	}()
}

func (a *app) editOverlay(c *ui.Context) {
	if editMode == "" {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, editPath).Bold().Grow(1).SingleLine()
			if editMode == "text" && ui.Button(c, a.tr("Save")).Clicked() {
				a.saveEditor()
			}
			if ui.Button(c, a.tr("Close")).Clicked() {
				a.closeEditor()
			}
		})
		if editMode == "text" && editState != nil {
			editor.View(c, editState).Grow(1).MinHeight(360)
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(360).Children(func() {
			ui.Text(c, editHex).FontSize(12)
		})
	})
}

func (a *app) downloadConflict(c *ui.Context) {
	ask := a.downloadAsk
	if ask == nil {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Text(c, a.trf("%s already exists", ask.name)).Bold()
		ui.Text(c, a.trf("Save as %s", ask.next)).FontSize(12).TextColor(c.Theme().TextMuted)
		if ask.rest {
			ui.Text(c, a.tr("Use this choice for the remaining files")).FontSize(12).TextColor(c.Theme().TextMuted)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, a.tr("Skip")).Clicked() {
				a.chooseDownload(dlSkip)
			}
			if ui.Button(c, a.tr("Overwrite")).Clicked() {
				a.chooseDownload(dlOverwrite)
			}
			if ui.PrimaryButton(c, a.tr("Rename")).Label(a.tr("Rename download")).Clicked() {
				a.chooseDownload(dlRename)
			}
		})
	})
}

func (a *app) conflictPrompt(c *ui.Context) {
	name := a.conflictName()
	if name == "" {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Text(c, a.trf("%s already exists", name)).Bold()
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, a.tr("Skip")).Clicked() {
				a.resolveConflict(false)
			}
			if ui.PrimaryButton(c, a.tr("Replace")).Clicked() {
				a.resolveConflict(true)
			}
		})
	})
}

func (a *app) treeView(c *ui.Context) {
	t := c.Theme()
	if treeRoot == nil && a.serial != "" {
		a.treeAt("/")
	}
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Gap(1).Children(func() {
			for _, row := range treeRows() {
				r := row
				pad := float32(8 + r.Depth*12)
				line := ui.Row(c.Key("tree-"+r.Path)).Padding(3, 8, 3, pad).Radius(6).Children(func() {
					mark := "> "
					if r.Open {
						mark = "v "
					}
					if r.Loading {
						mark = "... "
					}
					ui.Text(c, mark+r.Name).SingleLine().FontSize(12)
				})
				if a.focus().path == r.Path {
					line.Background(t.SurfacePressed)
				} else if line.Hovered() {
					line.Background(t.SurfaceHover)
				}
				if line.Clicked() && a.serial != "" {
					a.toggleTree(r.Path)
					a.load(r.Path)
				}
			}
		})
	})
}

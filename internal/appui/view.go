package appui

import (
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oldj/switchhosts/internal/hostswrite"
	"github.com/oldj/switchhosts/internal/store"
)

const (
	leftSidebarWidth = 40
	topBarHeight     = 40
	statusBarHeight  = 22
	panelMinWidth    = 100
)

// View draws the main window: the title bar, the rail of views, the hosts
// list, the editor and the details panel.
func (a *App) View(c *ui.Context) {
	p := a.Palette()
	theme := themeFor(p)
	theme.FontSize = 14
	c.SetTheme(theme)

	bar := c.TitleBar()
	ui.Column(c).Fill().Background(p.WindowBG).Children(func() {
		a.topBar(c, p, bar)
		a.body(c, p)
	})
	a.overlays(c, p)
	a.runWindowAction()
	a.runPermissionExit()
}

// runWindowAction performs the window control the title bar pressed, now
// that the frame is built.
func (a *App) runWindowAction() {
	action := a.TakeWindowAction()
	if action == windowActionNone || a.host == nil {
		return
	}
	switch action {
	case windowActionMinimize:
		a.host.Minimize()
	case windowActionMaximize:
		a.host.ToggleMaximize()
	case windowActionClose:
		a.host.Close()
	}
}

// topBar draws the title bar with the brand, the panel toggles and the
// window controls the desktop's title bar would show.
func (a *App) topBar(c *ui.Context, p palette, bar ui.TitleBar) {
	height := float32(topBarHeight)
	if bar.Height > height {
		height = bar.Height
	}
	leftInset := bar.Left
	if leftInset < 0 {
		leftInset = 0
	}
	rightInset := bar.Right
	if rightInset < 0 {
		rightInset = 0
	}
	title := a.titleClusterText()
	showBrand := runtime.GOOS != "darwin"
	showControls := runtime.GOOS != "darwin"

	row := ui.Row(c).Height(height).FillWidth().AlignItems(ui.Center).
		Padding(0, 10+float32(rightInset), 0, float32(leftInset)).DragWindow()
	row.Children(func() {
		if showBrand {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Width(leftSidebarWidth).Height(height).Center().Children(func() {
					if img := logoImage(); img != nil {
						ui.Image(c, img).Size(20, 20).Radius(4)
					}
				})
				ui.Text(c, "SwitchHosts").FontSize(13).Bold().TextColor(p.Text)
			})
		}
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			svg := icon("LayoutSidebarLeftCollapse")
			if !a.leftOpen {
				svg = icon("LayoutSidebarLeftExpand")
			}
			if titleBarButton(c, p, svg, a.T("toggle_left_panel"), a.leftOpen).Clicked() {
				a.setLeftOpen(!a.leftOpen)
			}
			if titleBarButton(c, p, icon("Plus"), a.T("hosts_add"), false).Clicked() {
				a.openEditDialog(nil)
			}
		})

		ui.Spacer(c)

		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon(iconForType(a.currentKindName()))).Size(16, 16).
				TextColor(p.TextMuted)
			ui.Text(c, title).FontSize(14).MaxLines(1).TextColor(p.Text).
				MaxWidth(320)
			if a.IsReadOnly() {
				ui.Badge(c, a.T("read_only")).Background(p.BorderWeak).TextColor(p.TextMuted).
					FontSize(11)
			}
		})

		ui.Spacer(c)

		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			if !a.leftOpen {
				a.recoveryNotice(c, p, true)
			}
			svg := icon("LayoutSidebarRightExpand")
			if a.rightOpen {
				svg = icon("LayoutSidebarRightCollapse")
			}
			if titleBarButton(c, p, svg, a.T("toggle_right_panel"), a.rightOpen).Clicked() {
				a.setRightOpen(!a.rightOpen)
			}
			if showControls {
				ui.Box(c).Width(1).Height(18).Background(p.Border)
				if titleBarButton(c, p, icon("Minus"), a.T("minimize"), false).Clicked() {
					a.windowAction = windowActionMinimize
				}
				if titleBarButton(c, p, icon("Square"), a.T("maximize"), false).Clicked() {
					a.windowAction = windowActionMaximize
				}
				if titleBarButton(c, p, icon("X"), a.T("close_window"), false).Clicked() {
					a.windowAction = windowActionClose
				}
			}
		})
	})
}

// currentKindName returns "system" or the selected node's type.
func (a *App) currentKindName() string {
	node := a.Current()
	if node == nil {
		return "system"
	}
	return store.NodeType(node)
}

// titleClusterText is the title the title bar shows.
func (a *App) titleClusterText() string {
	node := a.Current()
	if node == nil {
		return a.T("system_hosts")
	}
	return a.NodeTitleText(node)
}

// titleBarButton draws a borderless icon button of the title bar.
func titleBarButton(c *ui.Context, p palette, svg *ui.SVG, label string, active bool) ui.Element {
	btn := ui.ButtonBase(c).Size(28, 28).Radius(4).Focusable().Label(label).
		Background(ui.Transparent)
	if active {
		btn.Background(p.AccentSoft)
	}
	btn.Children(func() {
		ui.Icon(c, svg).Size(18, 18).TextColor(p.Text)
	})
	if btn.Hovered() && !active {
		btn.Background(p.BorderWeak)
	}
	btn.Tooltip(label)
	return btn
}

func iconForType(kind string) string {
	switch kind {
	case "folder":
		return "Folder"
	case "remote":
		return "World"
	case "group":
		return "Stack2"
	case "system":
		return "DeviceDesktop"
	case "trashcan":
		return "Trash"
	default:
		return "FileText"
	}
}

// body lays out the rail, the panels and the editor.
func (a *App) body(c *ui.Context, p palette) {
	ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
		a.leftSidebar(c, p)

		if a.leftOpen {
			panel := ui.Box(c).Width(a.leftWidth).Shrink(0).Children(func() {
				a.leftPanel(c, p)
			})
			a.panelSplitter(c, p, panel, &a.leftWidth, true)
		}

		ui.Box(c).Grow(1).Shrink(1).Children(func() {
			a.mainPanel(c, p)
		})

		if a.rightOpen {
			a.splitHandle(c, p, false)
			ui.Box(c).Width(a.rightWidth).Shrink(0).Children(func() {
				a.rightPanel(c, p)
			})
		}
	})
}

// leftSidebar is the narrow rail of views on the left edge.
func (a *App) leftSidebar(c *ui.Context, p palette) {
	ui.Column(c).Width(leftSidebarWidth).FillHeight().Padding(0, 0, 24, 0).
		AlignItems(ui.Center).Gap(20).Children(func() {
		ui.Box(c).Height(8)
		if railButton(c, p, icon("List"), a.leftView == 0, a.T("hosts")).Clicked() {
			a.toggleLeftView(0)
		}
		if railButton(c, p, icon("Search"), false, a.T("search")).Clicked() {
			a.openFind()
		}
		if railButton(c, p, icon("History"), false, a.T("show_history")).Clicked() {
			a.openHistory()
		}
		ui.Spacer(c)
		if railButton(c, p, icon("Trash"), a.leftView == 1, a.T("trashcan")).Clicked() {
			a.toggleLeftView(1)
		}
		if railButton(c, p, icon("Settings"), false, a.T("settings")).Clicked() {
			a.openPreferences()
		}
	})
}

func railButton(c *ui.Context, p palette, svg *ui.SVG, active bool, label string) ui.Element {
	btn := ui.ButtonBase(c).Size(28, 28).Radius(4).Focusable().Label(label).
		Background(ui.Transparent)
	if active {
		btn.Background(p.AccentSoft)
	}
	btn.Children(func() {
		ui.Icon(c, svg).Size(18, 18).TextColor(p.Text)
	})
	if btn.Hovered() && !active {
		btn.Background(p.BorderWeak)
	}
	btn.Tooltip(label)
	return btn
}

func (a *App) toggleLeftView(view int) {
	if !a.leftOpen {
		a.setLeftOpen(true)
		a.leftView = view
		return
	}
	if a.leftView == view {
		a.setLeftOpen(false)
		return
	}
	a.leftView = view
}

func (a *App) setLeftOpen(open bool) {
	a.leftOpen = open
	_ = a.UpdateConfig(map[string]any{"left_panel_show": open})
}

func (a *App) setRightOpen(open bool) {
	a.rightOpen = open
	_ = a.UpdateConfig(map[string]any{"right_panel_show": open})
}

// splitHandle draws a draggable edge beside a panel.
func (a *App) splitHandle(c *ui.Context, p palette, left bool) {
	handle := ui.Box(c).Width(4).FillHeight().Cursor(ui.CursorResizeE)
	if handle.Hovered() {
		handle.Background(p.Border)
	}
}

// panelSplitter draws the edge of the left panel and resizes it as the
// pointer drags.
func (a *App) panelSplitter(c *ui.Context, p palette, panel ui.Element, width *float32, left bool) {
	handle := ui.Box(c).Width(4).FillHeight().Cursor(ui.CursorResizeE)
	if handle.Hovered() {
		handle.Background(p.Border)
	}
	if dx, _, ok := handle.Dragged(); ok && dx != 0 {
		next := *width + dx
		if next < panelMinWidth {
			next = panelMinWidth
		}
		if windowWidth, _ := c.Size(); next > windowWidth*0.5 {
			next = windowWidth * 0.5
		}
		*width = next
	}
	if _, _, ok := handle.DragEnded(); ok {
		key := "left_panel_width"
		if !left {
			key = "right_panel_width"
		}
		_ = a.UpdateConfig(map[string]any{key: int(*width)})
	}
	_ = panel
}

// Window actions the title bar's own buttons report.
const (
	windowActionNone = iota
	windowActionMinimize
	windowActionMaximize
	windowActionClose
)

// TakeWindowAction consumes a window control the title bar pressed.
func (a *App) TakeWindowAction() int {
	action := a.windowAction
	a.windowAction = windowActionNone
	return action
}

// leftPanel is the hosts list or the trashcan.
func (a *App) leftPanel(c *ui.Context, p palette) {
	ui.Column(c).Fill().Background(p.PanelBG).Border(1, p.Border).Children(func() {
		if a.leftView == 0 {
			a.hostsList(c, p)
		} else {
			a.trashcanList(c, p)
		}
		ui.Box(c).Height(statusBarHeight).FillWidth()
	})
}

// hostsList draws the entries, the folders and the system hosts.
func (a *App) hostsList(c *ui.Context, p palette) {
	ui.Scroll(c).Grow(1).Padding(5, 10).Children(func() {
		a.recoveryNotice(c, p, false)
		a.listRow(c, p, nil, -1, false)
		a.nodesView(c, p, a.list, 0)
	})
}

func (a *App) nodesView(c *ui.Context, p palette, nodes []store.NodeJSON, depth int) {
	for _, node := range nodes {
		node := node
		kind := store.NodeType(node)
		if kind == "folder" {
			collapsed, _ := node["is_collapsed"].(bool)
			a.listRow(c, p, node, depth, collapsed)
			if !collapsed {
				a.nodesView(c, p, store.Children(node), depth+1)
			}
			continue
		}
		a.listRow(c, p, node, depth, false)
	}
}

// listRow draws one row of the list: the system hosts when node is nil.
func (a *App) listRow(c *ui.Context, p palette, node store.NodeJSON, depth int, collapsed bool) {
	id := "0"
	title := a.T("system_hosts")
	kind := "system"
	on := false
	if node != nil {
		id = store.NodeID(node)
		title = a.NodeTitleText(node)
		kind = store.NodeType(node)
		on = store.NodeOn(node)
	}
	selected := a.currentID == id

	row := ui.Row(c).FillWidth().Height(28).Radius(4).Gap(6).AlignItems(ui.Center).
		Padding(0, 6).Focusable()
	if depth > 0 {
		row.Padding(0, 6, 0, float32(6+depth*18))
	}
	if selected {
		row.Background(p.SelectedBG)
		row.DrawOver(func(painter *ui.Painter, r ui.Rect) {
			var indented ui.Path
			painter.FillPath(indented.MoveTo(r.X, r.Y+3).LineTo(r.X+3, r.Y+3).
				LineTo(r.X+3, r.Y+r.H-3).LineTo(r.X, r.Y+r.H-3), p.AccentIndicator)
		})
	}
	row.Children(func() {
		if kind == "folder" {
			arrow := icon("ChevronRight")
			if !collapsed {
				arrow = icon("ChevronDown")
			}
			ui.Icon(c, arrow).Size(12, 12).TextColor(p.TextMuted)
		} else {
			ui.Box(c).Width(12)
		}
		ui.Icon(c, icon(iconForType(kind))).Size(16, 16).TextColor(textColorFor(p, selected))
		ui.Text(c, title).Grow(1).MaxLines(1).TextColor(textColorFor(p, selected))
		if node != nil {
			if ui.Switch(c, &on).Tooltip(a.F("toggle_entry", title)).Changed() {
				a.toggleAsync(id, on)
			}
		}
	})
	switch {
	case row.RightClicked():
		if node != nil {
			a.openRowMenu(c, node)
		}
	case row.Clicked():
		if node != nil && kind == "folder" && row.ClickModifiers() == 0 {
			a.Select(id)
		} else if node != nil {
			a.Select(id)
		} else {
			a.Select("0")
		}
	}
}

func textColorFor(p palette, selected bool) ui.Color {
	if selected {
		return p.SelectedText
	}
	return p.Text
}

// openRowMenu shows the context menu of a list row: the actions the web
// interface's right-click menu offers.
func (a *App) openRowMenu(c *ui.Context, node store.NodeJSON) {
	id := store.NodeID(node)
	ui.Overlay(c, func() {
		ui.Column(c).Absolute().Left(float32(leftSidebarWidth)+a.leftWidth+40).Top(80).
			Width(180).Padding(4).Radius(6).Background(p_panelBg(a)).Border(1, p_border(a)).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.25)).Children(func() {
			if menuRow(c, a.T("edit")).Clicked() {
				a.openEditDialog(node)
			}
			if store.NodeType(node) == "remote" {
				if menuRow(c, a.T("refresh")).Clicked() {
					a.refreshAsync(id)
				}
			}
			if menuRow(c, a.T("move_to_trashcan")).Clicked() {
				a.trashAsync([]string{id})
			}
		})
	})
}

func menuRow(c *ui.Context, label string) ui.Element {
	return ui.ButtonBase(c).FillWidth().Height(26).Padding(0, 8).Radius(4).
		Children(func() {
			ui.Text(c, label).FontSize(13)
		})
}

func p_panelBg(a *App) ui.Color { return a.Palette().PanelBG }
func p_border(a *App) ui.Color  { return a.Palette().Border }

// trashcanList draws the entries in the trashcan.
func (a *App) trashcanList(c *ui.Context, p palette) {
	ui.Scroll(c).Grow(1).Padding(5, 10).Children(func() {
		if len(a.trashcan) == 0 {
			ui.Text(c, a.T("trashcan_empty")).Padding(12).TextColor(p.TextMuted)
			return
		}
		for _, item := range a.trashcan {
			item := item
			id := store.NodeID(item.Data)
			selected := a.currentID == id
			row := ui.Row(c).FillWidth().Height(28).Radius(4).Gap(6).AlignItems(ui.Center).
				Padding(0, 6).Focusable()
			if selected {
				row.Background(p.SelectedBG)
			}
			row.Children(func() {
				ui.Icon(c, icon(iconForType(store.NodeType(item.Data)))).Size(16, 16).
					TextColor(textColorFor(p, selected))
				ui.Text(c, a.NodeTitleText(item.Data)).Grow(1).MaxLines(1).
					TextColor(textColorFor(p, selected))
			})
			if row.Clicked() {
				a.Select(id)
			}
		}
	})
}

// mainPanel draws the editor and the status bar under it.
func (a *App) mainPanel(c *ui.Context, p palette) {
	ui.Column(c).Fill().Children(func() {
		readOnly := a.IsReadOnly()
		editor := ui.Box(c).Grow(1).FillWidth().Background(p.MainBG).
			Border(1, p.Border).Clip()
		if readOnly {
			editor.Background(p.ReadOnlyBG)
		}
		editor.Children(func() {
			a.editorView(c, p, readOnly)
		})
		a.statusBar(c, p)
	})
}

// statusBar shows the line and byte counts under the editor.
func (a *App) statusBar(c *ui.Context, p palette) {
	lines := 0
	if a.editorContent != "" {
		lines = strings.Count(a.editorContent, "\n")
		if !strings.HasSuffix(a.editorContent, "\n") {
			lines++
		}
	}
	label := itoa(int64(lines)) + " " + a.T("lines")
	if lines == 1 {
		label = itoa(int64(lines)) + " " + a.T("line")
	}
	ui.Row(c).Height(statusBarHeight).FillWidth().Padding(0, 10).Gap(16).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(10).TextColor(p.StatusText)
		ui.Text(c, prettyBytes(len(a.editorContent))).FontSize(10).TextColor(p.StatusText)
		if a.IsReadOnly() {
			ui.Text(c, a.T("read_only")).FontSize(10).TextColor(p.StatusText)
		}
	})
}

// rightPanel shows the details of the selected entry.
func (a *App) rightPanel(c *ui.Context, p palette) {
	ui.Column(c).Fill().Background(p.PanelBG).Border(1, p.Border).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			trashed := a.InTrashcan(a.currentID)
			if trashed {
				a.trashedDetails(c, p)
				return
			}
			node := a.Current()
			if node == nil {
				a.systemHostsDetails(c, p)
				return
			}
			a.nodeDetails(c, p, node)
		})
		ui.Box(c).Height(statusBarHeight).FillWidth()
	})
}

// trashedDetails shows a trashed entry, with the actions to restore it or
// delete it for good.
func (a *App) trashedDetails(c *ui.Context, p palette) {
	var node store.NodeJSON
	for _, item := range a.trashcan {
		if store.NodeID(item.Data) == a.currentID {
			node = item.Data
			break
		}
	}
	if node == nil {
		a.systemHostsDetails(c, p)
		return
	}
	kind := store.NodeType(node)
	ui.Column(c).Children(func() {
		a.detailsHeader(c, p, a.NodeTitleText(node), iconForType(kind), "")
		ui.Column(c).Padding(12).Gap(10).Children(func() {
			a.infoRow(c, p, a.T("hosts_type"), a.T(kind), false)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, a.T("restore")).Clicked() {
					a.restoreAsync(store.NodeID(node))
				}
				if ui.Button(c, a.T("delete")).Clicked() {
					a.deleteAsync(store.NodeID(node))
				}
			})
		})
	})
}

func (a *App) systemHostsDetails(c *ui.Context, p palette) {
	ui.Column(c).Children(func() {
		a.detailsHeader(c, p, a.T("system_hosts"), "DeviceDesktop", "")
		ui.Column(c).Padding(12).Gap(10).Children(func() {
			a.infoRow(c, p, a.T("hosts_type"), a.T("system"), false)
			path, _ := hostswrite.SystemHostsPath()
			a.infoRow(c, p, strings.TrimRight(a.T("your_hosts_file_is"), ":："), path, true)
			if content, _, err := hostswrite.ReadSystemHosts(); err == nil {
				a.infoRow(c, p, a.T("rules"), itoa(int64(hostswrite.CountRules(content))), false)
			}
			if ui.Button(c, a.T("show_history")).Clicked() {
				a.openHistory()
			}
		})
	})
}

func (a *App) nodeDetails(c *ui.Context, p palette, node store.NodeJSON) {
	kind := store.NodeType(node)
	id := store.NodeID(node)
	ui.Column(c).Children(func() {
		a.detailsHeader(c, p, a.NodeTitleText(node), iconForType(kind), a.T("edit"))
		ui.Column(c).Padding(12).Gap(10).Children(func() {
			a.infoRow(c, p, a.T("hosts_type"), a.T(kind), false)
			if kind == "local" || kind == "remote" {
				content, _ := store.ReadEntry(a.paths, id)
				a.infoRow(c, p, a.T("rules"), itoa(int64(hostswrite.CountRules(content))), false)
			}
			switch kind {
			case "remote":
				if source, _ := node["source"].(string); source == "domain" {
					a.infoRow(c, p, a.T("domain_list"), strings.Join(domainList(node), "\n"), true)
				} else if url, _ := node["url"].(string); url != "" {
					a.infoRow(c, p, "URL", url, true)
				}
				refresh, _ := numberField(node["refresh_interval"])
				a.infoRow(c, p, a.T("auto_refresh"), a.FormatInterval(int(refresh)), false)
				last, _ := node["last_refresh"].(string)
				if last == "" {
					last = "N/A"
				}
				a.infoRow(c, p, strings.TrimRight(a.T("last_refresh"), ":："), last, false)
				if ui.Button(c, a.T("refresh")).Clicked() {
					a.refreshAsync(id)
				}
			case "folder":
				mode := a.T("choice_mode_default")
				if v, _ := numberField(node["folder_mode"]); v == 1 {
					mode = a.T("choice_mode_single")
				} else if v == 2 {
					mode = a.T("choice_mode_multiple")
				}
				a.infoRow(c, p, a.T("choice_mode"), mode, false)
			case "group":
				include, _ := node["include"].([]any)
				ui.Text(c, a.F("content_count", itoa(int64(len(include))))).Bold().FontSize(12)
				if len(include) == 0 {
					ui.Text(c, "—").TextColor(p.TextMuted)
				}
				for _, ref := range include {
					refID, _ := ref.(string)
					title := refID
					iconName := "FileText"
					if ref := store.FindNode(a.list, refID); ref != nil {
						title = a.NodeTitleText(ref)
						iconName = iconForType(store.NodeType(ref))
					}
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, icon(iconName)).Size(16, 16).TextColor(p.TextMuted)
						ui.Text(c, title).TextColor(p.Text)
					})
				}
			}
			if a.InTrashcan(id) {
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, a.T("trashcan_restore")).Clicked() {
						a.restoreAsync(id)
					}
					if ui.Button(c, a.T("hosts_delete")).Clicked() {
						a.deleteAsync(id)
					}
				})
			}
		})
	})
}

func (a *App) detailsHeader(c *ui.Context, p palette, title, iconName, actionLabel string) {
	ui.Column(c).Children(func() {
		ui.Row(c).Padding(10, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon(iconName)).Size(16, 16).TextColor(p.TextMuted)
			ui.Text(c, title).FontSize(14).Bold().Grow(1).MaxLines(1).TextColor(p.Text)
			if actionLabel != "" {
				if ui.Button(c, actionLabel).Clicked() {
					a.openEditDialog(a.Current())
				}
			}
		})
		ui.Box(c).Height(1).FillWidth().Background(p.Border)
	})
}

func (a *App) infoRow(c *ui.Context, p palette, label, value string, mono bool) {
	ui.Column(c).Gap(2).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.TextMuted)
		text := ui.Text(c, value).FontSize(13).TextColor(p.Text)
		if mono {
			text.Font("Consolas, Menlo, monospace").FontSize(12)
		}
	})
}

// recoveryNotice shows the banner about an apply that needs reconciliation.
func (a *App) recoveryNotice(c *ui.Context, p palette, compact bool) {
	view := a.apply.Snapshot()
	if view == nil {
		return
	}
	message := a.T("hosts_applied_save_failed")
	if view.Status == "unknown" {
		message = a.T("hosts_application_unknown")
	}
	if compact {
		ui.Badge(c, "!").Background(p.Accent).TextColor(ui.Hex("#ffffff")).Tooltip(message)
		return
	}
	ui.Column(c).Padding(8).Gap(6).Radius(4).Background(p.AccentSoft).Children(func() {
		ui.Text(c, message).FontSize(12).TextColor(p.AccentSoftText).MaxLines(4)
	})
}

// overlays draws the dialogs and the toasts above the window's content.
func (a *App) overlays(c *ui.Context, p palette) {
	a.toastView(c, p)
	switch a.dialogKind {
	case dialogEdit:
		a.editDialog(c, p)
	case dialogPreferences:
		a.preferencesDialog(c, p)
	case dialogWriteMode:
		a.writeModeDialog(c, p)
	case dialogHistory:
		a.historyDialog(c, p)
	case dialogFind:
		a.findDialog(c, p)
	case dialogPermission:
		a.permissionDialog(c, p)
	case dialogClose:
		a.closeDialogView(c, p)
	case dialogAbout:
		a.aboutDialog(c, p)
	case dialogRecovery:
		a.recoveryDialog(c, p)
	}
	if a.HasToasts() {
		c.After(1 * time.Second)
	}
}

package appui

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oldj/switchhosts/internal/hostswrite"
	"github.com/oldj/switchhosts/internal/i18n"
	"github.com/oldj/switchhosts/internal/store"
)

// toastView shows the outcome of finished actions in the corner.
func (a *App) toastView(c *ui.Context, p palette) {
	toasts := a.pruneToasts()
	if len(toasts) == 0 {
		return
	}
	ui.Overlay(c, func() {
		ui.Column(c).Absolute().Bottom(16).Right(16).Gap(8).Width(320).Children(func() {
			for _, entry := range toasts {
				entry := entry
				bg := p.TooltipBG
				fg := p.TooltipFG
				if entry.Error {
					bg = p.Accent
					fg = ui.Hex("#ffffff")
				}
				ui.Column(c).Key(entry.ID).Padding(10, 14).Radius(6).Gap(2).
					Background(bg).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.25)).
					Children(func() {
						ui.Text(c, entry.Title).Bold().FontSize(13).TextColor(fg)
						if entry.Message != "" {
							ui.Text(c, entry.Message).FontSize(12).TextColor(fg).MaxLines(4)
						}
					})
			}
		})
	})
	if a.HasToasts() {
		c.After(2 * time.Second)
	}
}

// dialogShell draws a modal dialog with a title and a footer. It uses
// DialogBase, which draws no panel of its own: the theme's Modal adds a
// second, empty card around this one.
func dialogShell(c *ui.Context, p palette, title string, width float32, body func(), footer func()) {
	var open = true
	_, windowHeight := c.Size()
	height := windowHeight - 80
	if height > 520 {
		height = 520
	}
	if height < 220 {
		height = 220
	}
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.4))
		panel.Width(width).Height(height).Radius(8).
			Background(p.PanelBG).Shadow(0, 20, 60, 0, ui.RGBA(0, 0, 0, 0.35)).
			Clip().Children(func() {
			ui.Row(c).Padding(14, 16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).Bold().FontSize(15).Grow(1).TextColor(p.Text)
			})
			ui.Box(c).Height(1).FillWidth().Background(p.Border)
			ui.Scroll(c).Grow(1).FillWidth().Padding(16).Children(func() {
				ui.Column(c).FillWidth().Gap(10).Children(body)
			})
			if footer != nil {
				ui.Box(c).Height(1).FillWidth().Background(p.Border)
				ui.Row(c).Padding(12, 16).Gap(8).Justify(ui.End).Shrink(0).Children(footer)
			}
		})
	})
}

// editDialog adds or edits an entry.
func (a *App) editDialog(c *ui.Context, p palette) {
	form := &a.dialogForm
	title := a.T("hosts_add")
	if !form.isNew {
		title = a.T("edit")
	}
	dialogShell(c, p, title, 460, func() {
		ui.Field(c, a.T("hosts_title"), func() {
			if ui.TextInput(c, &form.title).Submitted() {
				a.commitEdit()
			}
		})
		kindNames := a.NodeKinds()
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, a.T("hosts_type")).FontSize(12).TextColor(p.TextMuted)
			ui.Segmented(c, &form.kind, kindNames...)
		})
		switch form.kind {
		case 1:
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, a.T("source_type")).FontSize(12).TextColor(p.TextMuted)
				ui.Segmented(c, &form.source, a.T("source_url"), a.T("source_domain"))
			})
			if form.source == 0 {
				ui.Field(c, "URL", func() {
					ui.TextInput(c, &form.url).Placeholder("https://example.com/hosts")
				})
			} else {
				ui.Field(c, a.T("domain_list"), func() {
					ui.TextArea(c, &form.domains).Lines(6, 12).
						Placeholder(a.T("domain_placeholder"))
				})
				ui.Text(c, a.T("domain_list_hint")).FontSize(12).TextColor(p.TextMuted)
			}
			interval := form.interval
			ui.Field(c, a.T("auto_refresh"), func() {
				ui.NumberInput(c, &interval, 0, 8760, 1)
			})
			form.interval = interval
		case 2:
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, a.T("content")).FontSize(12).TextColor(p.TextMuted)
				nodes := a.groupCandidates(form.nodeID)
				labels := make([]string, len(nodes))
				for i, node := range nodes {
					labels[i] = a.NodeTitleText(node)
				}
				selected := form.includeText
				if ui.Select(c, &selected, labels).Changed() {
					form.includeText = selected
					for _, node := range nodes {
						if a.NodeTitleText(node) == selected {
							form.include = appendUnique(form.include, store.NodeID(node))
						}
					}
				}
				for _, id := range form.include {
					id := id
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						name := id
						if node := store.FindNode(a.list, id); node != nil {
							name = a.NodeTitleText(node)
						}
						ui.Icon(c, icon("FileText")).Size(14, 14).TextColor(p.TextMuted)
						ui.Text(c, name).Grow(1).TextColor(p.Text)
						if ui.Button(c, "×").Clicked() {
							form.include = removeString(form.include, id)
						}
					})
				}
			})
		case 3:
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, a.T("choice_mode")).FontSize(12).TextColor(p.TextMuted)
				ui.Segmented(c, &form.folderMode, a.T("choice_mode_default"),
					a.T("choice_mode_single"), a.T("choice_mode_multiple"))
			})
		}
		if form.error != "" {
			ui.Text(c, form.error).FontSize(12).TextColor(p.Accent).MaxLines(3)
		}
	}, func() {
		if ui.Button(c, a.T("btn_cancel")).Clicked() {
			a.closeDialog()
		}
		if ui.PrimaryButton(c, a.T("btn_ok")).Clicked() {
			a.commitEdit()
		}
	})
}

// commitEdit saves the edit dialog's form.
func (a *App) commitEdit() {
	form := &a.dialogForm
	node := store.NodeJSON{}
	if !form.isNew {
		existing := store.FindNode(a.list, form.nodeID)
		if existing != nil {
			node = store.CloneNode(existing)
		}
		node["id"] = form.nodeID
	}
	node["title"] = strings.TrimSpace(form.title)
	switch form.kind {
	case 0:
		node["type"] = "local"
		delete(node, "url")
		delete(node, "source")
		delete(node, "domains")
	case 1:
		node["type"] = "remote"
		if form.source == 0 {
			url := strings.TrimSpace(form.url)
			if url == "" {
				form.error = a.T("unknown_error")
				return
			}
			node["url"] = url
			node["source"] = "url"
			delete(node, "domains")
		} else {
			domains := parseDomainLines(form.domains)
			if len(domains) == 0 {
				form.error = a.T("domain_list_empty")
				return
			}
			node["source"] = "domain"
			list := make([]any, len(domains))
			for i, d := range domains {
				list[i] = d
			}
			node["domains"] = list
			node["url"] = domains[0]
		}
		node["refresh_interval"] = int(form.interval) * 3600
	case 2:
		node["type"] = "group"
		list := make([]any, len(form.include))
		for i, id := range form.include {
			list[i] = id
		}
		node["include"] = list
	case 3:
		node["type"] = "folder"
		node["folder_mode"] = form.folderMode
		if _, ok := node["children"]; !ok {
			node["children"] = []any{}
		}
	}
	if _, ok := node["on"]; !ok {
		node["on"] = false
	}
	if err := a.UpsertNode(node, form.isNew); err != nil {
		form.error = err.Error()
		return
	}
	a.Select(store.NodeID(node))
	a.discardEditor()
	a.closeDialog()
}

func appendUnique(list []string, value string) []string {
	for _, item := range list {
		if item == value {
			return list
		}
	}
	return append(list, value)
}

func removeString(list []string, value string) []string {
	out := list[:0]
	for _, item := range list {
		if item != value {
			out = append(out, item)
		}
	}
	return out
}

func (a *App) groupCandidates(excludeID string) []store.NodeJSON {
	var out []store.NodeJSON
	for _, node := range store.Flatten(a.list) {
		id := store.NodeID(node)
		if id == excludeID {
			continue
		}
		switch store.NodeType(node) {
		case "local", "remote", "group":
			out = append(out, node)
		}
	}
	return out
}

// parseDomainLines reads the domains of a domain-sourced entry.
func parseDomainLines(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(store.NormalizeToLF(text), "\n") {
		for _, field := range strings.Fields(line) {
			value := strings.TrimSpace(field)
			if value == "" || strings.HasPrefix(value, "#") {
				break
			}
			domain := extractDomain(value)
			if domain == "" || seen[domain] {
				continue
			}
			seen[domain] = true
			out = append(out, domain)
		}
	}
	return out
}

// extractDomain returns the domain of a pasted line, "" when it has none.
func extractDomain(input string) string {
	value := strings.TrimSpace(input)
	if value == "" {
		return ""
	}
	index := strings.Index(value, "://")
	if index < 0 {
		if isValidDomain(value) {
			return value
		}
		return ""
	}
	rest := value[index+3:]
	if end := strings.IndexAny(rest, "/?#"); end >= 0 {
		rest = rest[:end]
	}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		rest = rest[at+1:]
	}
	if colon := strings.Index(rest, ":"); colon >= 0 {
		rest = rest[:colon]
	}
	if isValidDomain(rest) {
		return rest
	}
	return ""
}

func isValidDomain(s string) bool {
	if s == "" || len(s) > 253 || strings.Contains(s, "://") || strings.HasSuffix(s, ".") {
		return false
	}
	if strings.ContainsAny(s, "/:@ \t\r\n") {
		return false
	}
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return false
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	if len(labels[len(labels)-1]) < 2 {
		return false
	}
	return !looksLikeAddress(s)
}

// preferencesDialog edits the preferences.
func (a *App) preferencesDialog(c *ui.Context, p palette) {
	form := &a.dialogForm
	cfg := a.config
	dialogShell(c, p, a.T("preferences"), 580, func() {
		ui.Tabs(c, &form.tab, a.T("general"), a.T("advanced"), a.T("proxy"), a.T("commands"))
		ui.Column(c).Gap(12).Padding(0, 0, 12, 0).Children(func() {
			switch form.tab {
			case 0:
				names := i18n.LocaleNames()
				locales := i18n.Locales()
				labels := make([]string, len(locales))
				current := names[a.Language()]
				for i, key := range locales {
					labels[i] = names[key]
				}
				ui.Field(c, a.T("language"), func() {
					if ui.Select(c, &current, labels).Changed() {
						for i, label := range labels {
							if label == current {
								_ = a.UpdateConfig(map[string]any{"locale": locales[i]})
								break
							}
						}
					}
				})
				themes := []string{"light", "dark", "system"}
				mode := a.themeMode
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, a.T("theme")).FontSize(12).TextColor(p.TextMuted)
					if ui.Segmented(c, &mode, a.T("theme_light"), a.T("theme_dark"), a.T("theme_system")).Changed() {
						_ = a.UpdateConfig(map[string]any{"theme": themes[mode]})
					}
				})
				choice := cfg.ChoiceMode - 1
				if choice < 0 {
					choice = 0
				}
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, a.T("choice_mode")).FontSize(12).TextColor(p.TextMuted)
					if ui.Segmented(c, &choice, a.T("choice_mode_single"),
						a.T("choice_mode_multiple")).Changed() {
						_ = a.UpdateConfig(map[string]any{"choice_mode": choice + 1})
					}
				})
				history := cfg.HistoryEnabled
				if ui.Checkbox(c, &history, a.T("history_record")).Changed() {
					_ = a.UpdateConfig(map[string]any{"history_enabled": history})
				}
				limit := float64(cfg.HistoryLimit)
				ui.Field(c, strings.TrimRight(a.T("system_hosts_history_limit"), ":："), func() {
					ui.Stepper(c, &limit, 0, 10000, 10)
				})
				if int(limit) != cfg.HistoryLimit {
					_, _ = hostswrite.UpdateLimit(a.paths, cfg, int(limit), nil)
				}
				autostart := cfg.LaunchAtLogin
				if ui.Checkbox(c, &autostart, a.T("launch_at_login")).Changed() {
					_ = a.UpdateConfig(map[string]any{"launch_at_login": autostart})
				}
				hideAtLaunch := cfg.HideAtLaunch
				if ui.Checkbox(c, &hideAtLaunch, a.T("hide_at_launch")).Changed() {
					_ = a.UpdateConfig(map[string]any{"hide_at_launch": hideAtLaunch})
				}
				quitOnClose := cfg.QuitOnClose
				if ui.Checkbox(c, &quitOnClose, a.T("quit_on_close")).Changed() {
					_ = a.UpdateConfig(map[string]any{"quit_on_close": quitOnClose})
				}
				lightweight := cfg.LightweightMode
				if ui.Checkbox(c, &lightweight, a.T("lightweight_mode")).Changed() {
					_ = a.UpdateConfig(map[string]any{"lightweight_mode": lightweight})
				}
			case 1:
				multi := cfg.MultiChooseFolderAll
				if ui.Checkbox(c, &multi, a.T("multi_chose_folder_switch_all")).Changed() {
					_ = a.UpdateConfig(map[string]any{"multi_chose_folder_switch_all": multi})
				}
				dedup := cfg.RemoveDuplicate
				if ui.Checkbox(c, &dedup, a.T("remove_duplicate_records")).Changed() {
					_ = a.UpdateConfig(map[string]any{"remove_duplicate_records": dedup})
				}
				startup := cfg.RefreshRemoteOnStartup
				if ui.Checkbox(c, &startup, a.T("refresh_remote_hosts_on_startup")).Changed() {
					_ = a.UpdateConfig(map[string]any{"refresh_remote_hosts_on_startup": startup})
				}
				mini := cfg.TrayMiniWindow
				if ui.Checkbox(c, &mini, a.T("tray_mini_window")).Changed() {
					_ = a.UpdateConfig(map[string]any{"tray_mini_window": mini})
				}
				autoUpdate := cfg.AutoCheckUpdate
				if ui.Checkbox(c, &autoUpdate, a.T("auto_check_update")).Changed() {
					_ = a.UpdateConfig(map[string]any{"auto_check_update": autoUpdate})
				}
				api := cfg.HTTPAPIOn
				if ui.Checkbox(c, &api, a.T("http_api_on")).Changed() {
					_ = a.UpdateConfig(map[string]any{"http_api_on": api})
				}
				port := float64(cfg.HTTPAPIPort)
				ui.Field(c, a.T("port"), func() {
					ui.NumberInput(c, &port, 1, 65535, 1)
				})
				if int(port) != cfg.HTTPAPIPort {
					_ = a.UpdateConfig(map[string]any{"http_api_port": int(port)})
				}
				path, _ := hostswrite.SystemHostsPath()
				a.infoRow(c, p, strings.TrimRight(a.T("your_hosts_file_is"), ":："), path, true)
				a.infoRow(c, p, strings.TrimRight(a.T("your_data_is"), ":："), a.paths.Root, true)
			case 2:
				proxy := cfg.UseProxy
				if ui.Checkbox(c, &proxy, a.T("use_proxy")).Changed() {
					_ = a.UpdateConfig(map[string]any{"use_proxy": proxy})
				}
				proto := cfg.ProxyProtocol
				ui.Field(c, a.T("protocol"), func() {
					if ui.Select(c, &proto, []string{"http", "https", "socks5"}).Changed() {
						_ = a.UpdateConfig(map[string]any{"proxy_protocol": proto})
					}
				})
				host := cfg.ProxyHost
				ui.Field(c, a.T("host"), func() {
					if ui.TextInput(c, &host).Changed() {
						_ = a.UpdateConfig(map[string]any{"proxy_host": host})
					}
				})
				port := float64(cfg.ProxyPort)
				ui.Field(c, a.T("port"), func() {
					ui.NumberInput(c, &port, 0, 65535, 1)
				})
				if int(port) != cfg.ProxyPort {
					_ = a.UpdateConfig(map[string]any{"proxy_port": int(port)})
				}
			case 3:
				command := cfg.CmdAfterHostsApply
				ui.Field(c, a.T("commands_title"), func() {
					if ui.TextArea(c, &command).Lines(3, 8).Changed() {
						_ = a.UpdateConfig(map[string]any{"cmd_after_hosts_apply": command})
					}
				})
				ui.Text(c, a.T("commands_help")).FontSize(12).TextColor(p.TextMuted).
					MaxLines(4)
			}
		})
	}, func() {
		if ui.Button(c, a.T("close")).Clicked() {
			a.closeDialog()
		}
	})
}

// writeModeDialog asks which write mode applies, then runs the toggle that
// waited for it.
func (a *App) writeModeDialog(c *ui.Context, p palette) {
	form := &a.dialogForm
	choice := form.writeMode
	dialogShell(c, p, a.T("write_mode_set"), 420, func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Segmented(c, &choice, a.T("append"), a.T("overwrite"))
			help := a.T("write_mode_append_help")
			if choice == 1 {
				help = a.T("write_mode_overwrite_help")
			}
			ui.Text(c, help).FontSize(12).TextColor(p.TextMuted).MaxLines(4)
		})
	}, func() {
		if ui.Button(c, a.T("btn_cancel")).Clicked() {
			a.closeDialog()
		}
		if ui.PrimaryButton(c, a.T("btn_ok")).Clicked() {
			form.writeMode = choice
			mode := "append"
			if choice == 1 {
				mode = "overwrite"
			}
			_ = a.UpdateConfig(map[string]any{"write_mode": mode})
			id, on, has := form.pendingID, form.pendingOn, form.hasPending
			a.closeDialog()
			if has {
				go a.applyPendingToggle(id, on)
			}
		}
	})
}

// historyDialog lists the saved system hosts snapshots.
func (a *App) historyDialog(c *ui.Context, p palette) {
	items := a.historyItems()
	dialogShell(c, p, a.T("show_history"), 640, func() {
		if len(items) == 0 {
			ui.Text(c, a.T("history_empty")).TextColor(p.TextMuted)
			return
		}
		ui.Column(c).Gap(6).Children(func() {
			for _, item := range items {
				item := item
				ui.Column(c).Key(item.ID).Padding(8).Radius(4).Background(p.PanelBG).
					Border(1, p.Border).Gap(4).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, store.FormatTimestamp(item.AddTimeMS)).FontSize(12).
							TextColor(p.TextMuted).Grow(1)
						if ui.Button(c, a.T("restore")).Clicked() {
							a.restoreHistory(item)
						}
						if ui.Button(c, "×").Clicked() {
							_, _ = hostswrite.DeleteHistoryItem(
								hostswrite.HistoriesPath(a.paths), item.ID)
						}
					})
					preview := item.Content
					if lines := splitLines(preview); len(lines) > 4 {
						preview = strings.Join(lines[:4], "\n") + "\n…"
					}
					ui.Text(c, preview).Font("Consolas, Menlo, monospace").FontSize(11).
						TextColor(p.EditorComment).MaxLines(5)
				})
			}
		})
	}, func() {
		if ui.Button(c, a.T("clear_history")).Clicked() {
			_ = hostswrite.ClearHistory(hostswrite.HistoriesPath(a.paths))
		}
		if ui.Button(c, a.T("close")).Clicked() {
			a.closeDialog()
		}
	})
}

// restoreHistory puts a saved snapshot back.
func (a *App) restoreHistory(item hostswrite.HistoryItem) {
	previous, _, err := hostswrite.ReadSystemHosts()
	if err != nil {
		a.addToast(a.T("fail"), err.Error(), true)
		return
	}
	if _, err := hostswrite.Apply(item.Content, "overwrite", ""); err != nil {
		a.addToast(a.T("fail"), err.Error(), true)
		return
	}
	_ = hostswrite.RecordChange(a.paths, a.config.HistoryLimit, a.config.HistoryEnabled,
		previous, item.Content)
	if a.currentID == "0" {
		a.loadEditor()
		a.discardEditor()
	}
}

// aboutDialog shows the version and the project's links.
func (a *App) aboutDialog(c *ui.Context, p palette) {
	dialogShell(c, p, a.T("about"), 380, func() {
		ui.Column(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "SwitchHosts").Bold().FontSize(18).TextColor(p.Text)
			ui.Text(c, "v"+a.version).TextColor(p.TextMuted)
			ui.Text(c, a.T("acknowledgement")).FontSize(12).TextColor(p.TextMuted)
			ui.Link(c, "switchhosts.app", "https://switchhosts.app")
			ui.Link(c, "github.com/oldj/SwitchHosts", "https://github.com/oldj/SwitchHosts")
		})
	}, func() {
		if ui.Button(c, a.T("close")).Clicked() {
			a.closeDialog()
		}
	})
}

// recoveryDialog asks where the data lives when the recorded folder is gone.
func (a *App) recoveryDialog(c *ui.Context, p palette) {
	if a.recovery == nil {
		return
	}
	title := a.T("data_dir_missing_title")
	message := a.T("data_dir_missing_message")
	if a.recovery.Kind == "invalid" {
		title = a.T("data_dir_invalid_title")
		message = a.T("data_dir_invalid_message")
	}
	var open = true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.4))
		panel.Width(460).Padding(20).Gap(12).Radius(8).Background(p.PanelBG).
			Shadow(0, 20, 60, 0, ui.RGBA(0, 0, 0, 0.35)).Children(func() {
			ui.Text(c, title).Bold().FontSize(15).TextColor(p.Text)
			ui.Text(c, message).FontSize(13).TextColor(p.Text).MaxLines(4)
			if a.recovery.Path != "" {
				ui.Text(c, a.recovery.Path).Font("Consolas, Menlo, monospace").FontSize(12).
					TextColor(p.TextMuted)
			}
			if ui.PrimaryButton(c, a.T("data_dir_use_default")).Clicked() {
				_ = store.ClearPointer()
				if a.host != nil {
					a.host.Quit()
				}
			}
			if ui.Button(c, a.T("data_dir_choose_new")).Clicked() {
				a.restartWithPickedDir()
			}
			if ui.Button(c, a.T("quit")).Clicked() {
				if a.host != nil {
					a.host.Quit()
				}
			}
		})
	})
}

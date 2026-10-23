package appui

import (
	"github.com/egoist/mygo/ui"
	"github.com/oldj/switchhosts/internal/store"
)

// findDialog searches the content of every entry and jumps to a match.
func (a *App) findDialog(c *ui.Context, p palette) {
	find := &a.find
	replacement := find.replacement
	changed := false
	dialogShell(c, p, a.T("find"), 560, func() {
		ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
			query := find.query
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				ui.Text(c, a.T("find")).FontSize(12).TextColor(p.TextMuted)
				if ui.TextInput(c, &query).AutoFocus().Changed() {
					find.query = query
					changed = true
				}
			})
		})
		ui.Row(c).Gap(16).Children(func() {
			ignore := find.ignoreCase
			if ui.Checkbox(c, &ignore, a.T("ignore_case")).Changed() {
				find.ignoreCase = ignore
				changed = true
			}
			useRegexp := find.regexp
			if ui.Checkbox(c, &useRegexp, a.T("regexp")).Changed() {
				find.regexp = useRegexp
				changed = true
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				ui.Text(c, a.T("replace")).FontSize(12).TextColor(p.TextMuted)
				ui.TextInput(c, &replacement)
			})
			if ui.Button(c, a.T("replace_all")).Clicked() {
				find.replacement = replacement
				count := a.replaceAllWith(find.query, replacement, find.ignoreCase, find.regexp)
				if count > 0 {
					a.addToast(a.T("replace"), a.T("replace")+": "+itoa(int64(count)), false)
				}
			}
		})
		if find.error != "" {
			ui.Text(c, find.error).FontSize(12).TextColor(p.EditorError).MaxLines(3)
		}
		if len(find.results) == 0 {
			if find.query != "" && find.changedAt != "" {
				ui.Text(c, a.T("find")+" —").TextColor(p.TextMuted)
			}
			return
		}
		ui.Text(c, itoa(int64(len(find.results)))).Bold().FontSize(12).
			TextColor(p.TextMuted)
		ui.Column(c).Gap(4).Children(func() {
			for _, result := range find.results {
				result := result
				row := ui.Column(c).Key(result.ID).Padding(8).Radius(4).Gap(2).
					Background(p.PanelBG).Border(1, p.Border).Focusable()
				row.Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, icon(iconForType(result.Kind))).Size(14, 14).
							TextColor(p.TextMuted)
						ui.Text(c, result.Title).Bold().FontSize(13).TextColor(p.Text)
						ui.Text(c, "("+itoa(int64(result.Count))+")").
							FontSize(11).TextColor(p.TextMuted)
					})
					ui.Text(c, result.Snippet).Font("Consolas, Menlo, monospace").FontSize(11).
						TextColor(p.EditorComment).MaxLines(2)
				})
				if row.Clicked() {
					a.Select(result.ID)
					a.discardEditor()
					a.closeDialog()
				}
			}
		})
	}, func() {
		if ui.Button(c, a.T("close")).Clicked() {
			a.closeDialog()
		}
	})
	if changed || find.changedAt == "" {
		if find.query != "" {
			find.results = a.searchEntries(find.query, find.ignoreCase, find.regexp)
		} else {
			find.results = nil
		}
		c.Invalidate()
	}
	_ = store.NowMillis
}

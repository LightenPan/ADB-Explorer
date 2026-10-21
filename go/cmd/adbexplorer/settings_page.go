package main

import (
	"strconv"

	"github.com/egoist/mygo/ui"
)

func (a *app) settingsPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Text(c, a.tr("Settings")).FontSize(20).Bold()

		hidden := a.showHidden()
		if ui.Checkbox(c, &hidden, a.tr("Show hidden files")).Changed() {
			a.setShowHidden(hidden)
		}

		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, a.tr("Refresh devices every"))
			text := ""
			if n := a.pollSeconds(); n > 0 {
				text = strconv.Itoa(n)
			}
			if ui.TextInput(c, &text).Width(60).Label(a.tr("seconds")).Submitted() {
				n, err := strconv.Atoi(text)
				if err != nil {
					n = 0
				}
				a.setPoll(n)
			}
			ui.Text(c, a.tr("seconds, 0 to stop")).TextColor(t.TextMuted)
		})

		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, a.tr("Default folder"))
			path := current.DefaultPath
			if ui.TextInput(c, &path).Width(240).Label("path").Submitted() {
				a.setDefaultPath(path)
			}
		})

		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, a.tr("Preview at most"))
			text := strconv.Itoa(current.MaxPreviewKB)
			if current.MaxPreviewKB == 0 {
				text = "300"
			}
			if ui.TextInput(c, &text).Width(60).Label("KB").Submitted() {
				n, err := strconv.Atoi(text)
				if err != nil {
					n = 300
				}
				a.setMaxPreviewKB(n)
			}
			ui.Text(c, "KB").TextColor(t.TextMuted)
		})

		pdf := a.previewPDFEnabled()
		if ui.Checkbox(c, &pdf, a.tr("Preview PDF pages")).Changed() {
			a.setPreviewPDF(pdf)
		}

		ui.Text(c, a.tr("Bookmarks")).Bold()
		for _, loc := range a.locations() {
			item := loc
			ui.Row(c.Key("mark-" + item.Path)).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, item.Label).Grow(1)
				ui.Text(c, item.Path).FontSize(12).TextColor(t.TextMuted)
				if ui.Button(c, a.tr("Remove")).Clicked() {
					a.removeLocation(item.Path)
				}
			})
		}
		if ui.Button(c, a.tr("Bookmark this folder")).Disabled(a.serial == "").Clicked() {
			p := a.focus()
			a.addLocation(p.path, baseName(p.path))
		}
		ui.Text(c, a.tr("Settings are saved in the app data folder.")).FontSize(12).TextColor(t.TextMuted)
	})
}

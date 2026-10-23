package appui

import (
	"github.com/egoist/mygo/ui"
)

// closeDialogView asks what the window's close button should do. It shows
// the three outcomes the title bar's ✕ has: hiding to the tray, minimizing
// to the taskbar, and quitting.
func (a *App) closeDialogView(c *ui.Context, p palette) {
	var open = true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.4))
		panel.Width(440).Padding(20).Gap(14).Radius(8).Background(p.PanelBG).
			Shadow(0, 20, 60, 0, ui.RGBA(0, 0, 0, 0.35)).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icon("X")).Size(18, 18).TextColor(p.TextMuted)
				ui.Text(c, a.T("close_dialog_title")).Bold().FontSize(15).TextColor(p.Text)
			})
			ui.Text(c, a.T("close_dialog_message")).FontSize(13).TextColor(p.TextMuted).
				MaxLines(3)

			for _, choice := range []struct {
				Kind  int
				Icon  string
				Label string
				Hint  string
				// Accessible is the label the choice is found by, distinct from
				// the window buttons that share these words.
				Accessible string
			}{
				{closeToTray, "ChevronDown", a.T("close_to_tray"), a.T("close_to_tray_hint"),
					a.T("close_to_tray")},
				{closeMinimize, "Minus", a.T("minimize"), a.T("close_minimize_hint"),
					a.T("close_minimize_choice")},
				{closeQuit, "X", a.T("quit"), a.T("close_quit_hint"),
					a.T("close_quit_choice")},
			} {
				choice := choice
				btn := ui.Row(c).FillWidth().Padding(10, 12).Radius(6).Gap(10).
					AlignItems(ui.Center).Background(p.MainBG).Border(1, p.Border).
					Focusable().Label(choice.Accessible)
				if btn.Hovered() {
					btn.Background(p.AccentSoft)
				}
				btn.Children(func() {
					ui.Icon(c, icon(choice.Icon)).Size(16, 16).TextColor(p.TextMuted)
					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, choice.Label).FontSize(13).TextColor(p.Text)
						ui.Text(c, choice.Hint).FontSize(11).TextColor(p.TextMuted).MaxLines(1)
					})
				})
				if btn.Clicked() {
					a.chooseClose(choice.Kind)
				}
			}

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, a.T("btn_cancel")).Clicked() {
					a.closeDialog()
				}
			})
		})
	})
}

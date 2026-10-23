package appui

import (
	"github.com/egoist/mygo/ui"
)

// permissionDialog tells the user the app cannot write the system hosts
// file, and offers to restart with administrator rights. Without that
// right, saving or switching entries silently does nothing, so the notice
// comes up before the window's content is used.
func (a *App) permissionDialog(c *ui.Context, p palette) {
	var open = true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.4))
		panel.Width(520).Padding(20).Gap(14).Radius(8).Background(p.PanelBG).
			Shadow(0, 20, 60, 0, ui.RGBA(0, 0, 0, 0.35)).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icon("DeviceDesktop")).Size(20, 20).TextColor(p.Accent)
				ui.Text(c, a.T("permission_title")).Bold().FontSize(15).TextColor(p.Text)
			})
			ui.Text(c, a.T("permission_message")).FontSize(13).TextColor(p.Text).MaxLines(5)
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, a.T("permission_path")).FontSize(12).TextColor(p.TextMuted)
				ui.Text(c, a.permissionPath).Font("Consolas, Menlo, monospace").FontSize(12).
					TextColor(p.Text)
			})
			if a.elevated {
				ui.Text(c, a.T("permission_elevated")).FontSize(12).TextColor(p.TextMuted)
			}
			if !a.canElevate {
				ui.Text(c, a.T("permission_later")).FontSize(12).TextColor(p.TextMuted).MaxLines(3)
			}
			if a.elevationError != "" {
				ui.Text(c, a.elevationError).FontSize(12).TextColor(p.EditorError).MaxLines(3)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, a.T("permission_continue")).Clicked() {
					a.dismissPermission()
					if a.host != nil {
						a.host.Refresh()
					}
				}
				if a.canElevate && !a.elevated {
					if ui.PrimaryButton(c, a.T("permission_restart")).Clicked() {
						a.restartElevated()
						if a.exitForElevation && a.host != nil {
							a.host.Refresh()
						} else if a.host != nil {
							a.host.Refresh()
						}
					}
				}
			})
		})
	})
}

// runPermissionExit quits the instance once an elevated one was started.
func (a *App) runPermissionExit() {
	if !a.TakeExitForElevation() || a.host == nil {
		return
	}
	a.host.Quit()
}

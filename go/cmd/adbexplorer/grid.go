package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *app) iconGrid(c *ui.Context, p *pane, index int) {
	rows := a.visibleOf(p)
	t := c.Theme()
	p.grid.Key = func(i int) any { return rows[i].Name }
	p.grid.Label = func(i int) string { return rows[i].Name }
	p.grid.Selection = &p.chosen
	p.grid.Selected = &p.picked
	grid := ui.GridView(c.Key(fmt.Sprintf("icons-%d", index)), &p.grid, len(rows), 104, 96, func(i int) {
		e := rows[i]
		remote := joinPath(p.path, e.Name)
		if isImageName(e.Name) {
			a.wantIcon(remote, e.Name, e.Size)
		}
		ui.Column(c).Center().Gap(4).Padding(6).Children(func() {
			if bmp := a.iconOf(remote); bmp != nil {
				ui.Image(c, bmp).Size(64, 64)
			} else {
				ui.Box(c).Size(64, 64).Radius(8).Background(t.SurfacePressed).Center().Children(func() {
					a.fileIcon(c, p, e, 40)
				})
			}
			ui.Text(c, e.Name).FontSize(11).SingleLine()
		})
	}).Grow(1)
	if grid.Submitted() && p.picked >= 0 && p.picked < len(rows) {
		a.openListed(p, index, rows, p.picked)
	}
}

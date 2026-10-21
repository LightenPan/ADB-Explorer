package main

import "strings"

func (a *app) apkInfoSelected() {
	p := a.focus()
	sel := selectedOf(p, a.visibleOf(p))
	if len(sel) == 1 && !sel[0].IsDir() && apkFamily(strings.ToLower(sel[0].Name)) {
		a.showDetail()
		return
	}
	a.status = a.tr("Select an apk")
}

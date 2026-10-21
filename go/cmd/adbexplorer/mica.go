package main

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

//go:embed appicon.png
var appIconPNG []byte

var appIconBmp *ui.Bitmap

func appIcon() *ui.Bitmap {
	if appIconBmp == nil && len(appIconPNG) > 0 {
		appIconBmp, _ = ui.DecodeBitmap(appIconPNG)
	}
	return appIconBmp
}

// micaTheme is the light green file-manager palette from the Mica study.
func micaTheme() *ui.Theme {
	t := ui.LightTheme()
	t.Background = ui.Hex("#f3f3f3")
	t.Surface = ui.Hex("#f7f7f7")
	t.SurfaceHover = ui.Hex("#ececec")
	t.SurfacePressed = ui.Hex("#d9efe4")
	t.Border = ui.Hex("#e5e5e5")
	t.Text = ui.Hex("#1a1a1a")
	t.TextMuted = ui.Hex("#5c5c5c")
	t.Accent = ui.Hex("#0e6b4f")
	t.AccentHover = ui.Hex("#0c5b43")
	t.AccentPressed = ui.Hex("#0a4c38")
	t.AccentText = ui.Hex("#ffffff")
	t.Selection = ui.Hex("#d9efe4")
	t.Danger = ui.Hex("#a32020")
	t.Radius = 6
	t.FontSize = 13
	return t
}

// railGlyphs are the page marks. Icon paints them in the text color.
var railGlyphs = map[string]*ui.SVG{
	pageExplorer: ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M1.5 4.2h4.2L7 6h7.5V13H1.5z"/></svg>`)),
	pageDevices:  ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M5.2 1.4h5.6a1 1 0 0 1 1 1v11.2a1 1 0 0 1-1 1H5.2a1 1 0 0 1-1-1V2.4a1 1 0 0 1 1-1z"/></svg>`)),
	pageOps:      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M2 6.2h7.2L7.4 4.4 8.6 3.2 12 6.2 8.6 9.2 7.4 8 9.2 6.2H2zM14 9.8H6.8L8.6 11.6 7.4 12.8 4 9.8l3.4-3 1.2 1.2L6.8 9.8H14z"/></svg>`)),
	pageApps:     ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect x="2" y="2" width="5" height="5" rx="1"/><rect x="9" y="2" width="5" height="5" rx="1"/><rect x="2" y="9" width="5" height="5" rx="1"/><rect x="9" y="9" width="5" height="5" rx="1"/></svg>`)),
	pageLog:      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect x="2" y="2.4" width="12" height="1.7"/><rect x="2" y="5.6" width="12" height="1.7"/><rect x="2" y="8.8" width="8" height="1.7"/><rect x="2" y="12" width="5" height="1.7"/></svg>`)),
	pageSettings: ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect x="2" y="3" width="12" height="1.6"/><rect x="2" y="7.2" width="12" height="1.6"/><rect x="2" y="11.4" width="12" height="1.6"/><circle cx="6" cy="3.8" r="1.3"/><circle cx="11" cy="8" r="1.3"/><circle cx="7" cy="12.2" r="1.3"/></svg>`)),
}

var pkgPalette = []ui.Color{
	ui.Hex("#0e6b4f"),
	ui.Hex("#1d4e89"),
	ui.Hex("#8d3f24"),
	ui.Hex("#6b3fa0"),
	ui.Hex("#9a6b12"),
	ui.Hex("#3d5a40"),
	ui.Hex("#8a3d55"),
	ui.Hex("#2f5d62"),
}

func pkgColor(id string) ui.Color {
	h := 0
	for _, r := range id {
		h = h*33 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return pkgPalette[h%len(pkgPalette)]
}

func pkgMark(name string) string {
	for _, r := range name {
		return string(r)
	}
	return "?"
}

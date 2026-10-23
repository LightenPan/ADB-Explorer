package appui

import (
	"embed"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

//go:embed icons/*.svg
var iconFiles embed.FS

//go:embed icons/logo.png
var logoPNG []byte

var (
	iconOnce sync.Once
	icons    map[string]*ui.SVG
	logo     *ui.Bitmap
)

// logoImage returns the app's logo, decoded once.
func logoImage() *ui.Bitmap {
	iconOnce.Do(parseIcons)
	return logo
}

func parseIcons() {
	icons = map[string]*ui.SVG{}
	if bitmap, err := ui.DecodeBitmap(logoPNG); err == nil {
		logo = bitmap
	}
	entries, err := iconFiles.ReadDir("icons")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".svg") {
			continue
		}
		data, err := iconFiles.ReadFile("icons/" + entry.Name())
		if err != nil {
			continue
		}
		svg, err := ui.ParseSVG(data)
		if err != nil {
			continue
		}
		name := entry.Name()
		icons[name[:len(name)-len(".svg")]] = svg
	}
}

// icon returns an embedded icon by name, parsed once.
func icon(name string) *ui.SVG {
	iconOnce.Do(parseIcons)
	return icons[name]
}

// The look of the app, matching the light and dark themes of the web UI.
type palette struct {
	Dark bool

	WindowBG   ui.Color
	TopBarBG   ui.Color
	PanelBG    ui.Color
	MainBG     ui.Color
	Border     ui.Color
	BorderWeak ui.Color

	Text      ui.Color
	TextMuted ui.Color
	TextFaint ui.Color

	AccentSoft      ui.Color
	AccentSoftHover ui.Color
	AccentSoftText  ui.Color
	AccentIndicator ui.Color
	Accent          ui.Color

	SelectedBG   ui.Color
	SelectedText ui.Color

	SwitchOn        ui.Color
	SwitchOff       ui.Color
	SwitchOffBorder ui.Color
	SwitchKnob      ui.Color

	EditorText    ui.Color
	EditorComment ui.Color
	EditorIP      ui.Color
	EditorError   ui.Color
	LineNumber    ui.Color
	ReadOnlyBG    ui.Color

	StatusText ui.Color
	TooltipBG  ui.Color
	TooltipFG  ui.Color
}

var lightPalette = palette{
	WindowBG:        ui.Hex("#f1f3f5"),
	TopBarBG:        ui.Hex("#ffffff"),
	PanelBG:         ui.Hex("#ffffff"),
	MainBG:          ui.Hex("#ffffff"),
	Border:          ui.Hex("#e9ecef"),
	BorderWeak:      ui.Hex("#f1f3f5"),
	Text:            ui.Hex("#000000"),
	TextMuted:       ui.Hex("#868e96"),
	TextFaint:       ui.Hex("#adb5bd"),
	AccentSoft:      ui.Hex("#f7dce2"),
	AccentSoftHover: ui.Hex("#f2ccd5"),
	AccentSoftText:  ui.Hex("#98283b"),
	AccentIndicator: ui.Hex("#ce3549"),
	Accent:          ui.Hex("#cf3949"),
	SelectedBG:      ui.Hex("#f7dce2"),
	SelectedText:    ui.Hex("#98283b"),
	SwitchOn:        ui.Hex("#c9364f"),
	SwitchOff:       ui.Hex("#d6d8dd"),
	SwitchOffBorder: ui.Hex("#a0a4ad"),
	SwitchKnob:      ui.Hex("#ffffff"),
	EditorText:      ui.Hex("#303238"),
	EditorComment:   ui.Hex("#77716e"),
	EditorIP:        ui.Hex("#8250b0"),
	EditorError:     ui.Hex("#b42336"),
	LineNumber:      ui.Hex("#bbb6b3"),
	ReadOnlyBG:      ui.Hex("#fafafb"),
	StatusText:      ui.Hex("#999999"),
	TooltipBG:       ui.Hex("#212529"),
	TooltipFG:       ui.Hex("#ffffff"),
}

var darkPalette = palette{
	Dark:            true,
	WindowBG:        ui.Hex("#1a1b1e"),
	TopBarBG:        ui.Hex("#272b34"),
	PanelBG:         ui.Hex("#25262b"),
	MainBG:          ui.Hex("#25262b"),
	Border:          ui.Hex("#373a40"),
	BorderWeak:      ui.Hex("#2c2e33"),
	Text:            ui.Hex("#aaaaaa"),
	TextMuted:       ui.Hex("#666666"),
	TextFaint:       ui.Hex("#555555"),
	AccentSoft:      ui.Hex("#623b47"),
	AccentSoftHover: ui.Hex("#754654"),
	AccentSoftText:  ui.Hex("#f2cdd5"),
	AccentIndicator: ui.Hex("#e88496"),
	Accent:          ui.Hex("#cf3949"),
	SelectedBG:      ui.Hex("#623b47"),
	SelectedText:    ui.Hex("#f2cdd5"),
	SwitchOn:        ui.Hex("#d46a7e"),
	SwitchOff:       ui.Hex("#50535b"),
	SwitchOffBorder: ui.Hex("#777b85"),
	SwitchKnob:      ui.Hex("#ffffff"),
	EditorText:      ui.Hex("#dedbd9"),
	EditorComment:   ui.Hex("#aaa3a0"),
	EditorIP:        ui.Hex("#cba2f2"),
	EditorError:     ui.Hex("#ff8f9e"),
	LineNumber:      ui.Hex("#625d5a"),
	ReadOnlyBG:      ui.Hex("#383a3e"),
	StatusText:      ui.Hex("#666666"),
	TooltipBG:       ui.Hex("#e9ecef"),
	TooltipFG:       ui.Hex("#1a1b1e"),
}

// themeFor returns the widget theme built on the app's palette, so widgets
// and custom elements share colors.
func themeFor(p palette) *ui.Theme {
	t := ui.LightTheme()
	if p.Dark {
		t = ui.DarkTheme()
	}
	t.Background = p.WindowBG
	t.Surface = p.PanelBG
	t.SurfaceHover = p.BorderWeak
	t.SurfacePressed = p.Border
	t.Border = p.Border
	t.Text = p.Text
	t.TextMuted = p.TextMuted
	t.Accent = p.Accent
	t.AccentHover = p.Accent
	t.AccentPressed = p.Accent
	t.AccentText = ui.Hex("#ffffff")
	t.Selection = p.AccentSoft
	t.Focus = p.Accent
	t.Inverse = p.TooltipBG
	t.InverseText = p.TooltipFG
	t.Scrollbar = p.TextFaint
	t.Radius = 4
	t.Spacing = 4
	return t
}

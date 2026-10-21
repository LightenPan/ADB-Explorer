package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"path"
	"strings"
	"time"

	"adbexplorer/internal/adb"

	"github.com/egoist/mygo/ui"
)

// fileGlyphs are the small marks drawn in front of a name: a folder, a
// picture, a package, an archive and a plain file. They are paths only,
// which is all the UI draws.
var fileGlyphs = map[string]*ui.SVG{
	"dir":  ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill-rule="evenodd" d="M3 6.5A2.5 2.5 0 0 1 5.5 4H9l2 2.5h7.5A2.5 2.5 0 0 1 21 9v8.5a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5z M5.5 8.5h13a1 1 0 0 1 1 1V17a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1V9.5a1 1 0 0 1 1-1z"/></svg>`)),
	"file": ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill-rule="evenodd" d="M6 2.5h7.2L20 9.3V20a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V4.5a2 2 0 0 1 2-2z M8 11h8v1.8H8z m0 3.6h8V16.4H8z M8 7h4.2v1.8H8z"/></svg>`)),
}

// fileIcon draws the mark for a file at size×size. When a picture or an apk
// already has its own icon cached, that is shown instead.
func (a *app) fileIcon(c *ui.Context, p *pane, e adb.Entry, size float32) {
	t := c.Theme()
	if bmp := a.iconOf(joinPath(p.path, e.Name)); bmp != nil {
		ui.Image(c, bmp).Size(size, size)
		return
	}
	glyph := fileGlyphs["file"]
	if e.IsDir() {
		glyph = fileGlyphs["dir"]
	}
	ui.Icon(c, glyph).Size(size, size).TextColor(t.Accent)
}

// iconFor reports the largest PNG icon stored in an apk. Adaptive icon XML
// and binary manifests are skipped; those need a decoder this build does not have.
func iconFor(data []byte) ([]byte, bool) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, false
	}
	var best []byte
	var bestScore int
	for _, f := range r.File {
		name := strings.ToLower(f.Name)
		if !strings.HasPrefix(name, "res/") || !strings.HasSuffix(name, ".png") {
			continue
		}
		if !strings.Contains(name, "icon") && !strings.Contains(name, "launcher") {
			continue
		}
		if f.UncompressedSize64 > 512*1024 {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		buf, err := io.ReadAll(io.LimitReader(rc, 512*1024))
		rc.Close()
		if err != nil || len(buf) < 8 {
			continue
		}
		score := iconScore(name, len(buf))
		if score > bestScore {
			best, bestScore = buf, score
		}
	}
	return best, best != nil
}

func iconScore(name string, size int) int {
	score := size
	for _, hint := range []struct {
		text string
		add  int
	}{
		{"xxxhdpi", 400000},
		{"xxhdpi", 300000},
		{"xhdpi", 200000},
		{"hdpi", 100000},
		{"mdpi", 0},
		{"ldpi", -100000},
	} {
		if strings.Contains(name, hint.text) {
			score += hint.add
		}
	}
	if strings.Contains(name, "foreground") {
		score += 50000
	}
	if strings.Contains(name, "round") {
		score -= 10000
	}
	return score
}

// fileIcons caches a decoded image per remote path.
var fileIcons = map[string]*ui.Bitmap{}

func (a *app) iconOf(remote string) *ui.Bitmap {
	if b, ok := fileIcons[remote]; ok {
		return b
	}
	return nil
}

func (a *app) wantIcon(remote, name string, size uint32) {
	if _, ok := fileIcons[remote]; ok || a.serial == "" {
		return
	}
	if size > 8*1024*1024 {
		return
	}
	low := strings.ToLower(name)
	kind := ""
	switch {
	case strings.HasSuffix(low, ".apk"):
		kind = "apk"
	case strings.HasSuffix(low, ".png"), strings.HasSuffix(low, ".jpg"), strings.HasSuffix(low, ".jpeg"), strings.HasSuffix(low, ".gif"), strings.HasSuffix(low, ".webp"), strings.HasSuffix(low, ".bmp"):
		kind = "img"
	default:
		return
	}
	fileIcons[remote] = nil
	serial := a.serial
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		var buf bytes.Buffer
		_, err := a.client().Pull(ctx, serial, remote, &buf)
		var bmp *ui.Bitmap
		if err == nil {
			data := buf.Bytes()
			if kind == "apk" {
				if png, ok := iconFor(data); ok {
					data = png
				} else {
					data = nil
				}
			}
			if len(data) > 0 {
				bmp, _ = ui.DecodeBitmap(data)
			}
		}
		a.winUpdate(func() {
			if bmp != nil {
				setFileIcon(remote, bmp)
			} else {
				delete(fileIcons, remote)
			}
		})
	}()
}

func isImageName(name string) bool {
	low := strings.ToLower(path.Base(name))
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".apk"} {
		if strings.HasSuffix(low, ext) {
			return true
		}
	}
	return false
}

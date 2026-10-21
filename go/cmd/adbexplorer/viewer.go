package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"os"
	"path"
	"strings"
	"time"

	"adbexplorer/internal/adb"

	"github.com/egoist/mygo/ui"
)

// videoCap is how much of a video the viewer will copy to a temp file.
// Larger files stay on the device; the viewer says why.
const videoCap = 1024 * 1024 * 1024

// launchFile opens a local path in the default app. Tests replace it.
var launchFile = launchFileOS

// viewer is the fullscreen image and video surface. The details pane stays
// the small preview; double-click and View large open this.
//
// A slow pull for item A must not paint over item B (gen). Closing or
// stepping cancels the pull. An image past rasterCap, or a video past
// videoCap, is an error, not a partial decode. A failed video pull deletes
// its temp file; switching items deletes the previous one. Playback is the
// system player: this UI has no video widget.
type viewer struct {
	open    bool
	gen     int
	items   []viewerItem
	index   int
	loading bool
	err     string
	name    string
	kind    string
	gif     bool
	bmp     *ui.Bitmap
	svg     *ui.SVG
	pixels  image.Image
	zoom    float32 // 0 fits the stage; otherwise a fraction of the pixel size
	rot     int     // quarter turns clockwise
	temp    string
	played  bool
	got     int64
	total   int64
	full    bool
	cancel  context.CancelFunc
}

type viewerItem struct {
	name   string
	remote string
	kind   string
	size   uint32
}

func canViewLarge(kind string) bool {
	return kind == kindImage || kind == kindSVG || kind == kindVideo
}

func (a *app) openViewer(name string) {
	p := a.focus()
	rows := a.visibleOf(p)
	items := viewerItems(rows, p.path)
	idx := -1
	for i, it := range items {
		if it.name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	a.viewer.open = true
	a.viewer.items = items
	a.viewer.index = idx
	a.loadViewerItem()
}

func viewerItems(rows []adb.Entry, dir string) []viewerItem {
	var out []viewerItem
	for _, e := range rows {
		kind := previewKind(e)
		if !canViewLarge(kind) {
			continue
		}
		out = append(out, viewerItem{
			name:   e.Name,
			remote: joinPath(dir, e.Name),
			kind:   kind,
			size:   e.Size,
		})
	}
	return out
}

func (a *app) loadViewerItem() {
	if a.viewer.cancel != nil {
		a.viewer.cancel()
		a.viewer.cancel = nil
	}
	a.removeViewerFile(true)
	v := &a.viewer
	if v.index < 0 || v.index >= len(v.items) {
		return
	}
	it := v.items[v.index]
	v.gen++
	gen := v.gen
	v.loading = true
	v.err = ""
	v.bmp = nil
	v.svg = nil
	v.pixels = nil
	v.name = it.name
	v.kind = it.kind
	v.gif = hasExt(it.name, ".gif")
	v.zoom = 0
	v.rot = 0
	v.got = 0
	v.total = int64(it.size)
	timeout := 12 * time.Second
	if it.kind == kindVideo {
		timeout = 3 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	v.cancel = cancel
	serial := a.serial
	go a.fetchViewer(ctx, gen, serial, it)
}

func (a *app) fetchViewer(ctx context.Context, gen int, serial string, it viewerItem) {
	switch it.kind {
	case kindImage:
		a.fetchViewerImage(ctx, gen, serial, it)
	case kindSVG:
		a.fetchViewerSVG(ctx, gen, serial, it)
	default:
		a.fetchViewerVideo(ctx, gen, serial, it)
	}
}

func (a *app) viewerOK(gen int) bool {
	return a.viewer.open && a.viewer.gen == gen
}

func (a *app) failViewer(gen int, err string) {
	a.winUpdate(func() {
		if !a.viewerOK(gen) {
			return
		}
		a.viewer.loading = false
		a.viewer.err = err
	})
}

func (a *app) fetchViewerImage(ctx context.Context, gen int, serial string, it viewerItem) {
	if it.size > rasterCap {
		size := int64(it.size)
		a.winUpdate(func() {
			if !a.viewerOK(gen) {
				return
			}
			a.viewer.loading = false
			a.viewer.err = a.trf("Image too large to preview (%s)", sizeBytes(size))
		})
		return
	}
	data, _, err := a.pullCapped(ctx, serial, it.remote, rasterCap)
	if err != nil {
		a.failViewer(gen, err.Error())
		return
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || img == nil {
		a.winUpdate(func() {
			if !a.viewerOK(gen) {
				return
			}
			a.viewer.loading = false
			a.viewer.err = a.tr("Could not decode image")
		})
		return
	}
	if format == "jpeg" {
		if o := jpegOrientation(data); o > 1 {
			img = applyOrientation(img, o)
		}
	}
	a.winUpdate(func() {
		if !a.viewerOK(gen) {
			return
		}
		a.viewer.loading = false
		a.viewer.pixels = img
		a.viewer.bmp = ui.NewBitmap(img)
	})
}

func (a *app) fetchViewerSVG(ctx context.Context, gen int, serial string, it viewerItem) {
	if it.size > svgCap {
		size := int64(it.size)
		a.winUpdate(func() {
			if !a.viewerOK(gen) {
				return
			}
			a.viewer.loading = false
			a.viewer.err = a.trf("Image too large to preview (%s)", sizeBytes(size))
		})
		return
	}
	data, _, err := a.pullCapped(ctx, serial, it.remote, svgCap)
	if err != nil {
		a.failViewer(gen, err.Error())
		return
	}
	svg, err := ui.ParseSVG(data)
	if err != nil || svg == nil {
		a.winUpdate(func() {
			if !a.viewerOK(gen) {
				return
			}
			a.viewer.loading = false
			a.viewer.err = a.tr("Could not decode image")
		})
		return
	}
	a.winUpdate(func() {
		if !a.viewerOK(gen) {
			return
		}
		a.viewer.loading = false
		a.viewer.svg = svg
	})
}

func (a *app) fetchViewerVideo(ctx context.Context, gen int, serial string, it viewerItem) {
	if it.size > videoCap {
		size := int64(it.size)
		a.winUpdate(func() {
			if !a.viewerOK(gen) {
				return
			}
			a.viewer.loading = false
			a.viewer.err = a.trf("Video is too large to open (%s)", sizeBytes(size))
		})
		return
	}
	f, err := os.CreateTemp("", "adbexplorer-*"+viewerExt(it.name))
	if err != nil {
		a.failViewer(gen, err.Error())
		return
	}
	tmp := f.Name()
	keep := false
	defer func() {
		f.Close()
		if !keep {
			os.Remove(tmp)
		}
	}()
	var n int64
	if a.previewLocal != nil {
		data, ok := a.previewLocal[it.remote]
		if !ok {
			a.failViewer(gen, "no preview for "+it.remote)
			return
		}
		if len(data) > videoCap {
			size := int64(len(data))
			a.winUpdate(func() {
				if !a.viewerOK(gen) {
					return
				}
				a.viewer.loading = false
				a.viewer.err = a.trf("Video is too large to open (%s)", sizeBytes(size))
			})
			return
		}
		if _, err = f.Write(data); err != nil {
			a.failViewer(gen, err.Error())
			return
		}
		n = int64(len(data))
	} else {
		n, err = a.client().PullProgress(ctx, serial, it.remote, f, func(got int64) {
			a.winUpdate(func() {
				if a.viewer.gen != gen {
					return
				}
				a.viewer.got = got
			})
		})
		if err != nil {
			a.failViewer(gen, err.Error())
			return
		}
	}
	if err = f.Close(); err != nil {
		a.failViewer(gen, err.Error())
		return
	}
	keep = true
	a.winUpdate(func() {
		if !a.viewerOK(gen) {
			os.Remove(tmp)
			return
		}
		a.viewer.loading = false
		a.viewer.temp = tmp
		a.viewer.got = n
		if a.viewer.total == 0 {
			a.viewer.total = n
		}
	})
}

func (a *app) removeViewerFile(force bool) {
	if a.viewer.temp == "" {
		return
	}
	if force || !a.viewer.played {
		os.Remove(a.viewer.temp)
	}
	a.viewer.temp = ""
	a.viewer.played = false
}

func (a *app) closeViewer() {
	if a.viewer.cancel != nil {
		a.viewer.cancel()
		a.viewer.cancel = nil
	}
	if a.win != nil && a.viewer.full {
		a.win.SetFullScreen(false)
	}
	a.viewer.full = false
	a.removeViewerFile(false)
	a.viewer.open = false
	a.viewer.loading = false
	a.viewer.bmp = nil
	a.viewer.svg = nil
	a.viewer.pixels = nil
	a.viewer.err = ""
}

func (a *app) viewerStep(d int) {
	n := len(a.viewer.items)
	if n == 0 {
		return
	}
	next := a.viewer.index + d
	if next < 0 || next >= n {
		return
	}
	a.viewer.index = next
	a.loadViewerItem()
}

func (a *app) viewerZoom(dir int) {
	if a.viewer.bmp == nil && a.viewer.svg == nil {
		return
	}
	steps := []float32{0.25, 0.5, 0.75, 1, 1.5, 2, 3, 4, 6}
	z := a.viewer.zoom
	if z == 0 {
		if dir > 0 {
			a.viewer.zoom = 1.5
		} else {
			a.viewer.zoom = 0.5
		}
		return
	}
	if dir > 0 {
		for _, s := range steps {
			if s > z+0.01 {
				a.viewer.zoom = s
				return
			}
		}
		return
	}
	prev := float32(0)
	found := false
	for _, s := range steps {
		if s < z-0.01 {
			prev = s
			found = true
		}
	}
	if !found {
		a.viewer.zoom = 0
		return
	}
	a.viewer.zoom = prev
}

func (a *app) viewerRotate(dir int) {
	if a.viewer.pixels == nil {
		return
	}
	a.viewer.rot += dir
	a.viewer.rot %= 4
	if a.viewer.rot < 0 {
		a.viewer.rot += 4
	}
	a.viewer.bmp = ui.NewBitmap(rotateImage(a.viewer.pixels, a.viewer.rot))
}

func (a *app) toggleViewerFull() {
	if a.win == nil {
		return
	}
	on := !a.win.IsFullScreen()
	a.win.SetFullScreen(on)
	a.viewer.full = on
}

func (a *app) playViewer() {
	if a.viewer.temp == "" {
		return
	}
	a.viewer.played = true
	if err := launchFile(a.viewer.temp); err != nil {
		a.viewer.played = false
		a.viewer.err = a.tr("Could not open the file")
	}
}

func (a *app) viewerOverlay(c *ui.Context) {
	if !a.viewer.open {
		return
	}
	ink := ui.Hex("#101010")
	bar := ui.Hex("#1a1a1a")
	paper := ui.Hex("#f5f5f5")
	muted := ui.Hex("#c6c6c6")
	ui.Overlay(c, func() {
		shell := ui.Column(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Background(ink).Modal()
		_ = shell.Clicked()
		if shell.OverlayShortcut(0, ui.KeyEscape) {
			a.closeViewer()
		}
		if shell.OverlayShortcut(0, ui.KeyLeft) {
			a.viewerStep(-1)
		}
		if shell.OverlayShortcut(0, ui.KeyRight) {
			a.viewerStep(1)
		}
		if shell.OverlayShortcut(0, ui.KeyMinus) {
			a.viewerZoom(-1)
		}
		if shell.OverlayShortcut(0, ui.KeyEqual) {
			a.viewerZoom(1)
		}
		if shell.OverlayShortcut(0, ui.Key0) {
			a.viewer.zoom = 0
		}
		if shell.OverlayShortcut(0, ui.Key1) {
			a.viewer.zoom = 1
		}
		if shell.OverlayShortcut(0, ui.KeyF11) {
			a.toggleViewerFull()
		}
		shell.Children(func() {
			a.viewerTop(c, bar, paper, muted)
			a.viewerStage(c, paper, muted)
			a.viewerBottom(c, bar)
		})
	})
}

func (a *app) viewerTop(c *ui.Context, bar, paper, muted ui.Color) {
	v := a.viewer
	ui.Row(c).Height(48).Padding(0, 12).Gap(12).AlignItems(ui.Center).Background(bar).Children(func() {
		ui.Text(c, v.name).Bold().Grow(1).SingleLine().TextColor(paper)
		if v.gif {
			ui.Text(c, a.tr("GIF · first frame")).FontSize(12).TextColor(muted)
		}
		if v.zoom > 0 {
			ui.Text(c, fmt.Sprintf("%d%%", int(v.zoom*100+0.5))).FontSize(12).TextColor(muted)
		}
		if n := len(v.items); n > 0 {
			ui.Text(c, a.trf("%d / %d", v.index+1, n)).FontSize(12).TextColor(muted)
		}
		if ui.Button(c, a.tr("Close")).Clicked() {
			a.closeViewer()
		}
	})
}

func (a *app) viewerStage(c *ui.Context, paper, muted ui.Color) {
	v := a.viewer
	ui.Box(c).Grow(1).Clip().Children(func() {
		switch {
		case v.loading:
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Spinner(c)
				ui.Text(c, a.tr("Loading preview…")).TextColor(muted)
				if v.kind == kindVideo && v.total > 0 {
					ui.Text(c, sizeBytes(v.got)+" / "+sizeBytes(v.total)).TextColor(muted)
				}
			})
		case v.err != "":
			ui.Column(c).Grow(1).Center().Padding(24).Children(func() {
				ui.Text(c, v.err).TextColor(ui.Hex("#ffb4b4"))
			})
		case v.kind == kindVideo:
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Text(c, a.tr("Video")).FontSize(22).Bold().TextColor(paper)
				ui.Text(c, sizeBytes(v.total)).TextColor(muted)
				ui.Text(c, a.tr("Video plays in the system player")).TextColor(muted)
				if v.temp != "" && ui.PrimaryButton(c, a.tr("Play")).Clicked() {
					a.playViewer()
				}
			})
		case v.zoom != 0 && (v.bmp != nil || v.svg != nil):
			ui.ScrollBoth(c).Grow(1).Children(func() {
				a.viewerPicture(c, v.zoom)
			})
		default:
			if v.bmp != nil || v.svg != nil {
				pic := a.viewerPicture(c, 0)
				pic.Absolute().Left(16).Right(16).Top(8).Bottom(8).Fit(ui.Contain)
			}
		}
	})
}

func (a *app) viewerPicture(c *ui.Context, zoom float32) ui.Element {
	if a.viewer.bmp != nil {
		img := ui.Image(c, a.viewer.bmp)
		if zoom > 0 {
			w, h := a.viewer.bmp.Size()
			img.Size(float32(w)*zoom, float32(h)*zoom)
		}
		return img
	}
	img := ui.Image(c, a.viewer.svg)
	if zoom > 0 {
		w, h := a.viewer.svg.Size()
		zw, zh := w*zoom, h*zoom
		if zw < 1 {
			zw = 1
		}
		if zh < 1 {
			zh = 1
		}
		img.Size(zw, zh)
	}
	return img
}

func (a *app) viewerBottom(c *ui.Context, bar ui.Color) {
	v := a.viewer
	canZoom := !v.loading && v.err == "" && (v.bmp != nil || v.svg != nil)
	canRotate := !v.loading && v.err == "" && v.pixels != nil
	ui.Row(c).Height(52).Padding(0, 12).Gap(6).AlignItems(ui.Center).Justify(ui.Center).Background(bar).Children(func() {
		if ui.Button(c, a.tr("Previous")).Disabled(v.index <= 0).Clicked() {
			a.viewerStep(-1)
		}
		if ui.Button(c, a.tr("Next")).Disabled(v.index+1 >= len(v.items)).Clicked() {
			a.viewerStep(1)
		}
		if ui.Button(c, a.tr("Zoom out")).Disabled(!canZoom).Clicked() {
			a.viewerZoom(-1)
		}
		if ui.Button(c, a.tr("Zoom in")).Disabled(!canZoom).Clicked() {
			a.viewerZoom(1)
		}
		if ui.Button(c, a.tr("Fit window")).Disabled(!canZoom).Clicked() {
			a.viewer.zoom = 0
		}
		if ui.Button(c, a.tr("Actual size")).Disabled(!canZoom).Clicked() {
			a.viewer.zoom = 1
		}
		if ui.Button(c, a.tr("Rotate left")).Disabled(!canRotate).Clicked() {
			a.viewerRotate(-1)
		}
		if ui.Button(c, a.tr("Rotate right")).Disabled(!canRotate).Clicked() {
			a.viewerRotate(1)
		}
		if ui.Button(c, a.tr("Full screen")).Clicked() {
			a.toggleViewerFull()
		}
	})
}

func viewerExt(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if len(ext) < 2 || len(ext) > 8 {
		return ".bin"
	}
	for _, r := range ext[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ".bin"
		}
	}
	return ext
}

// jpegOrientation reads a JPEG EXIF orientation, 1 to 8, or 0 when absent.
func jpegOrientation(jpg []byte) int {
	if len(jpg) < 4 || jpg[0] != 0xff || jpg[1] != 0xd8 {
		return 0
	}
	for i := 2; i+4 <= len(jpg); {
		if jpg[i] != 0xff {
			return 0
		}
		marker, size := jpg[i+1], int(binary.BigEndian.Uint16(jpg[i+2:]))
		if marker == 0xda || size < 2 || i+2+size > len(jpg) {
			return 0
		}
		seg := jpg[i+4 : i+2+size]
		if marker == 0xe1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrient(seg[6:])
		}
		i += 2 + size
	}
	return 0
}

func tiffOrient(t []byte) int {
	if len(t) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	ifd := int(bo.Uint32(t[4:]))
	if ifd < 8 || ifd+2 > len(t) {
		return 0
	}
	n := int(bo.Uint16(t[ifd:]))
	for e := ifd + 2; e+12 <= len(t) && n > 0; e, n = e+12, n-1 {
		if bo.Uint16(t[e:]) == 0x0112 && bo.Uint16(t[e+2:]) == 3 {
			if o := int(bo.Uint16(t[e+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 0
		}
	}
	return 0
}

// applyOrientation turns src the way EXIF orientation o says a camera stored it.
func applyOrientation(src image.Image, o int) image.Image {
	if src == nil || o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx, sy := x, y
			switch o {
			case 2:
				sx, sy = w-1-x, y
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			}
			dst.Set(x, y, src.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return dst
}

func rotateImage(src image.Image, turns int) image.Image {
	switch ((turns % 4) + 4) % 4 {
	case 1:
		return applyOrientation(src, 6)
	case 2:
		return applyOrientation(src, 3)
	case 3:
		return applyOrientation(src, 8)
	default:
		return src
	}
}

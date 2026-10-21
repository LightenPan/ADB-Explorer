package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"adbexplorer/internal/adb"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	kindFolder  = "Folder"
	kindImage   = "Image"
	kindSVG     = "SVG"
	kindPDF     = "PDF"
	kindHEIC    = "HEIC"
	kindAPK     = "APK"
	kindArchive = "Archive"
	kindAudio   = "Audio"
	kindVideo   = "Video"
	kindText    = "Text"
	kindBinary  = "Binary"
	kindLink    = "Link"

	rasterCap = 8 * 1024 * 1024
	svgCap    = 1 * 1024 * 1024
	pdfCap    = 8 * 1024 * 1024
	apkCap    = 8 * 1024 * 1024
	hexCap    = 4 * 1024
	headCap   = 64 * 1024
	heicCap   = 64
	reuseIcon = 256 * 1024
)

var errPreviewLimit = errors.New("preview limit")

type limitedWriter struct {
	W io.Writer
	N int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.N <= 0 {
		return 0, errPreviewLimit
	}
	if len(p) > w.N {
		p = p[:w.N]
	}
	n, err := w.W.Write(p)
	w.N -= n
	if w.N <= 0 {
		return n, errPreviewLimit
	}
	return n, err
}

func (a *app) showDetail() {
	p := a.focus()
	sel := selectedOf(p, a.visibleOf(p))
	if len(sel) != 1 {
		a.detail.err = a.tr("Select one item")
		a.detailOpen = true
		return
	}
	e := sel[0]
	remote := joinPath(p.path, e.Name)
	a.detailOpen = true
	a.detail.gen++
	gen := a.detail.gen
	kind := previewKind(e)
	a.detail = detail{
		path:    remote,
		name:    e.Name,
		kind:    kind,
		size:    sizeOf(e),
		sizeN:   int64(e.Size),
		mod:     e.MTime.Format("2006-01-02 15:04"),
		mode:    modeString(e.Mode),
		gen:     gen,
		loading: !e.IsDir(),
		canEdit: !e.IsDir() && e.Size <= editLimit && (kind == kindText || kind == kindBinary || kind == kindSVG),
	}
	if e.IsDir() {
		if remote == p.path {
			a.detail.preview = a.trf("%d items", len(p.entries))
		}
		return
	}
	if a.serial == "" && a.previewLocal == nil {
		a.detail.loading = false
		a.detail.err = a.tr("No device")
		return
	}
	serial := a.serial
	size := e.Size
	mtime := e.MTime
	go a.loadPreview(gen, serial, remote, e.Name, kind, size, mtime)
}

func (a *app) syncDetail() {
	if !a.detailOpen {
		return
	}
	p := a.focus()
	sel := selectedOf(p, a.visibleOf(p))
	if len(sel) != 1 {
		a.detailOpen = false
		return
	}
	remote := joinPath(p.path, sel[0].Name)
	if remote != a.detail.path {
		a.showDetail()
	}
}

func (a *app) loadPreview(gen int, serial, remote, name, kind string, size uint32, mtime time.Time) {
	timeout := 8 * time.Second
	if kind == kindImage || kind == kindPDF || kind == kindAPK {
		timeout = 12 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch kind {
	case kindImage:
		a.previewRaster(ctx, gen, serial, remote, name, size, mtime)
	case kindSVG:
		a.previewSVG(ctx, gen, serial, remote, name, size)
	case kindPDF:
		a.previewPDF(ctx, gen, serial, remote, size)
	case kindHEIC:
		a.previewHEIC(ctx, gen, serial, remote)
	case kindAPK:
		a.previewAPK(ctx, gen, serial, remote, name, size, mtime)
	case kindArchive:
		a.previewArchive(ctx, gen, serial, remote, name)
	case kindAudio, kindVideo:
		a.previewMedia(ctx, gen, serial, remote, kind)
	case kindText:
		a.previewText(ctx, gen, serial, remote, name, size)
	default:
		a.previewHexOrText(ctx, gen, serial, remote, name, size)
	}
}

func (a *app) publishDetail(gen int, fn func(*detail)) {
	a.winUpdate(func() {
		if a.detail.gen != gen {
			return
		}
		a.detail.loading = false
		fn(&a.detail)
	})
}

func (a *app) pullCapped(ctx context.Context, serial, remote string, cap int) ([]byte, bool, error) {
	if cap <= 0 {
		return nil, false, nil
	}
	if a.previewLocal != nil {
		data, ok := a.previewLocal[remote]
		if !ok {
			return nil, false, fmt.Errorf("no preview for %s", remote)
		}
		trunc := false
		if len(data) > cap {
			data = data[:cap]
			trunc = true
		}
		return append([]byte(nil), data...), trunc, nil
	}
	var buf bytes.Buffer
	_, err := a.client().Pull(ctx, serial, remote, &limitedWriter{W: &buf, N: cap})
	trunc := errors.Is(err, errPreviewLimit)
	if err != nil && !trunc {
		return buf.Bytes(), false, err
	}
	return buf.Bytes(), trunc, nil
}

func (a *app) shellPreview(ctx context.Context, serial, cmd string) (string, error) {
	if a.previewShell != nil {
		if out, ok := a.previewShell[cmd]; ok {
			return out, nil
		}
		return "", fmt.Errorf("no shell preview for %s", cmd)
	}
	return a.client().Shell(ctx, serial, cmd)
}

func (a *app) previewRaster(ctx context.Context, gen int, serial, remote, name string, size uint32, mtime time.Time) {
	if size > rasterCap {
		a.publishDetail(gen, func(d *detail) {
			d.err = a.trf("Image too large to preview (%s)", sizeBytes(int64(size)))
		})
		return
	}
	if size <= reuseIcon {
		if bmp := a.iconOf(remote); bmp != nil {
			a.publishDetail(gen, func(d *detail) {
				d.bitmaps = []*ui.Bitmap{bmp}
				if hasExt(name, ".gif") {
					d.caption = a.tr("GIF · first frame")
				}
			})
			return
		}
	}
	if hit, ok := previewCacheGet(remote, size, mtime); ok && len(hit.bitmaps) > 0 {
		a.publishDetail(gen, func(d *detail) {
			d.bitmaps = hit.bitmaps
			if hasExt(name, ".gif") {
				d.caption = a.tr("GIF · first frame")
			}
		})
		return
	}
	data, _, err := a.pullCapped(ctx, serial, remote, rasterCap)
	if err != nil {
		a.publishDetail(gen, func(d *detail) { d.err = err.Error() })
		return
	}
	bmp, err := ui.DecodeBitmap(data)
	if err != nil || bmp == nil {
		a.publishDetail(gen, func(d *detail) { d.err = a.tr("Could not decode image") })
		return
	}
	previewCachePut(remote, size, mtime, previewHit{bitmaps: []*ui.Bitmap{bmp}})
	a.publishDetail(gen, func(d *detail) {
		d.bitmaps = []*ui.Bitmap{bmp}
		if hasExt(name, ".gif") {
			d.caption = a.tr("GIF · first frame")
		}
	})
}

func (a *app) previewSVG(ctx context.Context, gen int, serial, remote, name string, size uint32) {
	if size > svgCap {
		a.publishDetail(gen, func(d *detail) {
			d.err = a.trf("Image too large to preview (%s)", sizeBytes(int64(size)))
		})
		return
	}
	data, _, err := a.pullCapped(ctx, serial, remote, svgCap)
	if err != nil {
		a.publishDetail(gen, func(d *detail) { d.err = err.Error() })
		return
	}
	svg, err := ui.ParseSVG(data)
	if err != nil || svg == nil {
		a.applyTextBytes(gen, name, data, int64(size), false)
		return
	}
	a.publishDetail(gen, func(d *detail) {
		d.svg = svg
		d.svgOwn = svgHasOwnColors(data)
		d.caption = a.tr("SVG · paths only")
		d.bytes = data
		d.canEdit = int64(size) <= editLimit
	})
}

func (a *app) previewPDF(ctx context.Context, gen int, serial, remote string, size uint32) {
	if !a.previewPDFEnabled() {
		a.publishDetail(gen, func(d *detail) { d.err = a.tr("PDF preview unavailable") })
		return
	}
	if size > pdfCap {
		a.publishDetail(gen, func(d *detail) {
			d.err = a.trf("File is too large to preview (%s)", sizeBytes(int64(size)))
		})
		return
	}
	data, _, err := a.pullCapped(ctx, serial, remote, pdfCap)
	if err != nil {
		a.publishDetail(gen, func(d *detail) { d.err = err.Error() })
		return
	}
	a.renderPDF(ctx, gen, data, "")
}

func (a *app) retryPDF() {
	d := a.detail
	if d.kind != kindPDF || len(d.bytes) == 0 {
		return
	}
	gen := d.gen
	data := append([]byte(nil), d.bytes...)
	password := d.password
	a.detail.loading = true
	a.detail.err = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		a.renderPDF(ctx, gen, data, password)
	}()
}

func (a *app) previewHEIC(ctx context.Context, gen int, serial, remote string) {
	_, _, _ = a.pullCapped(ctx, serial, remote, heicCap)
	a.publishDetail(gen, func(d *detail) {
		d.preview = a.tr("HEIC camera image — preview is not available")
	})
}

func (a *app) previewAPK(ctx context.Context, gen int, serial, remote, name string, size uint32, mtime time.Time) {
	quoted := shellQuote(remote)
	listOut, listErr := a.shellPreview(ctx, serial, "unzip -l "+quoted)
	manOut, _ := a.shellPreview(ctx, serial, "unzip -p "+quoted+" META-INF/MANIFEST.MF")
	info := a.formatAPKInfo(listOut, manOut)
	if listErr != nil && strings.TrimSpace(listOut) == "" {
		info = listErr.Error()
	}

	var icon *ui.Bitmap
	if bmp := a.iconOf(remote); bmp != nil {
		icon = bmp
	} else if size <= apkCap {
		data, _, err := a.pullCapped(ctx, serial, remote, apkCap)
		if err == nil {
			if png, ok := iconFor(data); ok {
				if bmp, err := ui.DecodeBitmap(png); err == nil && bmp != nil {
					icon = bmp
					setFileIcon(remote, bmp)
				}
			}
		}
	}
	_ = mtime
	a.publishDetail(gen, func(d *detail) {
		d.preview = info
		if icon != nil {
			d.bitmaps = []*ui.Bitmap{icon}
		}
		if listErr != nil && d.preview == listErr.Error() {
			d.err = listErr.Error()
			d.preview = ""
		}
	})
}

func (a *app) previewArchive(ctx context.Context, gen int, serial, remote, name string) {
	low := strings.ToLower(name)
	quoted := shellQuote(remote)
	var cmd string
	zip := zipFamily(low) || apkFamily(low)
	if zip {
		cmd = "unzip -l " + quoted
	} else {
		cmd = tarListCmd(low, quoted)
	}
	out, err := a.shellPreview(ctx, serial, cmd)
	if err != nil {
		a.publishDetail(gen, func(d *detail) { d.err = err.Error() })
		return
	}
	rows := archiveEntries(out, zip)
	text := formatArchiveTOC(rows, 200)
	a.publishDetail(gen, func(d *detail) { d.preview = text })
}

func (a *app) previewMedia(ctx context.Context, gen int, serial, remote, kind string) {
	data, _, err := a.pullCapped(ctx, serial, remote, headCap)
	if err != nil {
		a.publishDetail(gen, func(d *detail) { d.err = err.Error() })
		return
	}
	label := a.tr(kind + " · not playable in preview")
	if extra := mediaHint(data, kind); extra != "" {
		label += "\n" + extra
	}
	a.publishDetail(gen, func(d *detail) {
		d.preview = label
		d.bytes = data
	})
}

func (a *app) previewText(ctx context.Context, gen int, serial, remote, name string, size uint32) {
	cap := a.maxPreviewBytes()
	if int(size) >= cap {
		a.publishDetail(gen, func(d *detail) { d.err = a.tr("File is too large to preview") })
		return
	}
	data, trunc, err := a.pullCapped(ctx, serial, remote, cap)
	if err != nil {
		a.publishDetail(gen, func(d *detail) { d.err = err.Error() })
		return
	}
	a.applyTextBytes(gen, name, data, int64(size), trunc)
}

func (a *app) previewHexOrText(ctx context.Context, gen int, serial, remote, name string, size uint32) {
	data, _, err := a.pullCapped(ctx, serial, remote, hexCap)
	if err != nil {
		a.publishDetail(gen, func(d *detail) { d.err = err.Error() })
		return
	}
	if looksHEIC(data) {
		a.publishDetail(gen, func(d *detail) {
			d.kind = kindHEIC
			d.preview = a.tr("HEIC camera image — preview is not available")
		})
		return
	}
	if bytesContainNUL(data) || !printableBytes(data) {
		a.publishDetail(gen, func(d *detail) {
			d.kind = kindBinary
			d.bytes = data
			d.preview = hexDump(data)
			d.caption = a.tr("Binary · first 4 KB")
			d.canEdit = int64(size) <= editLimit
		})
		return
	}
	a.applyTextBytes(gen, name, data, int64(size), int(size) > len(data))
}

func (a *app) applyTextBytes(gen int, name string, data []byte, size int64, trunc bool) {
	if bytesContainNUL(data) {
		a.publishDetail(gen, func(d *detail) {
			d.kind = kindBinary
			d.bytes = data
			d.hexMode = true
			d.preview = hexDump(data)
			d.caption = a.tr("Binary · first 4 KB")
			d.canEdit = size <= editLimit
		})
		return
	}
	text := decodeText(data)
	st := &editor.State{Buffer: editor.NewBuffer(text), ReadOnly: true, SoftWrap: true}
	if lang := editor.LanguageFor(name); lang != nil {
		st.Buffer.SetLanguage(lang)
	}
	langName := a.tr("Plain text")
	if st.Buffer.Language() != nil {
		langName = st.Buffer.Language().Name
	}
	caption := langName
	if trunc || size > int64(len(data)) {
		caption = a.trf("%s · showing first %d KB", langName, (len(data)+1023)/1024)
	}
	a.publishDetail(gen, func(d *detail) {
		d.kind = kindText
		d.bytes = data
		d.edit = st
		d.lang = langName
		d.caption = caption
		d.preview = ""
		d.canEdit = size <= editLimit
	})
}

func (a *app) toggleHex() {
	d := &a.detail
	if len(d.bytes) == 0 {
		return
	}
	d.hexMode = !d.hexMode
	if d.hexMode {
		d.edit = nil
		d.preview = hexDump(d.bytes)
		if d.caption == "" || d.kind == kindText {
			d.caption = d.lang
			if d.caption == "" {
				d.caption = a.tr("Hex")
			} else {
				d.caption = a.trf("%s · hex", d.caption)
			}
		}
		return
	}
	text := decodeText(d.bytes)
	st := &editor.State{Buffer: editor.NewBuffer(text), ReadOnly: true, SoftWrap: true}
	if lang := editor.LanguageFor(d.name); lang != nil {
		st.Buffer.SetLanguage(lang)
	}
	d.edit = st
	d.preview = ""
	if d.lang != "" {
		d.caption = d.lang
	} else {
		d.caption = a.tr("Plain text")
	}
}

func previewKind(e adb.Entry) string {
	if e.IsDir() {
		if e.IsLink() {
			return kindLink
		}
		return kindFolder
	}
	name := e.Name
	low := strings.ToLower(name)
	switch {
	case hasExt(low, ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp"):
		return kindImage
	case hasExt(low, ".svg"):
		return kindSVG
	case hasExt(low, ".pdf"):
		return kindPDF
	case hasExt(low, ".heic", ".heif"):
		return kindHEIC
	case apkFamily(low):
		return kindAPK
	case zipFamily(low) || tarFamily(low):
		return kindArchive
	case hasExt(low, ".mp3", ".aac", ".ogg", ".oga", ".opus", ".wav", ".flac", ".m4a", ".amr"):
		return kindAudio
	case hasExt(low, ".mp4", ".webm", ".3gp", ".mkv", ".mov", ".avi"):
		return kindVideo
	case knownTextName(name):
		return kindText
	default:
		return kindBinary
	}
}

func apkFamily(name string) bool {
	return hasExt(name, ".apk", ".xapk", ".apks", ".apkm", ".apex")
}

func knownTextName(name string) bool {
	base := path.Base(name)
	low := strings.ToLower(base)
	switch low {
	case "makefile", "dockerfile", "containerfile", "build.prop", ".bashrc", ".zshrc", ".profile", ".bash_profile", "readme", "changelog", "gemfile", "rakefile":
		return true
	}
	if editor.LanguageFor(base) != nil {
		return true
	}
	return hasExt(low, ".txt", ".log", ".text", ".out", ".prop", ".rc", ".csv", ".tsv", ".smali")
}

func decodeText(b []byte) string {
	if len(b) >= 3 && b[0] == 0xef && b[1] == 0xbb && b[2] == 0xbf {
		b = b[3:]
	}
	if !utf8.Valid(b) {
		return string(b)
	}
	return string(b)
}

func bytesContainNUL(b []byte) bool {
	return bytes.IndexByte(b, 0) >= 0
}

func printable(s string) bool { return printableBytes([]byte(s)) }

func printableBytes(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if bytesContainNUL(b) {
		return false
	}
	bad := 0
	for _, r := range string(b) {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			bad++
		}
	}
	return bad*8 < len(b)
}

func svgHasOwnColors(data []byte) bool {
	low := strings.ToLower(string(data))
	if !strings.Contains(low, "fill=") && !strings.Contains(low, "stroke=") {
		return false
	}
	return !strings.Contains(low, "currentcolor") || strings.Count(low, "fill=") > strings.Count(low, "currentcolor")
}

func looksHEIC(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	if string(b[4:8]) != "ftyp" {
		return false
	}
	brand := strings.ToLower(string(b[8:12]))
	switch brand {
	case "heic", "heif", "mif1", "msf1":
		return true
	}
	return false
}

func mediaHint(data []byte, kind string) string {
	if kind == kindAudio {
		if h := wavHint(data); h != "" {
			return h
		}
		if t := id3Title(data); t != "" {
			return t
		}
	}
	if kind == kindVideo {
		if b := ftypBrand(data); b != "" {
			return "ftyp " + b
		}
	}
	if b := ftypBrand(data); b != "" {
		return "ftyp " + b
	}
	return ""
}

func wavHint(b []byte) string {
	if len(b) < 36 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return ""
	}
	// fmt chunk typically at 12
	if string(b[12:16]) != "fmt " {
		return ""
	}
	ch := int(b[22]) | int(b[23])<<8
	rate := int(b[24]) | int(b[25])<<8 | int(b[26])<<16 | int(b[27])<<24
	if ch <= 0 || rate <= 0 {
		return ""
	}
	return fmt.Sprintf("%d Hz · %d ch", rate, ch)
}

func id3Title(b []byte) string {
	if len(b) < 10 || string(b[0:3]) != "ID3" {
		return ""
	}
	// ID3v2: look for TIT2 or TT2 in the prefix
	if i := bytes.Index(b, []byte("TIT2")); i >= 0 && i+10 < len(b) {
		size := int(b[i+4])<<24 | int(b[i+5])<<16 | int(b[i+6])<<8 | int(b[i+7])
		start := i + 10
		if b[i+9]&1 == 1 {
			start++
		}
		if start < len(b) && size > 0 {
			end := start + size - 1
			if end > len(b) {
				end = len(b)
			}
			s := strings.Trim(string(b[start:end]), " \x00")
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func ftypBrand(b []byte) string {
	if len(b) < 12 || string(b[4:8]) != "ftyp" {
		return ""
	}
	return strings.TrimRight(string(b[8:12]), " ")
}

func formatArchiveTOC(rows []adb.Entry, cap int) string {
	if cap <= 0 {
		cap = 200
	}
	var b strings.Builder
	n := len(rows)
	if n > cap {
		n = cap
	}
	for i := 0; i < n; i++ {
		e := rows[i]
		if e.IsDir() {
			fmt.Fprintf(&b, "%s/\n", e.Name)
			continue
		}
		if e.Size > 0 {
			fmt.Fprintf(&b, "%s\t%s\n", e.Name, sizeBytes(int64(e.Size)))
		} else {
			fmt.Fprintf(&b, "%s\n", e.Name)
		}
	}
	if len(rows) > cap {
		fmt.Fprintf(&b, "… %d more\n", len(rows)-cap)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (a *app) formatAPKInfo(listOut, manOut string) string {
	rows := archiveEntries(listOut, true)
	body := formatArchiveTOC(rows, 30)
	man := strings.TrimSpace(manOut)
	if man != "" {
		lines := strings.Split(man, "\n")
		if len(lines) > 40 {
			lines = lines[:40]
			man = strings.Join(lines, "\n")
		}
		if body != "" {
			body += "\n\n"
		}
		body += man
	}
	if body == "" {
		return a.tr("No APK info")
	}
	return body
}

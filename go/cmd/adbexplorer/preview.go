package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

const editLimit = 512 * 1024

var (
	editState *editor.State
	editPath  string
	editHex   string
	editMode  string // "text" or "hex" or ""
)

func (a *app) openEditor() {
	p := a.focus()
	sel := selectedOf(p, a.visibleOf(p))
	if len(sel) != 1 || sel[0].IsDir() {
		a.status = a.tr("Select one file")
		return
	}
	e := sel[0]
	if e.Size > editLimit {
		a.status = a.tr("File is too large to open")
		return
	}
	path := joinPath(p.path, e.Name)
	if a.previewLocal != nil {
		data, ok := a.previewLocal[path]
		if !ok {
			a.status = a.tr("No device")
			return
		}
		a.applyEditorBytes(path, data)
		return
	}
	if a.serial == "" {
		a.status = a.tr("No device")
		return
	}
	serial := a.serial
	go func() {
		var buf bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := a.client().Pull(ctx, serial, path, &buf)
		a.winUpdate(func() {
			if err != nil {
				a.status = err.Error()
				return
			}
			a.applyEditorBytes(path, buf.Bytes())
		})
	}()
}

func (a *app) applyEditorBytes(path string, data []byte) {
	if printableBytes(data) {
		editMode = "text"
		editPath = path
		editHex = ""
		editState = &editor.State{Buffer: editor.NewBuffer(decodeText(data))}
		return
	}
	editMode = "hex"
	editPath = path
	editState = nil
	editHex = hexDump(data)
}

func (a *app) saveEditor() {
	if editMode != "text" || editState == nil || editState.Buffer == nil || editPath == "" || a.serial == "" {
		a.status = a.tr("Nothing to save")
		return
	}
	text := editState.Buffer.String()
	f, err := os.CreateTemp("", "adbexplorer-*.txt")
	if err != nil {
		a.status = err.Error()
		return
	}
	tmp := f.Name()
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		os.Remove(tmp)
		a.status = err.Error()
		return
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		a.status = err.Error()
		return
	}
	serial, remote := a.serial, editPath
	a.enqueue("upload", remote, int64(len(text)), func(_ *op, ctx context.Context, report func(int64)) error {
		defer os.Remove(tmp)
		if err := a.client().Push(ctx, serial, remote, tmp); err != nil {
			return err
		}
		report(int64(len(text)))
		a.winUpdate(func() { a.status = a.tr("Saved") })
		return nil
	})
}

func (a *app) closeEditor() {
	editMode = ""
	editPath = ""
	editState = nil
	editHex = ""
}

func hexDump(b []byte) string {
	if len(b) > 4096 {
		b = b[:4096]
	}
	var buf bytes.Buffer
	for off := 0; off < len(b); off += 16 {
		end := off + 16
		if end > len(b) {
			end = len(b)
		}
		row := b[off:end]
		fmt.Fprintf(&buf, "%08x  ", off)
		for i := 0; i < 16; i++ {
			if i == 8 {
				buf.WriteByte(' ')
			}
			if i < len(row) {
				fmt.Fprintf(&buf, "%02x ", row[i])
			} else {
				buf.WriteString("   ")
			}
		}
		buf.WriteString(" |")
		for _, c := range row {
			if c < 32 || c > 126 {
				buf.WriteByte('.')
			} else {
				buf.WriteByte(c)
			}
		}
		buf.WriteString("|\n")
	}
	return buf.String()
}

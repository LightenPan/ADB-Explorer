package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"adbexplorer/internal/adb"

	"github.com/egoist/mygo"
)

const (
	opQueued  = "queued"
	opRunning = "running"
	opDone    = "done"
	opFailed  = "failed"
	opCancel  = "cancelled"
)

// op is one transfer. Ops run one at a time so a later upload does not
// cancel the one already moving bytes.
type op struct {
	id     int
	kind   string
	label  string
	state  string
	detail string
	file   string   // file copying now
	names  []string // every file in this transfer
	index  int      // 1-based position in names
	count  int
	done   int64
	total  int64
	speed  int64 // bytes per second, from recent progress
	bpsAt  time.Time
	bpsN   int64
	cancel context.CancelFunc
}

func (o *op) progress() float64 {
	if o.total <= 0 {
		return 0
	}
	f := float64(o.done) / float64(o.total)
	if f > 1 {
		return 1
	}
	return f
}

func (a *app) enqueue(kind, label string, total int64, work func(item *op, ctx context.Context, report func(int64)) error) {
	a.nextOp++
	ctx, cancel := context.WithCancel(context.Background())
	item := &op{
		id:     a.nextOp,
		kind:   kind,
		label:  label,
		state:  opQueued,
		total:  total,
		cancel: cancel,
	}
	a.ops = append(a.ops, item)
	adb.UploadLog("enqueue id=%d kind=%s label=%s total=%d", item.id, kind, label, total)
	if len(a.ops) > 100 {
		a.ops = a.ops[len(a.ops)-100:]
	}
	a.kickOps()
	go a.runOp(item, ctx, work)
}

func (a *app) kickOps() {
	for _, item := range a.ops {
		if item.state == opRunning {
			return
		}
	}
	for _, item := range a.ops {
		if item.state == opQueued {
			item.state = opRunning
			return
		}
	}
}

func (a *app) runOp(item *op, ctx context.Context, work func(*op, context.Context, func(int64)) error) {
	for {
		// Update posts the read to the UI thread and returns before it runs.
		// The answer has to come back on a channel; a local set by that
		// callback is still false when this loop looks at it.
		ch := make(chan bool, 1)
		a.winUpdate(func() { ch <- item.state == opRunning })
		var running bool
		select {
		case running = <-ch:
		case <-ctx.Done():
		}
		adb.UploadLog("runOp id=%d running=%t err=%v", item.id, running, ctx.Err())
		if running || ctx.Err() != nil {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if ctx.Err() != nil {
		a.winUpdate(func() {
			if item.state == opQueued || item.state == opRunning {
				item.state = opCancel
			}
			a.kickOps()
		})
		return
	}
	var last time.Time
	var latest int64
	err := work(item, ctx, func(n int64) {
		atomic.StoreInt64(&latest, n)
		now := time.Now()
		if !last.IsZero() && now.Sub(last) < 150*time.Millisecond {
			return
		}
		last = now
		a.paintProgress(item, atomic.LoadInt64(&latest))
	})
	a.winUpdate(func() {
		cur := atomic.LoadInt64(&latest)
		adb.UploadLog("finish id=%d done=%d latest=%d total=%d", item.id, item.done, cur, item.total)
		if cur > item.done {
			item.done = cur
		}
		a.noteSpeed(item)
		switch {
		case errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
			item.state = opCancel
		case err != nil:
			item.state = opFailed
			item.detail = err.Error()
			a.reportOpError(err)
		default:
			item.state = opDone
			a.note(item.kind + " " + item.label)
			a.status = item.kind + " " + item.label
		}
		a.kickOps()
	})
}

// paintProgress records bytes copied so far. A later sample must not move
// the bar backwards when an older paint runs after a newer one.
func (a *app) paintProgress(item *op, done int64) {
	a.winUpdate(func() {
		adb.UploadLog("paint id=%d done=%d latest=%d total=%d file=%s", item.id, item.done, done, item.total, item.file)
		if done > item.done {
			item.done = done
		}
		a.noteSpeed(item)
	})
}

func (a *app) noteSpeed(item *op) {
	now := time.Now()
	if item.bpsAt.IsZero() {
		item.bpsAt = now
		item.bpsN = item.done
		return
	}
	dt := now.Sub(item.bpsAt)
	if dt < 300*time.Millisecond {
		return
	}
	delta := item.done - item.bpsN
	if delta < 0 {
		delta = 0
	}
	item.speed = delta * int64(time.Second) / int64(dt)
	item.bpsAt = now
	item.bpsN = item.done
}

func (a *app) setOpFile(item *op, name string, index int) {
	a.winUpdate(func() {
		item.file = name
		item.index = index
	})
}

func (a *app) setOpBatch(item *op, names []string, total int64) {
	a.winUpdate(func() {
		item.names = names
		item.count = len(names)
		if total > 0 {
			item.total = total
		}
	})
}

func (a *app) reportOpError(err error) {
	raw := err.Error()
	title := a.tr("Transfer failed")
	message := raw
	if strings.Contains(strings.ToLower(raw), "no space left on device") {
		title = a.tr("Upload failed")
		message = a.tr("The device has no free space, so the upload stopped.")
	}
	a.note(message)
	a.status = message
	adb.UploadLog("op error title=%s message=%s raw=%s", title, message, raw)
	if a.win == nil {
		return
	}
	win := a.win
	ok := a.tr("OK")
	go func() {
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{
			Parent:  win,
			Type:    mygo.MessageError,
			Title:   title,
			Message: message,
			Detail:  raw,
			Buttons: []string{ok},
		})
	}()
}

func (a *app) winUpdate(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}

func (a *app) cancelOp(id int) {
	for _, item := range a.ops {
		if item.id != id {
			continue
		}
		if item.state == opQueued || item.state == opRunning {
			item.cancel()
			if item.state == opQueued {
				item.state = opCancel
			}
		}
	}
	a.kickOps()
}

func (a *app) clearFinished() {
	kept := a.ops[:0]
	for _, item := range a.ops {
		if item.state == opQueued || item.state == opRunning {
			kept = append(kept, item)
		}
	}
	a.ops = kept
}

func (a *app) activeOp() *op {
	for _, item := range a.ops {
		if item.state == opRunning {
			return item
		}
	}
	return nil
}

func (a *app) pull(entries []adb.Entry) {
	if a.serial == "" {
		a.status = a.tr("No device")
		return
	}
	if len(entries) == 0 {
		a.status = a.tr("Select items to copy")
		return
	}
	var chosen []adb.Entry
	var total int64
	for _, e := range entries {
		if !safeChild(e.Name) || e.IsLink() {
			continue
		}
		chosen = append(chosen, e)
		if !e.IsDir() {
			total += int64(e.Size)
		}
	}
	if len(chosen) == 0 {
		a.status = a.tr("Folders are not downloaded")
		return
	}
	label := chosen[0].Name
	if len(chosen) > 1 {
		label = a.trf("%d files", len(chosen))
	}
	serial, remote := a.serial, a.focus().path
	a.status = a.tr("Choose a save folder")
	opts := mygo.OpenDialogOptions{
		Parent:            a.win,
		Title:             a.tr("Save to"),
		Directory:         true,
		CreateDirectories: true,
		DefaultPath:       a.lastWindowsDir(false),
	}
	finish := func(paths []string, err error) {
		adb.UploadLog("save dialog err=%v paths=%d", err, len(paths))
		if err != nil {
			a.status = err.Error()
			return
		}
		if len(paths) == 0 {
			a.status = a.tr("Skipped")
			return
		}
		a.rememberWindowsDir(false, paths[0])
		a.startPull(paths[0], serial, remote, chosen, label, total)
	}
	win := a.win
	if win == nil {
		paths, err := openDownloadDialog(opts)
		finish(paths, err)
		return
	}
	go func() {
		paths, err := openDownloadDialog(opts)
		win.Update(func() { finish(paths, err) })
	}()
}

// pullRemote copies one device file. Tests replace it.
var pullRemote = func(a *app, ctx context.Context, serial, remote string, w io.Writer, onBytes func(int64)) (int64, error) {
	return a.client().PullProgress(ctx, serial, remote, w, onBytes)
}

const (
	dlSkip = iota
	dlOverwrite
	dlRename
)

// downloadAsk is one existing local file waiting for skip, overwrite, or rename.
type downloadAsk struct {
	name  string
	next  string
	rest  bool
	reply chan int
}

// askDownload blocks until the user chooses, or ctx is cancelled.
// The local file is not opened yet, so Cancel and Skip leave it untouched.
// rest tells the prompt that the same choice covers later files in this batch.
func (a *app) askDownload(ctx context.Context, name, next string, rest bool) (int, error) {
	reply := make(chan int, 1)
	a.winUpdate(func() {
		a.downloadAsk = &downloadAsk{name: name, next: next, rest: rest, reply: reply}
	})
	select {
	case n := <-reply:
		return n, nil
	case <-ctx.Done():
		a.winUpdate(func() {
			if a.downloadAsk != nil && a.downloadAsk.reply == reply {
				a.downloadAsk = nil
			}
		})
		return 0, ctx.Err()
	}
}

func (a *app) chooseDownload(choice int) {
	ask := a.downloadAsk
	if ask == nil {
		return
	}
	a.downloadAsk = nil
	select {
	case ask.reply <- choice:
	default:
	}
}

// uniqueDownloadName is path with " (1)" before the extension, then " (2)",
// until that path is free.
func uniqueDownloadName(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, time.Now().Unix(), ext))
}

func laterDownloadExists(items []pullFile) bool {
	for _, it := range items {
		st, err := os.Stat(it.local)
		if err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

func (a *app) startPull(dir, serial, remote string, chosen []adb.Entry, label string, total int64) {
	a.status = a.trf("Downloading %s", label)
	a.enqueue("download", label, total, func(item *op, ctx context.Context, report func(int64)) error {
		// The selection's sizes miss files inside folders, so the bar used
		// to finish early or stay at 0 while those files were still copying.
		// Count every file after the walk, and add bytes actually copied,
		// not the size the directory listing claimed.
		var items []pullFile
		for _, e := range chosen {
			local := filepath.Join(dir, e.Name)
			from := joinPath(remote, e.Name)
			if e.IsDir() {
				if err := collectDir(ctx, a.client(), serial, from, local, &items); err != nil {
					return err
				}
				continue
			}
			items = append(items, pullFile{remote: from, local: local, size: int64(e.Size)})
		}
		names := make([]string, len(items))
		var sum int64
		for i, it := range items {
			names[i] = pullName(dir, it.local)
			sum += it.size
		}
		batch := sum
		if batch == 0 {
			batch = total
		}
		a.setOpBatch(item, names, batch)
		var got int64
		asked := false
		choice := dlSkip
		for i, it := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(it.local), 0o755); err != nil {
				return err
			}
			name := names[i]
			a.setOpFile(item, name, i+1)
			// Ask before creating the file. Create would truncate an existing
			// one, and a failed copy must not delete what was already there.
			if st, err := os.Stat(it.local); err == nil && !st.IsDir() {
				next := uniqueDownloadName(it.local)
				if !asked {
					var err error
					choice, err = a.askDownload(ctx, name, filepath.Base(next), laterDownloadExists(items[i+1:]))
					if err != nil {
						return err
					}
					asked = true
				}
				switch choice {
				case dlSkip:
					a.winUpdate(func() {
						if it.size > 0 && item.total >= it.size {
							item.total -= it.size
						}
						a.status = a.trf("Skipped %s", name)
					})
					continue
				case dlRename:
					it.local = next
					name = pullName(dir, it.local)
					names[i] = name
					n := name
					idx := i
					a.winUpdate(func() {
						if idx < len(item.names) {
							item.names[idx] = n
						}
						item.file = n
					})
				}
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
			base := got
			n, err := saveDownload(it.local, func(w io.Writer) (int64, error) {
				return pullRemote(a, ctx, serial, it.remote, w, func(copied int64) {
					report(base + copied)
				})
			})
			if err != nil {
				return err
			}
			if n > it.size {
				extra := n - it.size
				a.winUpdate(func() { item.total += extra })
			}
			got += n
			report(got)
		}
		a.winUpdate(func() {
			if got > 0 {
				item.done = got
				item.total = got
			}
			a.status = a.trf("Saved to %s", dir)
		})
		return nil
	})
}

// saveDownload copies into local.part and replaces local only after the
// copy succeeds, so a failure leaves an existing file as it was.
func saveDownload(local string, write func(w io.Writer) (int64, error)) (int64, error) {
	part := local + ".part"
	f, err := os.Create(part)
	if err != nil {
		return 0, err
	}
	n, werr := write(f)
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(part)
		return n, werr
	}
	if err := os.Rename(part, local); err != nil {
		// Windows will not replace an existing file by renaming over it.
		if rerr := os.Remove(local); rerr != nil {
			os.Remove(part)
			return n, err
		}
		if err := os.Rename(part, local); err != nil {
			os.Remove(part)
			return n, err
		}
	}
	return n, nil
}

func pullName(root, local string) string {
	rel, err := filepath.Rel(root, local)
	if err != nil || rel == "." || rel == "" {
		return filepath.Base(local)
	}
	return filepath.ToSlash(rel)
}

// pullFile is one remote file saved under the downloads folder.
type pullFile struct {
	remote string
	local  string
	size   int64
}

// collectDir lists remote and every directory under it, creating the same
// folders locally. A repeated path is skipped so a link cycle cannot loop.
func collectDir(ctx context.Context, client *adb.Client, serial, remote, local string, out *[]pullFile) error {
	seen := map[string]struct{}{}
	var walk func(remote, local string) error
	walk = func(remote, local string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, ok := seen[remote]; ok {
			return nil
		}
		seen[remote] = struct{}{}
		if err := os.MkdirAll(local, 0o755); err != nil {
			return err
		}
		entries, err := client.List(ctx, serial, remote)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !safeChild(e.Name) || e.IsLink() {
				continue
			}
			nextRemote := joinPath(remote, e.Name)
			nextLocal := filepath.Join(local, e.Name)
			if e.IsDir() {
				if err := walk(nextRemote, nextLocal); err != nil {
					return err
				}
				continue
			}
			*out = append(*out, pullFile{remote: nextRemote, local: nextLocal, size: int64(e.Size)})
		}
		return nil
	}
	return walk(remote, local)
}

func safeChild(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

func (a *app) upload(local string) {
	if a.serial == "" || local == "" {
		return
	}
	info, err := os.Stat(local)
	if err != nil {
		a.status = err.Error()
		return
	}
	if info.IsDir() {
		a.uploadDir(local)
		return
	}
	serial, remote := a.serial, a.focus().path
	name := filepath.Base(local)
	a.status = a.trf("Uploading %s", name)
	adb.UploadLog("upload file local=%s remote=%s size=%d", local, joinPath(remote, name), info.Size())
	a.enqueue("upload", name, info.Size(), func(_ *op, ctx context.Context, report func(int64)) error {
		err := a.client().PushProgress(ctx, serial, joinPath(remote, name), local, report)
		if err != nil {
			return err
		}
		report(info.Size())
		a.winUpdate(func() { a.load(remote) })
		return nil
	})
}

// uploadDir sends every file under local, keeping the relative folders.
// Remote directories are created with mkdir before the files are pushed.
func (a *app) uploadDir(local string) {
	name := filepath.Base(local)
	files, total, err := localTree(local)
	if err != nil {
		a.status = err.Error()
		return
	}
	serial, remote := a.serial, joinPath(a.focus().path, name)
	parent := a.focus().path
	a.status = a.trf("Uploading %s", name)
	adb.UploadLog("upload dir local=%s remote=%s files=%d total=%d", local, remote, len(files), total)
	a.enqueue("upload", name, total, func(_ *op, ctx context.Context, report func(int64)) error {
		dirs := map[string]struct{}{remote: {}}
		for _, f := range files {
			for _, part := range strings.Split(f.rel, "/") {
				if !safeChild(part) {
					return fmt.Errorf("upload %s: bad name", f.rel)
				}
			}
			dir := remote
			parts := strings.Split(f.rel, "/")
			for _, part := range parts[:len(parts)-1] {
				dir = joinPath(dir, part)
				dirs[dir] = struct{}{}
			}
		}
		var quoted []string
		for dir := range dirs {
			quoted = append(quoted, shellQuote(dir))
		}
		if len(quoted) > 0 {
			if _, err := a.client().Shell(ctx, serial, "mkdir -p "+strings.Join(quoted, " ")); err != nil {
				return err
			}
		}
		var got int64
		for _, f := range files {
			if err := ctx.Err(); err != nil {
				return err
			}
			base := got
			if err := a.client().PushProgress(ctx, serial, joinPath(remote, f.rel), f.path, func(n int64) {
				report(base + n)
			}); err != nil {
				return err
			}
			got += f.size
			report(got)
		}
		a.winUpdate(func() { a.load(parent) })
		return nil
	})
}

type localFile struct {
	path string
	rel  string
	size int64
}

func localTree(root string) ([]localFile, int64, error) {
	var files []localFile
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		for _, part := range strings.Split(rel, "/") {
			if !safeChild(part) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, localFile{path: path, rel: rel, size: info.Size()})
		total += info.Size()
		return nil
	})
	return files, total, err
}

func (a *app) shellOp(kind, label, command, reload string) {
	if a.serial == "" || strings.TrimSpace(command) == "" {
		return
	}
	serial := a.serial
	a.enqueue(kind, label, 0, func(_ *op, ctx context.Context, _ func(int64)) error {
		out, err := a.client().Shell(ctx, serial, command)
		if err != nil {
			return err
		}
		msg := strings.TrimSpace(out)
		low := strings.ToLower(msg)
		if msg != "" && (strings.Contains(low, "no such") || strings.Contains(msg, "Permission denied") || strings.Contains(msg, "Read-only") || strings.Contains(low, "failed")) {
			return errors.New(msg)
		}
		if reload != "" {
			a.winUpdate(func() { a.load(reload) })
		}
		return nil
	})
}

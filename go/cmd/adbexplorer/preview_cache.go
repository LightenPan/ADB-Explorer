package main

import (
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

type previewHit struct {
	bitmaps []*ui.Bitmap
	svg     *ui.SVG
}

type previewEntry struct {
	key     string
	size    uint32
	mtime   time.Time
	hit     previewHit
	touched int
}

var (
	iconMu    sync.Mutex
	iconOrder []string

	previewMu    sync.Mutex
	previewLRU   []previewEntry
	previewClock int
)

const (
	iconCacheCap    = 64
	previewCacheCap = 8
)

func setFileIcon(remote string, bmp *ui.Bitmap) {
	iconMu.Lock()
	defer iconMu.Unlock()
	if _, ok := fileIcons[remote]; !ok {
		iconOrder = append(iconOrder, remote)
	}
	fileIcons[remote] = bmp
	for len(iconOrder) > iconCacheCap {
		old := iconOrder[0]
		iconOrder = iconOrder[1:]
		delete(fileIcons, old)
	}
}

func clearIconCache() {
	iconMu.Lock()
	defer iconMu.Unlock()
	fileIcons = map[string]*ui.Bitmap{}
	iconOrder = nil
}

func previewCacheKey(remote string, size uint32, mtime time.Time) string {
	return remote + "\x00" + mtime.UTC().Format(time.RFC3339Nano) + "\x00" + itoaSize(size)
}

func itoaSize(n uint32) string {
	if n == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func previewCacheGet(remote string, size uint32, mtime time.Time) (previewHit, bool) {
	previewMu.Lock()
	defer previewMu.Unlock()
	key := previewCacheKey(remote, size, mtime)
	for i := range previewLRU {
		if previewLRU[i].key == key {
			previewClock++
			previewLRU[i].touched = previewClock
			return previewLRU[i].hit, true
		}
	}
	return previewHit{}, false
}

func previewCachePut(remote string, size uint32, mtime time.Time, hit previewHit) {
	previewMu.Lock()
	defer previewMu.Unlock()
	key := previewCacheKey(remote, size, mtime)
	previewClock++
	for i := range previewLRU {
		if previewLRU[i].key == key {
			previewLRU[i].hit = hit
			previewLRU[i].touched = previewClock
			return
		}
	}
	previewLRU = append(previewLRU, previewEntry{key: key, size: size, mtime: mtime, hit: hit, touched: previewClock})
	if len(previewLRU) <= previewCacheCap {
		return
	}
	oldest := 0
	for i := 1; i < len(previewLRU); i++ {
		if previewLRU[i].touched < previewLRU[oldest].touched {
			oldest = i
		}
	}
	previewLRU = append(previewLRU[:oldest], previewLRU[oldest+1:]...)
}

func clearPreviewCache() {
	previewMu.Lock()
	defer previewMu.Unlock()
	previewLRU = nil
}

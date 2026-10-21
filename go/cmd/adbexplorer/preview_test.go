package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// Isolated preview jobs can fail in ways a pane PNG cannot show:
// - a slow pull for file A must not paint over file B (stale gen)
// - a Pull past the cap must stop; errPreviewLimit is success, not an error
// - NUL / high-control-byte payloads must not count as text
// - unzip -l headers (Name / ----) must not become archive members

func TestLimitedWriterStopsAtCap(t *testing.T) {
	var buf bytes.Buffer
	w := &limitedWriter{W: &buf, N: 3}
	_, err := io.Copy(w, strings.NewReader("abcdef"))
	if !errors.Is(err, errPreviewLimit) {
		t.Fatalf("err %v, want errPreviewLimit", err)
	}
	if buf.String() != "abc" {
		t.Fatalf("wrote %q", buf.String())
	}
}

func TestPrintableRejectsNUL(t *testing.T) {
	if printable("ok\x00no") {
		t.Fatal("NUL counted as text")
	}
	if printableBytes([]byte{0}) {
		t.Fatal("single NUL counted as text")
	}
	if !printable("hello\n\tworld") {
		t.Fatal("plain text rejected")
	}
	bad := strings.Repeat(string(rune(1)), 20)
	if printable(bad) {
		t.Fatal("control bytes counted as text")
	}
}

func TestStalePreviewDoesNotPaint(t *testing.T) {
	a := newApp()
	a.detail.gen = 2
	a.publishDetail(1, func(d *detail) { d.preview = "stale" })
	if a.detail.preview == "stale" {
		t.Fatal("stale gen painted over the current file")
	}
	a.publishDetail(2, func(d *detail) { d.preview = "current" })
	if a.detail.preview != "current" {
		t.Fatalf("current gen not applied: %q", a.detail.preview)
	}
}

func TestArchiveEntriesSkipZipHeaders(t *testing.T) {
	out := "" +
		"Archive:  notes.zip\n" +
		"  Length      Date    Time    Name\n" +
		"---------  ---------- -----   ----\n" +
		"      12  2024-01-01 12:00   hello.txt\n" +
		"       0  2024-01-01 12:00   assets/\n" +
		"---------                     -------\n" +
		"      12                     2 files\n"
	rows := archiveEntries(out, true)
	if len(rows) != 2 {
		t.Fatalf("rows %+v", rows)
	}
	if rows[0].Name != "hello.txt" || rows[0].IsDir() {
		t.Fatalf("file %+v", rows[0])
	}
	if rows[1].Name != "assets" || !rows[1].IsDir() {
		t.Fatalf("dir %+v", rows[1])
	}
	for _, e := range rows {
		if e.Name == "Name" || e.Name == "----" {
			t.Fatalf("header leaked: %+v", e)
		}
	}
}

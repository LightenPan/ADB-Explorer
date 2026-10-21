package main

import (
	"context"
	"testing"
	"time"
)

func TestPaths(t *testing.T) {
	cases := map[string]string{
		"/sdcard":           "/sdcard",
		"/sdcard/":          "/sdcard",
		"sdcard":            "/sdcard",
		"/sdcard/../sdcard": "/sdcard",
		"/a/b/../c":         "/a/c",
		"/":                 "/",
	}
	for in, want := range cases {
		if got := cleanPath(in); got != want {
			t.Errorf("cleanPath(%q) = %q, want %q", in, got, want)
		}
	}
	if parentPath("/sdcard/DCIM") != "/sdcard" || parentPath("/sdcard") != "/" || parentPath("/") != "" {
		t.Fatalf("parents: %q %q %q", parentPath("/sdcard/DCIM"), parentPath("/sdcard"), parentPath("/"))
	}
	if joinPath("/", "a") != "/a" || joinPath("/sdcard", "a") != "/sdcard/a" {
		t.Fatal("join")
	}
	if shellQuote("a'b") != `'a'\''b'` {
		t.Fatalf("quote %s", shellQuote("a'b"))
	}
}

func TestJobsReplaceEachOther(t *testing.T) {
	a := newApp()
	ctx1, gen1 := a.begin(&a.browse)
	_, gen2 := a.begin(&a.browse)
	if gen1 == gen2 {
		t.Fatal("generation did not advance")
	}
	if ctx1.Err() == nil {
		t.Fatal("the replaced job was not cancelled")
	}
	if !a.current(&a.browse, gen2) || a.current(&a.browse, gen1) {
		t.Fatal("current generation")
	}
	if ctx1.Err() == nil {
		t.Fatal("the first browse context was un-cancelled")
	}
}

func TestOpsQueueOneAtATime(t *testing.T) {
	a := newApp()
	started := make(chan int, 2)
	release := make(chan struct{})
	a.enqueue("upload", "a", 1, func(_ *op, ctx context.Context, report func(int64)) error {
		started <- 1
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		report(1)
		return nil
	})
	a.enqueue("upload", "b", 1, func(_ *op, ctx context.Context, report func(int64)) error {
		started <- 2
		report(1)
		return nil
	})
	if got := <-started; got != 1 {
		t.Fatalf("first op %d", got)
	}
	select {
	case n := <-started:
		t.Fatalf("second op started early: %d", n)
	case <-time.After(80 * time.Millisecond):
	}
	close(release)
	if got := <-started; got != 2 {
		t.Fatalf("second op %d", got)
	}
	deadline := time.After(time.Second)
	for {
		if a.ops[0].state == opDone && a.ops[1].state == opDone {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("states %s %s", a.ops[0].state, a.ops[1].state)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

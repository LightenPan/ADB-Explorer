package main

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestIconForPicksDensity(t *testing.T) {
	apk := zipOf(t, map[string][]byte{
		"res/drawable-mdpi/ic_launcher.png":   pngBytes(t, 8),
		"res/drawable-xxhdpi/ic_launcher.png": pngBytes(t, 16),
		"res/drawable-hdpi/ic_other.png":      pngBytes(t, 32),
		"AndroidManifest.xml":                 []byte("binary"),
	})
	got, ok := iconFor(apk)
	if !ok {
		t.Fatal("no icon")
	}
	if len(got) != len(pngBytes(t, 16)) {
		t.Fatalf("picked %d bytes, want the xxhdpi icon", len(got))
	}
}

func TestIconForSkipsNonIcons(t *testing.T) {
	apk := zipOf(t, map[string][]byte{
		"res/drawable/background.png": pngBytes(t, 8),
		"classes.dex":                 []byte("dex"),
	})
	if _, ok := iconFor(apk); ok {
		t.Fatal("a background is not an icon")
	}
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pngBytes(t *testing.T, n int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 32, G: 120, B: 220, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

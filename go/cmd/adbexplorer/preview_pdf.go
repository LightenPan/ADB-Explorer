package main

import (
	"context"
	"errors"
	"image"
	"image/color"

	"github.com/egoist/mygo/ui"
	"github.com/timzifer/cera"
)

const pdfPageCap = 20

func (a *app) renderPDF(ctx context.Context, gen int, data []byte, password string) {
	if !a.previewPDFEnabled() {
		a.publishDetail(gen, func(d *detail) {
			d.bytes = data
			d.err = a.tr("PDF preview unavailable")
		})
		return
	}
	doc, err := openPDF(data, password)
	if err != nil {
		need := errors.Is(err, cera.ErrPassword)
		a.publishDetail(gen, func(d *detail) {
			d.bytes = data
			d.pdfNeed = need
			d.pdfWrong = need && password != ""
			if need {
				if password != "" {
					d.err = a.tr("Wrong password")
				} else {
					d.err = ""
					d.preview = a.tr("This PDF is encrypted")
				}
				return
			}
			d.err = err.Error()
		})
		return
	}
	n := doc.NumPages()
	if n <= 0 {
		a.publishDetail(gen, func(d *detail) {
			d.bytes = data
			d.err = a.tr("PDF has no pages")
		})
		return
	}
	limit := n
	if limit > pdfPageCap {
		limit = pdfPageCap
	}
	a.publishDetail(gen, func(d *detail) {
		d.bytes = data
		d.pdfNeed = false
		d.pdfWrong = false
		d.pages = n
		d.bitmaps = nil
		d.loading = true
		d.err = ""
		if n > pdfPageCap {
			d.caption = a.trf("Showing %d of %d pages", pdfPageCap, n)
		} else {
			d.caption = a.trf("%d pages", n)
		}
	})
	var pages []*ui.Bitmap
	for i := 0; i < limit; i++ {
		if ctx.Err() != nil {
			return
		}
		bmp, err := renderPDFPage(ctx, doc, i)
		if err != nil {
			a.publishDetail(gen, func(d *detail) {
				if len(pages) == 0 {
					d.err = err.Error()
				} else {
					d.bitmaps = append([]*ui.Bitmap(nil), pages...)
					d.err = err.Error()
				}
			})
			return
		}
		pages = append(pages, bmp)
		shot := append([]*ui.Bitmap(nil), pages...)
		a.winUpdate(func() {
			if a.detail.gen != gen {
				return
			}
			a.detail.bitmaps = shot
			a.detail.loading = i+1 < limit
			a.detail.pdfNeed = false
			a.detail.pdfWrong = false
		})
	}
}

func openPDF(data []byte, password string) (*cera.Document, error) {
	if password == "" {
		return cera.Open(data)
	}
	return cera.OpenWithPassword(data, password)
}

func renderPDFPage(ctx context.Context, doc *cera.Document, i int) (*ui.Bitmap, error) {
	page, err := doc.Page(i)
	if err != nil {
		return nil, err
	}
	defer page.Release()
	const paneW = 336.0
	w, h := page.Size()
	if w <= 0 {
		w = 1
	}
	scale := paneW / w
	if scale <= 0 {
		scale = 1
	}
	if h*scale > 480 {
		scale = 480 / h
	}
	bounds := page.Bounds(scale)
	dst := image.NewRGBA(bounds)
	err = page.Render(ctx, dst, cera.RenderOptions{
		Scale:      scale,
		Background: color.RGBA{255, 255, 255, 255},
		Workers:    1,
	})
	if err != nil && dst.Bounds().Empty() {
		return nil, err
	}
	return ui.NewBitmap(dst), nil
}

package files

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"
)

func TestFitWithin(t *testing.T) {
	cases := []struct {
		w, h, max    int
		wantW, wantH int
	}{
		{1024, 768, 512, 512, 384}, // landscape
		{768, 1024, 512, 384, 512}, // portrait
		{100, 100, 512, 100, 100},  // inside the box: never upscale
		{4000, 10, 512, 512, 1},    // extreme aspect keeps at least one pixel
		{10, 4000, 512, 1, 512},
	}
	for _, c := range cases {
		gotW, gotH := fitWithin(c.w, c.h, c.max)
		if gotW != c.wantW || gotH != c.wantH {
			t.Errorf("fitWithin(%d,%d,%d) = (%d,%d), want (%d,%d)",
				c.w, c.h, c.max, gotW, gotH, c.wantW, c.wantH)
		}
	}
}

func TestEncodeThumbnail(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for x := 0; x < 800; x++ {
		for y := 0; y < 600; y++ {
			src.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}

	data, w, h, err := encodeThumbnail(src, 512)
	if err != nil {
		t.Fatalf("encodeThumbnail: %v", err)
	}
	if w != 512 || h != 384 {
		t.Errorf("reported size = (%d,%d), want (512,384)", w, h)
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode produced image: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("format = %s, want jpeg", format)
	}
	if b := img.Bounds(); b.Dx() != 512 || b.Dy() != 384 {
		t.Errorf("decoded bounds = %dx%d, want 512x384", b.Dx(), b.Dy())
	}

	// A source already inside the box must be skipped rather than enlarged.
	small := image.NewRGBA(image.Rect(0, 0, 64, 64))
	if _, _, _, err := encodeThumbnail(small, 512); !errors.Is(err, errNoResize) {
		t.Errorf("err = %v, want errNoResize", err)
	}
}

func TestThumbnailSourceKind(t *testing.T) {
	cases := []struct {
		mime, fileType string
		want           thumbSource
	}{
		{"image/jpeg", "image", thumbImage},
		{"image/png", "image", thumbImage},
		{"image/svg+xml", "image", thumbUnsupported}, // not decodable by the std lib
		{"image/avif", "image", thumbUnsupported},
		{"video/mp4", "video", thumbVideo},
		{"application/pdf", "document", thumbUnsupported},
		{"audio/mpeg", "audio", thumbUnsupported},
		{"application/zip", "archive", thumbUnsupported},
		{"", "other", thumbUnsupported},
	}
	for _, c := range cases {
		if got := thumbnailSourceKind(c.mime, c.fileType); got != c.want {
			t.Errorf("thumbnailSourceKind(%q,%q) = %v, want %v", c.mime, c.fileType, got, c.want)
		}
	}
}

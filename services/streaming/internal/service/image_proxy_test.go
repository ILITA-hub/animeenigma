package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestSnapWidth(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 0},
		{-5, 0},
		{1, 128},
		{128, 128},
		{129, 256},
		{300, 384},
		{384, 384},
		{640, 640},
		{9999, 640}, // above the largest bucket → clamp to largest
	}
	for _, c := range cases {
		if got := snapWidth(c.in); got != c.want {
			t.Errorf("snapWidth(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func encodeTestImage(t *testing.T, w, h int, asPNG bool) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	var err error
	if asPNG {
		err = png.Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	}
	if err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	return buf.Bytes()
}

func TestDownscaleImage(t *testing.T) {
	// 700x1050 mimics a Shikimori 2:3 original
	src := encodeTestImage(t, 700, 1050, false)

	out, ct, err := downscaleImage(src, 128)
	if err != nil {
		t.Fatalf("downscaleImage: %v", err)
	}
	if ct != "image/jpeg" {
		t.Errorf("content type = %q, want image/jpeg", ct)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got := img.Bounds().Dx(); got != 128 {
		t.Errorf("output width = %d, want 128", got)
	}
	if got := img.Bounds().Dy(); got != 192 {
		t.Errorf("output height = %d, want 192 (aspect preserved)", got)
	}
	if len(out) >= len(src) {
		t.Errorf("resized output (%dB) not smaller than source (%dB)", len(out), len(src))
	}
}

func TestDownscaleImageNeverUpscales(t *testing.T) {
	src := encodeTestImage(t, 100, 150, false)

	out, _, err := downscaleImage(src, 640)
	if err != nil {
		t.Fatalf("downscaleImage: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got := img.Bounds().Dx(); got != 100 {
		t.Errorf("output width = %d, want 100 (no upscale)", got)
	}
}

func TestDownscaleImagePNGInput(t *testing.T) {
	src := encodeTestImage(t, 400, 600, true)

	out, ct, err := downscaleImage(src, 256)
	if err != nil {
		t.Fatalf("downscaleImage(png): %v", err)
	}
	if ct != "image/jpeg" {
		t.Errorf("content type = %q, want image/jpeg", ct)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got := img.Bounds().Dx(); got != 256 {
		t.Errorf("output width = %d, want 256", got)
	}
}

func TestDownscaleImageRejectsGarbage(t *testing.T) {
	if _, _, err := downscaleImage([]byte("not an image"), 128); err == nil {
		t.Error("expected error for non-image input")
	}
}

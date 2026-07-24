package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// fakeDegradation is a minimal degradationLevelReader for tests: it reports a
// fixed level without any Redis/governor dependency.
type fakeDegradation struct{ level int }

func (f fakeDegradation) Level() int { return f.level }

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

// TestResizeOrShedUnderCriticalDegradation proves the request-path lever: at
// degradation level >= 2 the CPU-heavy resize is skipped and the un-resized
// original ImageResult is served verbatim (and not cached), while level < 2
// downscales normally.
func TestResizeOrShedUnderCriticalDegradation(t *testing.T) {
	// 700x1050 mimics a Shikimori 2:3 original.
	src := encodeTestImage(t, 700, 1050, false)
	newFull := func() *ImageResult {
		return &ImageResult{Data: src, ContentType: "image/jpeg", Source: SourceShikimori}
	}

	t.Run("level>=2 sheds resize and serves original", func(t *testing.T) {
		for _, level := range []int{2, 3} {
			s := &ImageProxyService{degradation: fakeDegradation{level: level}, log: logger.Default()}
			full := newFull()
			got, cacheable := s.resizeOrShed(full, 128)
			if got != full {
				t.Fatalf("level %d: expected the original ImageResult served unchanged", level)
			}
			if !bytes.Equal(got.Data, src) {
				t.Errorf("level %d: original bytes not served under Critical degradation", level)
			}
			if got.ContentType != "image/jpeg" {
				t.Errorf("level %d: content type = %q, want image/jpeg (unchanged)", level, got.ContentType)
			}
			if cacheable {
				t.Errorf("level %d: shed path must not be cached", level)
			}
		}
	})

	t.Run("level<2 resizes normally", func(t *testing.T) {
		for _, level := range []int{0, 1} {
			s := &ImageProxyService{degradation: fakeDegradation{level: level}, log: logger.Default()}
			full := newFull()
			got, cacheable := s.resizeOrShed(full, 128)
			if got == full {
				t.Fatalf("level %d: expected a resized result, got the original unchanged", level)
			}
			if !cacheable {
				t.Errorf("level %d: a genuine resize must be cacheable", level)
			}
			if got.ContentType != "image/jpeg" {
				t.Errorf("level %d: content type = %q, want image/jpeg", level, got.ContentType)
			}
			img, _, err := image.Decode(bytes.NewReader(got.Data))
			if err != nil {
				t.Fatalf("level %d: decode resized output: %v", level, err)
			}
			if w := img.Bounds().Dx(); w != 128 {
				t.Errorf("level %d: resized width = %d, want 128", level, w)
			}
		}
	})

	t.Run("nil reader fails open and resizes", func(t *testing.T) {
		s := &ImageProxyService{degradation: nil, log: logger.Default()}
		got, cacheable := s.resizeOrShed(newFull(), 128)
		if got.Source == SourcePlaceholder || !cacheable {
			t.Fatalf("nil degradation reader must fail open and resize normally")
		}
		if img, _, err := image.Decode(bytes.NewReader(got.Data)); err != nil || img.Bounds().Dx() != 128 {
			t.Fatalf("nil reader: expected 128px resize, err=%v", err)
		}
	})
}

// TestNewImageProxyServiceNilWatcherFailOpen proves the wiring is nil-safe: a
// typed-nil *cache.DegradationWatcher (governor undeployed) reads level 0 via
// its nil-receiver-safe Level(), so the proxy resizes normally.
func TestNewImageProxyServiceNilWatcherFailOpen(t *testing.T) {
	svc := NewImageProxyService(nil, (*cache.DegradationWatcher)(nil), logger.Default(), "http://gacha:8093")
	src := encodeTestImage(t, 700, 1050, false)
	full := &ImageResult{Data: src, ContentType: "image/jpeg", Source: SourceShikimori}

	got, cacheable := svc.resizeOrShed(full, 128)
	if got == full || !cacheable {
		t.Fatalf("typed-nil *DegradationWatcher must fail open and resize normally")
	}
}

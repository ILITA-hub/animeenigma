package service

import (
	"bytes"
	"context"
	"image"
	"image/color"
	draw "image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
)

// fakeStore satisfies the objectStore interface for testing.
type fakeStore struct {
	uploaded    []fakeUpload
	uploadError error
}

type fakeUpload struct {
	key         string
	size        int64
	contentType string
	data        []byte
}

func (f *fakeStore) Upload(_ context.Context, key string, r io.Reader, size int64, contentType string) error {
	if f.uploadError != nil {
		return f.uploadError
	}
	data, _ := io.ReadAll(r)
	f.uploaded = append(f.uploaded, fakeUpload{key: key, size: size, contentType: contentType, data: data})
	return nil
}

func TestIngestFromURL_DownloadsAndStores(t *testing.T) {
	// httptest server returns a small PNG payload
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a} // minimal PNG header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()

	store := &fakeStore{}
	svc := NewImageService(store)
	svc.allowPrivate = true // reach the loopback httptest server

	key, err := svc.IngestFromURL(context.Background(), srv.URL, "cards")
	if err != nil {
		t.Fatalf("IngestFromURL: %v", err)
	}

	// Key must match cards/<uuid>.png
	matched, _ := regexp.MatchString(`^cards/[0-9a-f-]{36}\.png$`, key)
	if !matched {
		t.Errorf("key %q does not match expected pattern ^cards/[uuid].png$", key)
	}

	if len(store.uploaded) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(store.uploaded))
	}
	up := store.uploaded[0]
	if up.contentType != "image/png" {
		t.Errorf("content-type = %q, want image/png", up.contentType)
	}
	if up.size != int64(len(pngBytes)) {
		t.Errorf("size = %d, want %d", up.size, len(pngBytes))
	}
	if !bytes.Equal(up.data, pngBytes) {
		t.Error("uploaded bytes differ from source")
	}
}

func TestIngestFromURL_RejectsBadTypeAndTooLarge(t *testing.T) {
	// text/html should be rejected
	htmlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html>"))
	}))
	defer htmlSrv.Close()

	store := &fakeStore{}
	svc := NewImageService(store)
	svc.allowPrivate = true // reach the loopback httptest servers

	_, err := svc.IngestFromURL(context.Background(), htmlSrv.URL, "cards")
	if err == nil {
		t.Fatal("expected InvalidInput for text/html content type")
	}
	appErr, ok := err.(*apperrors.AppError)
	if !ok || appErr.Code != apperrors.CodeInvalidInput {
		t.Errorf("expected InvalidInput, got %v", err)
	}

	// Oversized response: Content-Length > 10 MiB should be rejected before download
	bigSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", "11534336") // 11 MiB
		w.WriteHeader(200)
		// We don't actually write 11MiB — the client should reject early
		_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", 100)))
	}))
	defer bigSrv.Close()

	_, err = svc.IngestFromURL(context.Background(), bigSrv.URL, "banners")
	if err == nil {
		t.Fatal("expected InvalidInput for oversized Content-Length")
	}
	appErr2, ok2 := err.(*apperrors.AppError)
	if !ok2 || appErr2.Code != apperrors.CodeInvalidInput {
		t.Errorf("expected InvalidInput for oversized, got %v", err)
	}
}

// TestIngestFromURL_RejectsLoopbackServer points at a LIVE loopback test server.
// Without the SSRF guard (finding #20) this fetch succeeds; the guard must
// reject the 127.0.0.1 IP-literal host before fetching.
func TestIngestFromURL_RejectsLoopbackServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	defer srv.Close()

	svc := NewImageService(&fakeStore{}) // guard ON (production default)
	if _, err := svc.IngestFromURL(context.Background(), srv.URL, "cards"); err == nil {
		t.Fatal("expected SSRF guard to reject loopback server URL, got success")
	}
}

// TestIngestFromURL_RejectsLoopbackViaHostname reaches the same live loopback
// server through the "localhost" hostname: the cheap pre-flight passes (it is
// not an IP literal) but the dial-time Control hook — the rebind-safe layer —
// must reject the connection to 127.0.0.1.
func TestIngestFromURL_RejectsLoopbackViaHostname(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	defer srv.Close()

	u := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
	svc := NewImageService(&fakeStore{}) // guard ON
	if _, err := svc.IngestFromURL(context.Background(), u, "cards"); err == nil {
		t.Fatal("expected dial-time guard to reject localhost (loopback), got success")
	}
}

func TestIngestUpload_RejectsUnknownExtension(t *testing.T) {
	store := &fakeStore{}
	svc := NewImageService(store)

	_, err := svc.IngestUpload(
		context.Background(),
		strings.NewReader("data"),
		"x.exe",
		"application/octet-stream",
		"cards",
	)
	if err == nil {
		t.Fatal("expected InvalidInput for .exe extension")
	}
	appErr, ok := err.(*apperrors.AppError)
	if !ok || appErr.Code != apperrors.CodeInvalidInput {
		t.Errorf("expected InvalidInput, got %v", err)
	}
}

// helper: encode an in-memory image
func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// noisyOpaqueImage builds a deterministic, non-flat opaque image: real card
// art has photographic detail that makes JPEG q85 genuinely smaller than
// PNG. A flat/solid fill does NOT have this property (see
// TestOptimize_KeepsSmallerOriginal). Note a smooth *linear* per-pixel
// pattern doesn't work either — e.g. R,G,B derived directly from x*7, x*3,
// y*5 wrapping mod 256 is deflate-friendly (constant pixel-to-pixel delta,
// so PNG's predictor crushes it) while its periodic wraparound edges are
// adversarial for JPEG's block DCT (high-frequency ringing), making JPEG
// *bigger* than PNG — empirically verified: 3000x1500 of that pattern is
// ~99KB as PNG vs. ~1.7MB as JPEG. What actually reproduces "JPEG wins" is
// a smooth low-frequency base (gradient) with genuinely uncorrelated
// per-pixel noise layered on top, mimicking real sensor/compression noise:
// deflate can't predict the noise (kills PNG), while JPEG's lossy
// quantization discards it cheaply. The noise here is a deterministic
// integer hash of (x, y) — no math/rand — so pixels are uncorrelated but
// the image is still fully reproducible.
func noisyOpaqueImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			h32 := uint32(x)*374761393 + uint32(y)*668265263
			h32 = (h32 ^ (h32 >> 13)) * 1274126177
			noise := uint8((h32 ^ (h32 >> 16)) & 0x3F) // 0..63
			img.Set(x, y, color.RGBA{
				R: uint8(x*255/w) + noise,
				G: uint8(y*255/h) + noise,
				B: 128 + noise,
				A: 255,
			})
		}
	}
	return img
}

func TestOptimize_OpaquePNGBecomesJPEG(t *testing.T) {
	// Noisy (non-flat) fixture: see noisyOpaqueImage doc comment for why a
	// flat fill can't be used here.
	out, ct, ext := optimize(encodePNG(t, noisyOpaqueImage(100, 100)), "image/png")
	if ct != "image/jpeg" || ext != ".jpg" {
		t.Errorf("opaque png must become jpeg, got ct=%q ext=%q", ct, ext)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("output not decodable jpeg: %v", err)
	}
}

func TestOptimize_KeepsSmallerOriginal(t *testing.T) {
	// Flat solid color: PNG deflate crushes this far smaller than any JPEG
	// q85 re-encode (fixed header + DCT block overhead loses to a trivial
	// flat signal). optimize must keep the smaller original rather than
	// force a re-encode that grows the file.
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{200, 30, 30, 255}}, image.Point{}, draw.Src)
	in := encodePNG(t, img)
	out, ct, ext := optimize(in, "image/png")
	if ct != "image/png" || ext != ".png" {
		t.Errorf("flat opaque png whose jpeg re-encode is bigger must stay png, got ct=%q ext=%q", ct, ext)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("flat opaque png must pass through byte-identical when jpeg would be bigger")
	}
}

func TestOptimize_AlphaPNGStaysPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 50, 50)) // zero-value = transparent
	in := encodePNG(t, img)
	out, ct, ext := optimize(in, "image/png")
	if ct != "image/png" || ext != ".png" {
		t.Errorf("alpha png must stay png, got ct=%q ext=%q", ct, ext)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("small alpha png must pass through unchanged")
	}
}

func TestOptimize_DownscalesOversized(t *testing.T) {
	// Noisy (non-flat) fixture so the downscaled JPEG genuinely wins on size
	// (a flat 3000x1500 fill's PNG can still beat its downscaled JPEG).
	out, ct, _ := optimize(encodePNG(t, noisyOpaqueImage(3000, 1500)), "image/png")
	if ct != "image/jpeg" {
		t.Fatalf("expected jpeg, got %q", ct)
	}
	dec, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if w := dec.Bounds().Dx(); w != 2048 {
		t.Errorf("longest side must be 2048, got %d", w)
	}
}

func TestOptimize_PassthroughGifWebpGarbage(t *testing.T) {
	for _, tc := range []struct{ ct string }{{"image/gif"}, {"image/webp"}} {
		in := []byte("not-an-image")
		out, ct, ext := optimize(in, tc.ct)
		if !bytes.Equal(out, in) || ct != tc.ct || ext == ".jpg" {
			t.Errorf("%s must pass through untouched", tc.ct)
		}
	}
	in := []byte{0x89, 0x50, 0x4E, 0x47, 0xDE, 0xAD} // broken png
	out, ct, _ := optimize(in, "image/png")
	if !bytes.Equal(out, in) || ct != "image/png" {
		t.Errorf("undecodable bytes must pass through")
	}
}

// TestIngestUpload_RecompressesOversizedOpaquePNG is a real end-to-end sanity
// check: a 3000x1500 opaque PNG goes through the full IngestUpload path (not
// optimize() directly) and must come out the other side as a much smaller
// JPEG in the fake store.
func TestIngestUpload_RecompressesOversizedOpaquePNG(t *testing.T) {
	// Noisy (non-flat) fixture: see noisyOpaqueImage doc comment for why a
	// flat fill can't be used to prove "jpeg is much smaller".
	pngBytes := encodePNG(t, noisyOpaqueImage(3000, 1500))

	store := &fakeStore{}
	svc := NewImageService(store)

	key, err := svc.IngestUpload(context.Background(), bytes.NewReader(pngBytes), "card.png", "image/png", "cards")
	if err != nil {
		t.Fatalf("IngestUpload: %v", err)
	}
	if !strings.HasSuffix(key, ".jpg") {
		t.Errorf("key %q should end in .jpg after recompression", key)
	}
	if len(store.uploaded) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(store.uploaded))
	}
	up := store.uploaded[0]
	if up.contentType != "image/jpeg" {
		t.Errorf("content-type = %q, want image/jpeg", up.contentType)
	}
	dec, err := jpeg.Decode(bytes.NewReader(up.data))
	if err != nil {
		t.Fatalf("stored bytes not decodable jpeg: %v", err)
	}
	if w := dec.Bounds().Dx(); w != 2048 {
		t.Errorf("stored image longest side = %d, want 2048", w)
	}
	if len(up.data) >= len(pngBytes) {
		t.Errorf("stored jpeg (%d bytes) should be much smaller than source png (%d bytes)", len(up.data), len(pngBytes))
	}
	if up.size != int64(len(up.data)) {
		t.Errorf("reported size %d does not match stored data length %d", up.size, len(up.data))
	}
}

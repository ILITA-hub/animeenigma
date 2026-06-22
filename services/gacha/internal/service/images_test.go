package service

import (
	"bytes"
	"context"
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

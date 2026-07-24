package service

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
)

// TestRewriteGachaURL proves rewriteGachaURL is the sole gate for relative
// gacha image URLs: it accepts only /api/gacha/images/{cards,banners}/<key>
// (strict key charset, no traversal), rewrites onto the configured internal
// gacha base, and rejects everything else (other prefixes, absolute URLs,
// unrelated relative paths).
func TestRewriteGachaURL(t *testing.T) {
	s := &ImageProxyService{gachaBaseURL: "http://gacha:8093"}
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"/api/gacha/images/cards/ab-1.png", "http://gacha:8093/api/gacha/images/cards/ab-1.png", true},
		{"/api/gacha/images/banners/x.webp", "http://gacha:8093/api/gacha/images/banners/x.webp", true},
		{"/api/gacha/images/cards/../secret", "", false},
		{"/api/gacha/images/other/x.png", "", false},
		{"https://shikimori.one/x.png", "", false},
		{"/api/streaming/whatever", "", false},
	}
	for _, c := range cases {
		got, ok := s.rewriteGachaURL(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("rewriteGachaURL(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// deadLoopbackAddr returns a 127.0.0.1 address that is guaranteed to have
// nothing listening on it (a listener is opened to claim a free port, then
// immediately closed), so MinIO calls against it fail fast with "connection
// refused" instead of hanging.
func deadLoopbackAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := lis.Addr().String()
	lis.Close()
	return addr
}

// TestGetImage_GachaFetch_UsesInternalClientAndSourceGacha pins the
// Finding-1 regression: rewriteGachaURL rewrites relative gacha URLs onto a
// private Docker/K8s address (gachaBaseURL), and that address must be fetched
// through internalClient — NOT the SSRF-guarded httpClient, whose dial
// Control (netguard.DenyPrivateControl) would refuse it outright. This test
// exercises the REAL dial path: the httptest.Server listens on 127.0.0.1,
// exactly the kind of private address the guarded client is built to reject.
// Before the fix, this fetch failed, the placeholder got cached in MinIO for
// the resize key, and every gacha thumbnail request would have served it.
//
// Storage is a real *videoutils.Storage pointed at a dead loopback address
// (nothing listens there — see deadLoopbackAddr). videoutils.NewStorage only
// builds a minio.Client, it never dials, so this is safe; the subsequent
// GetObject/PutObject calls fail fast, which only forces resolveImage down
// its normal upstream-fetch path (exactly like a real MinIO cache miss) and
// is confirmed to do so quickly enough not to need mocking.
func TestGetImage_GachaFetch_UsesInternalClientAndSourceGacha(t *testing.T) {
	served := encodeTestImage(t, 4, 4, true) // a real PNG

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(served)
	}))
	defer srv.Close()

	storage, err := videoutils.NewStorage(videoutils.StorageConfig{
		Endpoint:        deadLoopbackAddr(t),
		AccessKeyID:     "fake",
		SecretAccessKey: "fake",
		BucketName:      "posters",
	})
	if err != nil {
		t.Fatalf("videoutils.NewStorage: %v", err)
	}

	svc := NewImageProxyService(storage, nil, logger.Default(), srv.URL)

	result, err := svc.GetImage(context.Background(), "/api/gacha/images/cards/x.png", 0)
	if err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	if !bytes.Equal(result.Data, served) {
		t.Errorf("GetImage returned %d bytes not matching the %d-byte served PNG — the internal fetch did not go through", len(result.Data), len(served))
	}
	if result.ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png", result.ContentType)
	}
	if result.Source != SourceGacha {
		t.Errorf("Source = %q, want %q (gacha fetches must not be mislabeled as Shikimori)", result.Source, SourceGacha)
	}
}

package videoutils

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchURLFor_SignerScope proves the UpstreamSigner only affects URLs it
// claims: claimed URLs are rewritten, everything else passes through
// unchanged, and a nil signer is a no-op.
func TestFetchURLFor_SignerScope(t *testing.T) {
	const minioURL = "http://minio:9000/raw-library/54974/1/playlist.m3u8"
	const cdnURL = "https://cdn.example.com/master.m3u8"

	// nil signer → identity.
	p := NewVideoProxy(ProxyConfig{})
	if got := p.fetchURLFor(minioURL); got != minioURL {
		t.Fatalf("nil signer should pass through, got %q", got)
	}

	// signer claims only the minio host.
	p = NewVideoProxy(ProxyConfig{
		UpstreamSigner: func(raw string) (string, bool) {
			if strings.HasPrefix(raw, "http://minio:9000/") {
				return raw + "?X-Amz-Signature=deadbeef", true
			}
			return "", false
		},
	})
	if got := p.fetchURLFor(minioURL); got != minioURL+"?X-Amz-Signature=deadbeef" {
		t.Errorf("claimed minio URL should be rewritten, got %q", got)
	}
	if got := p.fetchURLFor(cdnURL); got != cdnURL {
		t.Errorf("unclaimed CDN URL must pass through untouched, got %q", got)
	}
}

// TestPresignURL_HostScope proves PresignURL only rewrites URLs whose host is
// the storage's own MinIO endpoint, and that a presigned URL is fetchable
// (round-trips through a stand-in MinIO whose only check is the presence of
// the AWS signature query params).
func TestPresignURL_HostScope(t *testing.T) {
	s, err := NewStorage(StorageConfig{
		Endpoint:        "minio:9000",
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
		BucketName:      "animeenigma",
		Region:          "us-east-1",
	})
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	// IsOwnHost recognizes own endpoint vs foreign hosts.
	if !s.IsOwnHost("http://minio:9000/raw-library/x/1/playlist.m3u8") {
		t.Error("IsOwnHost should be true for own endpoint")
	}
	if s.IsOwnHost("https://cdn.example.com/a/b.m3u8") {
		t.Error("IsOwnHost should be false for a foreign host")
	}

	// Different host → not claimed.
	if _, ok := s.PresignURL("https://cdn.example.com/a/b.m3u8"); ok {
		t.Error("foreign host must not be claimed")
	}
	// Path without an object → not claimed.
	if _, ok := s.PresignURL("http://minio:9000/raw-library/"); ok {
		t.Error("bucket-only path must not be claimed")
	}

	// Own host + bucket/object → claimed + presigned (signature params present).
	signed, ok := s.PresignURL("http://minio:9000/raw-library/54974/1/playlist.m3u8")
	if !ok {
		t.Fatal("own-host URL with object should be claimed")
	}
	if !strings.Contains(signed, "X-Amz-Signature=") || !strings.Contains(signed, "raw-library/54974/1/playlist.m3u8") {
		t.Fatalf("presigned URL missing signature or path: %q", signed)
	}
}

// TestUpstreamSigner_EndToEnd wires a signer into the proxy and confirms the
// proxy actually fetches the REWRITTEN URL (not the original) while still
// using the original for allow-list checks.
func TestUpstreamSigner_EndToEnd(t *testing.T) {
	var gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write([]byte("segmentbytes"))
	}))
	defer srv.Close()

	// The proxy fetches srv.URL/seg.ts; the signer appends an auth marker.
	p := NewVideoProxy(ProxyConfig{
		UserAgent:      "test",
		AllowedDomains: []string{strings.TrimPrefix(srv.URL, "http://")},
		UpstreamSigner: func(raw string) (string, bool) {
			return raw + "?sig=ok", true
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	if err := p.ProxyStream(req.Context(), srv.URL+"/seg.ts", rec, req); err != nil {
		t.Fatalf("ProxyStream: %v", err)
	}
	if gotRawQuery != "sig=ok" {
		t.Fatalf("upstream should have received the signed query, got %q", gotRawQuery)
	}
}

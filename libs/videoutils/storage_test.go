package videoutils

import (
	"net/url"
	"strings"
	"testing"
)

// newFakeStorage builds a Storage against a host that is never dialed.
// NewStorage:51-66 only constructs a minio.Client — no network I/O — so this
// is safe to use offline. Region MUST be set: PresignURL calls
// PresignedGetObject, which (via minio-go's newRequest -> getBucketLocation)
// only skips its own network round-trip to discover the bucket's region when
// c.region is already non-empty.
func newFakeStorage(t *testing.T, endpoint string, useSSL bool) *Storage {
	t.Helper()
	s, err := NewStorage(StorageConfig{
		Endpoint:        endpoint,
		AccessKeyID:     "fake-access-key",
		SecretAccessKey: "fake-secret-key",
		UseSSL:          useSSL,
		BucketName:      "raw-library",
		Region:          "us-east-1",
	})
	if err != nil {
		t.Fatalf("NewStorage(%q): %v", endpoint, err)
	}
	return s
}

func TestMultiStorage_RoutesToOwningHost(t *testing.T) {
	minio := newFakeStorage(t, "minio-fake:9000", false)
	// Non-default port even for the SSL backend: minio-go's endpoint URL
	// normalizes away an explicit ":443" (the default HTTPS port), which
	// would make host comparisons below ambiguous. Real external-S3
	// endpoints (e.g. s3.firstvds.ru) are configured bare, without a port,
	// so this normalization never bites in production.
	s3 := newFakeStorage(t, "s3-fake:8443", true)

	multi := NewMultiStorage(minio, s3)

	signed, ok := multi.PresignURL("http://minio-fake:9000/raw-library/videos/ep1.ts")
	if !ok {
		t.Fatalf("expected PresignURL to claim minio-fake host")
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse signed url: %v", err)
	}
	if u.Host != "minio-fake:9000" {
		t.Errorf("host = %q, want minio-fake:9000", u.Host)
	}
	if u.Scheme != "http" {
		t.Errorf("scheme = %q, want http (UseSSL=false)", u.Scheme)
	}

	signed, ok = multi.PresignURL("https://s3-fake:8443/raw-library/videos/ep1.ts")
	if !ok {
		t.Fatalf("expected PresignURL to claim s3-fake host")
	}
	u, err = url.Parse(signed)
	if err != nil {
		t.Fatalf("parse signed url: %v", err)
	}
	if u.Host != "s3-fake:8443" {
		t.Errorf("host = %q, want s3-fake:8443", u.Host)
	}
	if u.Scheme != "https" {
		t.Errorf("scheme = %q, want https (UseSSL=true)", u.Scheme)
	}
}

func TestMultiStorage_UnknownHost_ReturnsFalse(t *testing.T) {
	minio := newFakeStorage(t, "minio-fake:9000", false)
	s3 := newFakeStorage(t, "s3-fake:443", true)
	multi := NewMultiStorage(minio, s3)

	signed, ok := multi.PresignURL("https://some-other-cdn.example/video.m3u8")
	if ok || signed != "" {
		t.Errorf("PresignURL(unknown host) = (%q, %v); want (\"\", false)", signed, ok)
	}
}

func TestMultiStorage_Hosts(t *testing.T) {
	minio := newFakeStorage(t, "minio-fake:9000", false)
	s3 := newFakeStorage(t, "s3-fake:443", true)
	multi := NewMultiStorage(minio, s3)

	hosts := multi.Hosts()
	want := []string{"minio-fake:9000", "s3-fake:443"}
	if len(hosts) != len(want) {
		t.Fatalf("Hosts() = %v, want %v", hosts, want)
	}
	for i := range want {
		if hosts[i] != want[i] {
			t.Errorf("Hosts()[%d] = %q, want %q", i, hosts[i], want[i])
		}
	}
}

// TestMultiStorage_IsOwnHost covers the metrics-labeling seam: ae playback
// served from EITHER backend must be recognized as our own, everything else
// must not.
func TestMultiStorage_IsOwnHost(t *testing.T) {
	minio := newFakeStorage(t, "minio-fake:9000", false)
	s3 := newFakeStorage(t, "s3-fake:443", true)
	multi := NewMultiStorage(minio, s3)

	if !multi.IsOwnHost("http://minio-fake:9000/raw-library/x.m3u8") {
		t.Errorf("expected minio-fake host to be recognized as own")
	}
	if !multi.IsOwnHost("https://s3-fake:443/raw-library/x.m3u8") {
		t.Errorf("expected s3-fake host to be recognized as own")
	}
	if multi.IsOwnHost("https://some-cdn.example/x.m3u8") {
		t.Errorf("expected foreign host NOT to be recognized as own")
	}
}

// TestMultiStorage_NilEntriesSkipped locks in that a caller can pass an
// optional second Storage (e.g. external S3, absent when unconfigured) as a
// nil *Storage without a manual nil check.
func TestMultiStorage_NilEntriesSkipped(t *testing.T) {
	minio := newFakeStorage(t, "minio-fake:9000", false)
	multi := NewMultiStorage(minio, nil)

	if got := multi.Hosts(); len(got) != 1 || got[0] != "minio-fake:9000" {
		t.Errorf("Hosts() = %v, want [minio-fake:9000] (nil entries skipped)", got)
	}

	if _, ok := multi.PresignURL("https://s3-fake:443/raw-library/x.ts"); ok {
		t.Errorf("expected unknown/nil-backed host to return false")
	}
}

// TestStorage_PresignURL_RefusesBucketOutsidePresignScope pins the bucket
// bound on Storage.PresignURL: the bucket is still parsed from the URL path
// segment, but it is only signed when it is one of the Storage's configured
// PresignBuckets (default: BucketName). A URL naming any other bucket on the
// same host is left unclaimed, so the credential can never sign a read
// outside its own scope even when the URL is attacker-supplied.
func TestStorage_PresignURL_RefusesBucketOutsidePresignScope(t *testing.T) {
	s := newFakeStorage(t, "minio-fake:9000", false) // configured bucket: raw-library

	signed, ok := s.PresignURL("http://minio-fake:9000/some-other-bucket/videos/ep1.ts")
	if ok || signed != "" {
		t.Errorf("PresignURL(foreign bucket) = (%q, %v); want (\"\", false)", signed, ok)
	}

	// The configured bucket is still signed.
	signed, ok = s.PresignURL("http://minio-fake:9000/raw-library/videos/ep1.ts")
	if !ok {
		t.Fatalf("expected PresignURL to sign the configured bucket")
	}
	if !strings.Contains(signed, "/raw-library/videos/ep1.ts") || !strings.Contains(signed, "X-Amz-Signature=") {
		t.Errorf("signed URL = %q; want the configured bucket's object, signed", signed)
	}
}

// TestStorage_PresignURL_ExplicitPresignBuckets covers the streaming wiring:
// a Storage whose own BucketName is NOT the bucket the HLS proxy reads (local
// MinIO holds admin uploads in MINIO_BUCKET but serves library HLS out of
// raw-library) presigns exactly the buckets it was configured to presign.
func TestStorage_PresignURL_ExplicitPresignBuckets(t *testing.T) {
	s, err := NewStorage(StorageConfig{
		Endpoint:        "minio-fake:9000",
		AccessKeyID:     "fake-access-key",
		SecretAccessKey: "fake-secret-key",
		BucketName:      "animeenigma",
		Region:          "us-east-1",
		PresignBuckets:  []string{"raw-library"},
	})
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	if _, ok := s.PresignURL("http://minio-fake:9000/raw-library/54974/1/playlist.m3u8"); !ok {
		t.Errorf("expected the allowlisted bucket to be presigned")
	}
	// Its own BucketName is not in the allowlist, so it is refused too:
	// PresignBuckets is the whole scope, not an addition to BucketName.
	if _, ok := s.PresignURL("http://minio-fake:9000/animeenigma/videos/x/ep1.mp4"); ok {
		t.Errorf("expected a bucket outside PresignBuckets to be refused")
	}
}

// TestStorage_PresignURL_RejectsTraversalKeys locks the object-key shape gate
// (storage.go safeObjectKey). PresignURL's URL is attacker-controllable (the
// HLS proxy's `url=` parameter), and minio-go signs a key containing dot
// segments verbatim, so "raw-library/../gacha-cards/secret.png" would be
// admitted as bucket "raw-library" yet resolve to another bucket's object on
// the server. Keys must arrive already resolved.
func TestStorage_PresignURL_RejectsTraversalKeys(t *testing.T) {
	s := newFakeStorage(t, "minio-fake:9000", false)

	rejected := []struct {
		name string
		url  string
	}{
		{"parent segment escapes the bucket", "http://minio-fake:9000/raw-library/../gacha-cards/secret.png"},
		{"parent segment mid-key", "http://minio-fake:9000/raw-library/aeProvider/1/../../gacha-cards/secret.png"},
		{"percent-encoded traversal", "http://minio-fake:9000/raw-library/%2e%2e/gacha-cards/secret.png"},
		{"double-encoded traversal", "http://minio-fake:9000/raw-library/%252e%252e/gacha-cards/secret.png"},
		{"current-dir segment", "http://minio-fake:9000/raw-library/./playlist.m3u8"},
		{"empty interior segment", "http://minio-fake:9000/raw-library/aeProvider//RAW/1/playlist.m3u8"},
		{"trailing empty segment", "http://minio-fake:9000/raw-library/aeProvider/1/RAW/1/"},
	}
	for _, tc := range rejected {
		if signed, ok := s.PresignURL(tc.url); ok {
			t.Errorf("%s: PresignURL(%q) = (%q, true); want it rejected", tc.name, tc.url, signed)
		}
	}
}

// TestStorage_PresignURL_AcceptsRealLayouts guards the other side of the gate:
// every object layout actually written to our buckets must still presign, so
// the traversal check can never break real playback. Dots INSIDE a segment
// ("playlist.m3u8", "storyboard.vtt") are legitimate and must survive.
func TestStorage_PresignURL_AcceptsRealLayouts(t *testing.T) {
	s := newFakeStorage(t, "minio-fake:9000", false)

	accepted := []struct {
		name string
		key  string
	}{
		{"pool playlist", "aeProvider/54974/RAW/1/playlist.m3u8"},
		{"pool segment", "aeProvider/54974/RAW/1/segment_003.ts"},
		{"pool storyboard sheet", "aeProvider/54974/RAW/12/storyboard_001.jpg"},
		{"pool storyboard vtt", "aeProvider/54974/RAW/12/storyboard.vtt"},
		{"upscaled pool", "aeProvider/54974/UPSCALED-720p/3/playlist.m3u8"},
		{"pending job", "pending/6f1c9e2a-0b6d-4f5a-9a3c-1d2e3f4a5b6c/1/playlist.m3u8"},
		{"legacy pre-pool", "54974/1/playlist.m3u8"},
		{"self-hosted video", "videos/54974/ep1_720.mp4"},
		{"thumbnail", "thumbnails/54974/ep1.jpg"},
		{"poster", "posters/54974.jpg"},
		{"dots inside a segment name", "videos/54974/ep..1.mp4"},
	}
	for _, tc := range accepted {
		raw := "http://minio-fake:9000/raw-library/" + tc.key
		signed, ok := s.PresignURL(raw)
		if !ok {
			t.Errorf("%s: PresignURL(%q) = ok:false; real layouts must still presign", tc.name, raw)
			continue
		}
		if !strings.Contains(signed, "X-Amz-Signature=") {
			t.Errorf("%s: signed URL %q missing signature params", tc.name, signed)
		}
	}
}

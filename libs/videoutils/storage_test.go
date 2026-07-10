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

// TestStorage_PresignURL_UsesURLBucketNotConfiguredBucket pins the actual
// behavior of Storage.PresignURL: the bucket used in the presigned request
// is parsed from the URL path segment (storage.go:102), NOT the Storage's
// configured BucketName. In production both storages point at buckets named
// "raw-library" so this distinction is invisible, but MultiStorage's
// host-routing design depends on knowing which one actually wins.
func TestStorage_PresignURL_UsesURLBucketNotConfiguredBucket(t *testing.T) {
	s := newFakeStorage(t, "minio-fake:9000", false) // configured bucket: raw-library

	signed, ok := s.PresignURL("http://minio-fake:9000/some-other-bucket/videos/ep1.ts")
	if !ok {
		t.Fatalf("expected PresignURL to succeed")
	}
	if !strings.Contains(signed, "/some-other-bucket/") {
		t.Errorf("signed URL = %q; want it to contain the URL-parsed bucket %q (not the configured bucket %q)", signed, "some-other-bucket", "raw-library")
	}
	if strings.Contains(signed, "/raw-library/") {
		t.Errorf("signed URL = %q; unexpectedly used the configured bucket instead of the URL-parsed one", signed)
	}
}

package minio

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/minio/minio-go/v7"
)

// fakeUploader records every PutObject + MakeBucket call.
// segmentsDone is an atomic counter the test inspects to assert
// the playlist PutObject is invoked AFTER every segment finishes.
type fakeUploader struct {
	mu sync.Mutex

	bucketExists    bool
	bucketExistsErr error
	makeBucketErr   error
	makeBucketCalls []string
	bucketsCreated  int32

	// PutObject hooks.
	putErr        map[string]error // by object name
	putCalls      []putCall
	segmentsDone  atomic.Int32
	playlistOrder int32 // captures the segmentsDone value when playlist is PUT

	// Move / list / copy / remove hooks (unused in upscaler writer, kept for
	// Uploader interface satisfaction).
	listResults map[string][]minio.ObjectInfo
	listErr     error
	copyErr     map[string]error
	copyCalls   []copyCall
	removeErr   map[string]error
	removeCalls []string
}

type copyCall struct {
	srcBucket, srcObject string
	dstBucket, dstObject string
}

type putCall struct {
	bucket      string
	object      string
	contentType string
	size        int64
}

func (f *fakeUploader) BucketExists(_ context.Context, _ string) (bool, error) {
	return f.bucketExists, f.bucketExistsErr
}

func (f *fakeUploader) MakeBucket(_ context.Context, bucket string, _ minio.MakeBucketOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.makeBucketCalls = append(f.makeBucketCalls, bucket)
	atomic.AddInt32(&f.bucketsCreated, 1)
	return f.makeBucketErr
}

func (f *fakeUploader) PutObject(_ context.Context, bucket, object string, r interface {
	Read(p []byte) (int, error)
}, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	// Drain the reader so we observe the same byte count as the real client.
	_, _ = io.Copy(io.Discard, readerAdapter{r: r})
	f.mu.Lock()
	if err, ok := f.putErr[object]; ok && err != nil {
		f.mu.Unlock()
		return minio.UploadInfo{}, err
	}
	f.putCalls = append(f.putCalls, putCall{
		bucket: bucket, object: object, contentType: opts.ContentType, size: size,
	})
	isPlaylist := strings.HasSuffix(object, "playlist.m3u8")
	f.mu.Unlock()
	if isPlaylist {
		f.playlistOrder = f.segmentsDone.Load()
	} else {
		f.segmentsDone.Add(1)
	}
	return minio.UploadInfo{}, nil
}

func (f *fakeUploader) ListObjects(_ context.Context, _ string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	ch := make(chan minio.ObjectInfo)
	go func() {
		defer close(ch)
		if f.listErr != nil {
			ch <- minio.ObjectInfo{Err: f.listErr}
			return
		}
		for _, obj := range f.listResults[opts.Prefix] {
			ch <- obj
		}
	}()
	return ch
}

func (f *fakeUploader) CopyObject(_ context.Context, dst minio.CopyDestOptions, src minio.CopySrcOptions) (minio.UploadInfo, error) {
	f.mu.Lock()
	if err, ok := f.copyErr[src.Object]; ok && err != nil {
		f.mu.Unlock()
		return minio.UploadInfo{}, err
	}
	f.copyCalls = append(f.copyCalls, copyCall{
		srcBucket: src.Bucket, srcObject: src.Object,
		dstBucket: dst.Bucket, dstObject: dst.Object,
	})
	f.mu.Unlock()
	return minio.UploadInfo{}, nil
}

func (f *fakeUploader) RemoveObject(_ context.Context, _ string, object string, _ minio.RemoveObjectOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.removeErr[object]; ok && err != nil {
		return err
	}
	f.removeCalls = append(f.removeCalls, object)
	return nil
}

func (f *fakeUploader) GetObject(_ context.Context, _, _ string) (io.ReadCloser, error) {
	// writer_test.go does not exercise GetObject; satisfy the interface.
	return io.NopCloser(strings.NewReader("")), nil
}

// makeTempFile creates a file with content under dir and returns its path.
func makeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

func TestUpload_PlaylistLast_AndContentType(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		makeTempFile(t, dir, "segment_001.ts", "AAAA"),
		makeTempFile(t, dir, "segment_002.ts", "BBBB"),
		makeTempFile(t, dir, "segment_003.ts", "CCCC"),
		makeTempFile(t, dir, "segment_004.ts", "DDDD"),
		makeTempFile(t, dir, "segment_005.ts", "EEEE"),
		makeTempFile(t, dir, "segment_006.ts", "FFFF"),
		makeTempFile(t, dir, "playlist.m3u8", "#EXTM3U\n"),
	}
	fake := &fakeUploader{bucketExists: true}
	w := newWriterWithUploader(Config{
		Endpoint: "minio:9000", Bucket: "raw-library", UploadConcurrency: 4,
	}, fake, nil)

	totalBytes, err := w.Upload(context.Background(), "57466/1/", files)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	wantBytes := int64(4*6 + len("#EXTM3U\n"))
	if totalBytes != wantBytes {
		t.Errorf("bytes = %d, want %d", totalBytes, wantBytes)
	}
	if len(fake.putCalls) != 7 {
		t.Fatalf("put calls = %d, want 7", len(fake.putCalls))
	}
	// Playlist must be PUT after all 6 segments completed.
	if fake.playlistOrder != 6 {
		t.Errorf("playlist PUT observed %d segments done, want 6", fake.playlistOrder)
	}
	// Content-Type check.
	var sawTs, sawM3u8 bool
	for _, c := range fake.putCalls {
		if strings.HasSuffix(c.object, ".ts") {
			if c.contentType != "video/mp2t" {
				t.Errorf("ts content-type = %q, want video/mp2t", c.contentType)
			}
			sawTs = true
		}
		if strings.HasSuffix(c.object, ".m3u8") {
			if c.contentType != "application/vnd.apple.mpegurl" {
				t.Errorf("m3u8 content-type = %q, want application/vnd.apple.mpegurl", c.contentType)
			}
			sawM3u8 = true
		}
	}
	if !sawTs || !sawM3u8 {
		t.Errorf("missing content-type assertions: ts=%v m3u8=%v", sawTs, sawM3u8)
	}
	// Object keys include the prefix.
	for _, c := range fake.putCalls {
		if !strings.HasPrefix(c.object, "57466/1/") {
			t.Errorf("object %q missing prefix 57466/1/", c.object)
		}
	}
}

func TestUpload_SegmentErrorAbortsPlaylist(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		makeTempFile(t, dir, "segment_001.ts", "A"),
		makeTempFile(t, dir, "segment_002.ts", "B"),
		makeTempFile(t, dir, "segment_003.ts", "C"),
		makeTempFile(t, dir, "playlist.m3u8", "P"),
	}
	fake := &fakeUploader{
		bucketExists: true,
		putErr:       map[string]error{"pre/segment_003.ts": errors.New("simulated")},
	}
	w := newWriterWithUploader(Config{
		Endpoint: "minio:9000", Bucket: "raw-library", UploadConcurrency: 4,
	}, fake, nil)

	_, err := w.Upload(context.Background(), "pre", files) // prefix without trailing slash → writer adds /
	if err == nil {
		t.Fatal("expected error from segment failure")
	}
	// Playlist must never be uploaded.
	for _, c := range fake.putCalls {
		if strings.HasSuffix(c.object, "playlist.m3u8") {
			t.Errorf("playlist must NOT be uploaded after segment failure; got %v", c)
		}
	}
}

func TestEnsureBucket_AlreadyExists_NoError(t *testing.T) {
	fake := &fakeUploader{bucketExists: true}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)
	if err := w.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if fake.bucketsCreated != 0 {
		t.Errorf("MakeBucket should NOT be called when bucket exists; got %d calls", fake.bucketsCreated)
	}
}

func TestEnsureBucket_RaceSwallowed(t *testing.T) {
	fake := &fakeUploader{
		bucketExists:  false,
		makeBucketErr: minio.ErrorResponse{Code: "BucketAlreadyOwnedByYou"},
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)
	if err := w.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket must swallow BucketAlreadyOwnedByYou; got %v", err)
	}
}

func TestEnsureBucket_NewBucketCreated(t *testing.T) {
	fake := &fakeUploader{bucketExists: false}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)
	if err := w.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if fake.bucketsCreated != 1 {
		t.Errorf("MakeBucket should fire once when bucket absent; got %d", fake.bucketsCreated)
	}
}

func TestEnsureBucket_RealMakeBucketErrorBubbles(t *testing.T) {
	fake := &fakeUploader{
		bucketExists:  false,
		makeBucketErr: errors.New("permissions denied"),
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)
	if err := w.EnsureBucket(context.Background()); err == nil {
		t.Fatalf("EnsureBucket must surface non-race errors")
	}
}

func TestURLFor_HTTP(t *testing.T) {
	w := newWriterWithUploader(Config{
		Endpoint: "minio:9000", Bucket: "raw-library", UseSSL: false,
	}, &fakeUploader{}, nil)
	got := w.URLFor("foo/bar.m3u8")
	want := "http://minio:9000/raw-library/foo/bar.m3u8"
	if got != want {
		t.Errorf("URLFor = %q, want %q", got, want)
	}
}

func TestURLFor_HTTPS(t *testing.T) {
	w := newWriterWithUploader(Config{
		Endpoint: "minio.example.com", Bucket: "raw-library", UseSSL: true,
	}, &fakeUploader{}, nil)
	got := w.URLFor("57466/1/playlist.m3u8")
	want := "https://minio.example.com/raw-library/57466/1/playlist.m3u8"
	if got != want {
		t.Errorf("URLFor = %q, want %q", got, want)
	}
}

func TestContentTypeFor_Cases(t *testing.T) {
	cases := []struct{ name, want string }{
		{"playlist.m3u8", "application/vnd.apple.mpegurl"},
		{"PLAYLIST.M3U8", "application/vnd.apple.mpegurl"},
		{"segment_001.ts", "video/mp2t"},
		{"unknown.bin", "application/octet-stream"},
		{"no_extension", "application/octet-stream"},
	}
	for _, c := range cases {
		got := contentTypeFor(c.name)
		if got != c.want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

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
// `segmentsDone` is an atomic counter the test inspects to assert
// the playlist PutObject is invoked AFTER every segment finishes.
//
// Phase-5 additions: list / copy / remove hooks for Move() tests.
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

	// Phase-5 hooks for Move().
	listResults map[string][]minio.ObjectInfo // by prefix
	listErr     error
	copyErr     map[string]error // by src object key
	copyCalls   []copyCall
	removeErr   map[string]error // by object key
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

func (f *fakeUploader) BucketExists(_ context.Context, bucket string) (bool, error) {
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
	isPlaylist := strings.HasSuffix(object, "/playlist.m3u8") || object == "playlist.m3u8" || strings.HasSuffix(object, "playlist.m3u8")
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

// makeTempFile creates a file with content under dir; returns path.
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

	bytes, err := w.Upload(context.Background(), "57466/1/", files)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	wantBytes := int64(4*6 + len("#EXTM3U\n"))
	if bytes != wantBytes {
		t.Errorf("bytes = %d, want %d", bytes, wantBytes)
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
	// Object keys = bucket + prefix + basename — verify prefix lock.
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

func TestMove_HappyPath_CopiesThenRemoves(t *testing.T) {
	fake := &fakeUploader{
		listResults: map[string][]minio.ObjectInfo{
			"pending/abc/1/": {
				{Key: "pending/abc/1/playlist.m3u8"},
				{Key: "pending/abc/1/segment_000.ts"},
				{Key: "pending/abc/1/segment_001.ts"},
			},
		},
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)

	if err := w.Move(context.Background(), "pending/abc/1/", "57466/1/"); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(fake.copyCalls) != 3 {
		t.Fatalf("copy calls = %d, want 3", len(fake.copyCalls))
	}
	wantDst := map[string]string{
		"pending/abc/1/playlist.m3u8":  "57466/1/playlist.m3u8",
		"pending/abc/1/segment_000.ts": "57466/1/segment_000.ts",
		"pending/abc/1/segment_001.ts": "57466/1/segment_001.ts",
	}
	for _, c := range fake.copyCalls {
		want, ok := wantDst[c.srcObject]
		if !ok {
			t.Errorf("unexpected src %q", c.srcObject)
			continue
		}
		if c.dstObject != want {
			t.Errorf("dst for %q = %q, want %q", c.srcObject, c.dstObject, want)
		}
	}
	// All sources should have been removed after copy.
	if len(fake.removeCalls) != 3 {
		t.Fatalf("remove calls = %d, want 3 (after all copies succeed)", len(fake.removeCalls))
	}
}

func TestMove_EmptySource_Errors(t *testing.T) {
	fake := &fakeUploader{
		listResults: map[string][]minio.ObjectInfo{
			"pending/abc/1/": nil,
		},
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)
	err := w.Move(context.Background(), "pending/abc/1/", "57466/1/")
	if err == nil {
		t.Fatalf("Move on empty src must error")
	}
	if len(fake.copyCalls) != 0 {
		t.Errorf("no copies should fire on empty src; got %d", len(fake.copyCalls))
	}
	if len(fake.removeCalls) != 0 {
		t.Errorf("no removes should fire on empty src; got %d", len(fake.removeCalls))
	}
}

func TestMove_CopyError_AbortsWithoutRemove(t *testing.T) {
	fake := &fakeUploader{
		listResults: map[string][]minio.ObjectInfo{
			"pending/abc/1/": {
				{Key: "pending/abc/1/segment_000.ts"},
				{Key: "pending/abc/1/segment_001.ts"},
			},
		},
		copyErr: map[string]error{
			"pending/abc/1/segment_001.ts": errors.New("simulated copy fail"),
		},
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)

	err := w.Move(context.Background(), "pending/abc/1/", "57466/1/")
	if err == nil {
		t.Fatalf("Move should propagate copy error")
	}
	// CRITICAL: no source should be removed when any copy fails — data preserved.
	if len(fake.removeCalls) != 0 {
		t.Fatalf("RemoveObject must NOT fire on copy failure; got %d removes", len(fake.removeCalls))
	}
}

func TestMove_RemoveError_SoftFailsButReturnsNil(t *testing.T) {
	fake := &fakeUploader{
		listResults: map[string][]minio.ObjectInfo{
			"pending/abc/1/": {
				{Key: "pending/abc/1/segment_000.ts"},
			},
		},
		removeErr: map[string]error{
			"pending/abc/1/segment_000.ts": errors.New("simulated remove fail"),
		},
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)

	// Copy succeeds — data is at destination. Remove failure is a soft
	// orphan that we log but don't propagate.
	if err := w.Move(context.Background(), "pending/abc/1/", "57466/1/"); err != nil {
		t.Fatalf("Move should not propagate remove failures: %v", err)
	}
	if len(fake.copyCalls) != 1 {
		t.Errorf("copy should have fired; got %d", len(fake.copyCalls))
	}
}

func TestMove_NormalizesPrefixSlash(t *testing.T) {
	fake := &fakeUploader{
		listResults: map[string][]minio.ObjectInfo{
			"pending/abc/1/": {
				{Key: "pending/abc/1/playlist.m3u8"},
			},
		},
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)

	// Pass prefixes WITHOUT trailing slash — Move should normalize.
	if err := w.Move(context.Background(), "pending/abc/1", "57466/1"); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(fake.copyCalls) != 1 {
		t.Fatalf("copy calls = %d, want 1", len(fake.copyCalls))
	}
	if fake.copyCalls[0].dstObject != "57466/1/playlist.m3u8" {
		t.Errorf("dst = %q, want 57466/1/playlist.m3u8", fake.copyCalls[0].dstObject)
	}
}

func TestListObjectsByPrefix_SortsDeterministically(t *testing.T) {
	fake := &fakeUploader{
		listResults: map[string][]minio.ObjectInfo{
			"pending/abc/1/": {
				{Key: "pending/abc/1/segment_010.ts"},
				{Key: "pending/abc/1/playlist.m3u8"},
				{Key: "pending/abc/1/segment_001.ts"},
			},
		},
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)

	keys, err := w.ListObjectsByPrefix(context.Background(), "pending/abc/1/")
	if err != nil {
		t.Fatalf("ListObjectsByPrefix: %v", err)
	}
	want := []string{
		"pending/abc/1/playlist.m3u8",
		"pending/abc/1/segment_001.ts",
		"pending/abc/1/segment_010.ts",
	}
	if len(keys) != len(want) {
		t.Fatalf("len = %d, want %d", len(keys), len(want))
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("keys[%d] = %q, want %q", i, keys[i], want[i])
		}
	}
}

// TestDeletePrefix_RemovesAllKeys asserts DeletePrefix removes every object
// under the prefix (the Plan-10 evict-MinIO half).
func TestDeletePrefix_RemovesAllKeys(t *testing.T) {
	fake := &fakeUploader{
		listResults: map[string][]minio.ObjectInfo{
			"aeProvider/123/RAW/3/": {
				{Key: "aeProvider/123/RAW/3/playlist.m3u8"},
				{Key: "aeProvider/123/RAW/3/segment_000.ts"},
				{Key: "aeProvider/123/RAW/3/segment_001.ts"},
			},
		},
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)

	if err := w.DeletePrefix(context.Background(), "aeProvider/123/RAW/3/"); err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if len(fake.removeCalls) != 3 {
		t.Fatalf("removeCalls = %d, want 3 (all keys deleted)", len(fake.removeCalls))
	}
}

// TestDeletePrefix_NormalizesPrefixSlash verifies a prefix passed WITHOUT a
// trailing slash is normalized before listing.
func TestDeletePrefix_NormalizesPrefixSlash(t *testing.T) {
	fake := &fakeUploader{
		listResults: map[string][]minio.ObjectInfo{
			"aeProvider/123/RAW/3/": {
				{Key: "aeProvider/123/RAW/3/playlist.m3u8"},
			},
		},
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)

	// No trailing slash — DeletePrefix must normalize to hit the listResults key.
	if err := w.DeletePrefix(context.Background(), "aeProvider/123/RAW/3"); err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if len(fake.removeCalls) != 1 {
		t.Fatalf("removeCalls = %d, want 1", len(fake.removeCalls))
	}
}

// TestDeletePrefix_EmptyPrefix_ReturnsNil asserts an empty prefix (0 keys) is a
// nil no-op — evicting an already-gone prefix is idempotent, NOT an error
// (unlike Move, which errors on empty src).
func TestDeletePrefix_EmptyPrefix_ReturnsNil(t *testing.T) {
	fake := &fakeUploader{
		listResults: map[string][]minio.ObjectInfo{
			"aeProvider/999/RAW/1/": nil,
		},
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)

	if err := w.DeletePrefix(context.Background(), "aeProvider/999/RAW/1/"); err != nil {
		t.Fatalf("DeletePrefix on empty prefix must return nil, got %v", err)
	}
	if len(fake.removeCalls) != 0 {
		t.Fatalf("removeCalls = %d, want 0 on empty prefix", len(fake.removeCalls))
	}
}

// TestDeletePrefix_RemoveError_PropagatesImmediately is the critical T-10-02
// guard: on the FIRST RemoveObject error DeletePrefix returns that error and
// does NOT continue (a half-deleted prefix must never report success, so the
// caller leaves the row for the next sweep). Keys are listed sorted, so the
// failure on segment_000 stops before segment_001 is attempted.
func TestDeletePrefix_RemoveError_PropagatesImmediately(t *testing.T) {
	fake := &fakeUploader{
		listResults: map[string][]minio.ObjectInfo{
			"aeProvider/123/RAW/3/": {
				{Key: "aeProvider/123/RAW/3/playlist.m3u8"},
				{Key: "aeProvider/123/RAW/3/segment_000.ts"},
				{Key: "aeProvider/123/RAW/3/segment_001.ts"},
			},
		},
		removeErr: map[string]error{
			"aeProvider/123/RAW/3/segment_000.ts": errors.New("simulated remove fail"),
		},
	}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)

	err := w.DeletePrefix(context.Background(), "aeProvider/123/RAW/3/")
	if err == nil {
		t.Fatal("DeletePrefix must propagate the first RemoveObject error")
	}
	// ListObjectsByPrefix sorts: playlist.m3u8 < segment_000.ts < segment_001.ts.
	// playlist removed OK, segment_000 fails → segment_001 never attempted.
	if len(fake.removeCalls) != 1 {
		t.Fatalf("removeCalls = %d, want 1 (hard-fail stops at first error, leaving segment_001 untouched)", len(fake.removeCalls))
	}
	if fake.removeCalls[0] != "aeProvider/123/RAW/3/playlist.m3u8" {
		t.Fatalf("first remove = %q, want the sorted-first key playlist.m3u8", fake.removeCalls[0])
	}
}

// TestDeletePrefix_ListError_Propagates asserts a list failure is wrapped and
// returned (no removes attempted).
func TestDeletePrefix_ListError_Propagates(t *testing.T) {
	fake := &fakeUploader{listErr: errors.New("simulated list fail")}
	w := newWriterWithUploader(Config{Endpoint: "minio:9000", Bucket: "raw-library"}, fake, nil)

	if err := w.DeletePrefix(context.Background(), "aeProvider/123/RAW/3/"); err == nil {
		t.Fatal("DeletePrefix must propagate a list error")
	}
	if len(fake.removeCalls) != 0 {
		t.Fatalf("removeCalls = %d, want 0 on list error", len(fake.removeCalls))
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

// Package minio is the upscaler service's HLS uploader. Ported from
// services/library/internal/minio/writer.go with the same semantics:
//
//   - Idempotent bucket bootstrap (`raw-library` is created at service
//     start; BucketAlreadyOwnedByYou is swallowed).
//   - Concurrent segment upload + LAST playlist upload so HLS clients
//     never see a playlist referring to an unfinished segment.
//   - Content-Type per extension (`application/vnd.apple.mpegurl`
//     for .m3u8, `video/mp2t` for .ts).
//   - URLFor helper that returns the internal `http://endpoint/bucket/path`
//     URL the streaming proxy fronts.
package minio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/sync/errgroup"
)

// Config holds connection + bucket knobs.
type Config struct {
	Endpoint          string
	AccessKey         string
	SecretKey         string
	Bucket            string
	UseSSL            bool
	UploadConcurrency int // default 8 when <= 0
}

// Uploader is the slice of *minio.Client the Writer uses. Pulled out
// so unit tests can swap in a fake without spinning up a real MinIO.
type Uploader interface {
	BucketExists(ctx context.Context, bucket string) (bool, error)
	MakeBucket(ctx context.Context, bucket string, opts minio.MakeBucketOptions) error
	PutObject(ctx context.Context, bucket, object string, reader interface {
		Read(p []byte) (int, error)
	}, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	ListObjects(ctx context.Context, bucket string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo
	CopyObject(ctx context.Context, dst minio.CopyDestOptions, src minio.CopySrcOptions) (minio.UploadInfo, error)
	RemoveObject(ctx context.Context, bucket, object string, opts minio.RemoveObjectOptions) error
	// GetObject retrieves an object from MinIO and returns a streaming reader.
	// The caller MUST close the returned io.ReadCloser when done.
	GetObject(ctx context.Context, bucket, object string) (io.ReadCloser, error)
}

// minioClientAdapter wraps the real *minio.Client to satisfy Uploader.
type minioClientAdapter struct {
	c *minio.Client
}

func (a *minioClientAdapter) BucketExists(ctx context.Context, bucket string) (bool, error) {
	return a.c.BucketExists(ctx, bucket)
}

func (a *minioClientAdapter) MakeBucket(ctx context.Context, bucket string, opts minio.MakeBucketOptions) error {
	return a.c.MakeBucket(ctx, bucket, opts)
}

func (a *minioClientAdapter) PutObject(ctx context.Context, bucket, object string, reader interface {
	Read(p []byte) (int, error)
}, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return a.c.PutObject(ctx, bucket, object, readerAdapter{r: reader}, size, opts)
}

func (a *minioClientAdapter) ListObjects(ctx context.Context, bucket string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	return a.c.ListObjects(ctx, bucket, opts)
}

func (a *minioClientAdapter) CopyObject(ctx context.Context, dst minio.CopyDestOptions, src minio.CopySrcOptions) (minio.UploadInfo, error) {
	return a.c.CopyObject(ctx, dst, src)
}

func (a *minioClientAdapter) RemoveObject(ctx context.Context, bucket, object string, opts minio.RemoveObjectOptions) error {
	return a.c.RemoveObject(ctx, bucket, object, opts)
}

func (a *minioClientAdapter) GetObject(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	obj, err := a.c.GetObject(ctx, bucket, object, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// GetObject retrieves an object from MinIO using the Writer's configured bucket.
// This is the high-level helper that delegates to the underlying Uploader's GetObject.
func (w *Writer) GetObject(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	if bucket == "" {
		bucket = w.cfg.Bucket
	}
	return w.uploader.GetObject(ctx, bucket, object)
}

// readerAdapter promotes an "interface{ Read }" to io.Reader for the SDK.
type readerAdapter struct {
	r interface {
		Read(p []byte) (int, error)
	}
}

func (a readerAdapter) Read(p []byte) (int, error) { return a.r.Read(p) }

// Writer is the public façade.
type Writer struct {
	cfg      Config
	uploader Uploader
	log      *logger.Logger
}

// New constructs a Writer backed by a real *minio.Client.
func New(cfg Config, log *logger.Logger) (*Writer, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("minio endpoint required")
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "raw-library"
	}
	if cfg.UploadConcurrency <= 0 {
		cfg.UploadConcurrency = 8
	}
	c, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("init minio client: %w", err)
	}
	return &Writer{cfg: cfg, uploader: &minioClientAdapter{c: c}, log: log}, nil
}

// RawUploader returns the underlying Uploader so callers that need streaming
// PutObject access (e.g. the model admin handler's TeeReader upload) can use
// it directly without going through the Writer's byte-slice PutObject helper.
func (w *Writer) RawUploader() Uploader { return w.uploader }

// newWriterWithUploader is the test seam — bypasses the real
// minio.Client construction so unit tests inject a fake Uploader.
func newWriterWithUploader(cfg Config, u Uploader, log *logger.Logger) *Writer {
	if cfg.Bucket == "" {
		cfg.Bucket = "raw-library"
	}
	if cfg.UploadConcurrency <= 0 {
		cfg.UploadConcurrency = 8
	}
	return &Writer{cfg: cfg, uploader: u, log: log}
}

// EnsureBucket creates cfg.Bucket if absent. Idempotent: swallows
// BucketAlreadyOwnedByYou / BucketAlreadyExists ErrorResponse codes
// so two instances starting concurrently can both succeed.
func (w *Writer) EnsureBucket(ctx context.Context) error {
	exists, err := w.uploader.BucketExists(ctx, w.cfg.Bucket)
	if err == nil && exists {
		return nil
	}
	if err != nil && !isBucketRaceError(err) {
		if w.log != nil {
			w.log.Warnw("minio bucket exists probe failed; attempting MakeBucket",
				"bucket", w.cfg.Bucket, "error", err)
		}
	}
	mkErr := w.uploader.MakeBucket(ctx, w.cfg.Bucket, minio.MakeBucketOptions{})
	if mkErr == nil {
		return nil
	}
	if isBucketRaceError(mkErr) {
		return nil
	}
	return fmt.Errorf("minio make bucket: %w", mkErr)
}

// isBucketRaceError reports whether the err is one of the "bucket already
// exists" responses we want to swallow.
func isBucketRaceError(err error) bool {
	if err == nil {
		return false
	}
	var er minio.ErrorResponse
	if errors.As(err, &er) {
		return er.Code == "BucketAlreadyOwnedByYou" || er.Code == "BucketAlreadyExists"
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "bucketalreadyownedbyyou") ||
		strings.Contains(msg, "bucketalreadyexists")
}

// contentTypeFor maps a filename extension to its Content-Type.
func contentTypeFor(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	default:
		return "application/octet-stream"
	}
}

// Upload PUTs every file in filePaths to {bucket}/{prefix}/{basename}.
//
// Order semantics (LOCKED):
//  1. Segments upload concurrently via errgroup with SetLimit(cfg.UploadConcurrency).
//  2. The playlist (basename == "playlist.m3u8") uploads LAST on the
//     main goroutine — never in parallel with segments. This guarantees
//     HLS clients never see a playlist referring to a not-yet-uploaded segment.
//  3. On any segment error, the playlist is never uploaded.
//
// Returns total bytes PUT to MinIO (sum of os.Stat sizes).
func (w *Writer) Upload(ctx context.Context, prefix string, filePaths []string) (int64, error) {
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
	var playlist string
	var segments []string
	for _, p := range filePaths {
		if filepath.Base(p) == "playlist.m3u8" {
			playlist = p
			continue
		}
		segments = append(segments, p)
	}

	mu := newByteCounter()

	eg, gctx := errgroup.WithContext(ctx)
	eg.SetLimit(w.cfg.UploadConcurrency)
	for _, seg := range segments {
		seg := seg
		eg.Go(func() error {
			n, err := w.putFile(gctx, prefix, seg)
			if err != nil {
				return err
			}
			mu.add(n)
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return mu.value(), err
	}

	// Playlist — main goroutine, after every segment is done.
	if playlist != "" {
		n, err := w.putFile(ctx, prefix, playlist)
		if err != nil {
			return mu.value(), err
		}
		mu.add(n)
	}
	return mu.value(), nil
}

// putFile PUTs a single file at {bucket}/{prefix}{basename}.
func (w *Writer) putFile(ctx context.Context, prefix, path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}
	object := prefix + filepath.Base(path)
	_, err = w.uploader.PutObject(ctx, w.cfg.Bucket, object, f, st.Size(), minio.PutObjectOptions{
		ContentType: contentTypeFor(path),
	})
	if err != nil {
		return 0, fmt.Errorf("put %s: %w", object, err)
	}
	return st.Size(), nil
}

// byteCounter is a mutex-protected int64 accumulator for concurrent totals.
type byteCounter struct {
	mu sync.Mutex
	n  int64
}

func newByteCounter() *byteCounter { return &byteCounter{} }

func (c *byteCounter) add(n int64) {
	c.mu.Lock()
	c.n += n
	c.mu.Unlock()
}

func (c *byteCounter) value() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// PutObject writes an in-memory byte payload to {bucket}/{key} with the given
// Content-Type. Unlike Upload (which streams files from disk), this is for small
// generated artifacts — e.g. the per-job log dump the LogBuffer flushes at
// finalize. It satisfies the service.logFlusher interface.
func (w *Writer) PutObject(ctx context.Context, bucket, key string, data []byte, contentType string) error {
	if bucket == "" {
		bucket = w.cfg.Bucket
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := w.uploader.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("put object %s/%s: %w", bucket, key, err)
	}
	return nil
}

// URLFor returns the internal MinIO HTTP URL for a bucket-relative path.
// NOT a presigned URL — the streaming proxy fronts public access.
func (w *Writer) URLFor(path string) string {
	scheme := "http"
	if w.cfg.UseSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, w.cfg.Endpoint, w.cfg.Bucket, path)
}
